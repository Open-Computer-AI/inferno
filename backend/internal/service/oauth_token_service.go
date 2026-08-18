package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthdeviceauthorization"

	"github.com/golang-jwt/jwt/v5"
)

// RFC 8628 §3.5 error codes. The hermes client branches on these exact
// strings (hermes_cli/auth.py) — do not reword them.
var (
	ErrAuthorizationPending = errors.New("authorization_pending")
	ErrSlowDown             = errors.New("slow_down")
	ErrAccessDenied         = errors.New("access_denied")
	ErrExpiredToken         = errors.New("expired_token")

	// ErrInvalidGrant is RFC 6749 §5.2's error code for the refresh_token
	// grant: unknown token, wrong client, or expired. Deliberately the same
	// code for all three — distinguishing them on the wire would let a
	// caller enumerate token/client existence.
	ErrInvalidGrant = errors.New("invalid_grant")
)

const (
	// oauthAccessTokenTTL bounds how long an RS256 access token is valid.
	// Short-lived by design: the refresh token (below) is the long-lived
	// credential, and it is rotated on every use.
	oauthAccessTokenTTL = 15 * time.Minute

	// oauthRefreshTokenTTL bounds how long an issued refresh token — and its
	// whole rotation family — stays redeemable before a device must re-run
	// the authorization flow from scratch.
	oauthRefreshTokenTTL = 30 * 24 * time.Hour

	// oauthRefreshTokenPrefix distinguishes OAuth-issued refresh tokens from
	// Inferno's panel-session refresh tokens (which use refreshTokenPrefix,
	// "rt_") on the wire. Both classes of token end up hashed into the same
	// RefreshTokenCache keyspace, so this is a defense-in-depth type tag,
	// not what actually keeps them apart — the stored RefreshTokenData.
	// ClientID field is what a stray cross-flow lookup would fail on.
	oauthRefreshTokenPrefix = "art_"
)

// OAuthTokens is the RFC 8749/8628 token response body (minus token_type,
// which the handler sets to the constant "Bearer").
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	Scope        string
	ExpiresIn    int
}

// OAuthUserLookup is the minimal user-lookup capability
// OAuthTokenService.ExchangeRefreshToken needs to re-validate an account on
// every refresh. Deliberately narrower than the full UserRepository
// interface (which also owns creation, listing, avatars, etc.) — any value
// implementing UserRepository already satisfies this smaller interface
// structurally, no adapter needed, but tests only have to stub one method.
type OAuthUserLookup interface {
	GetByID(ctx context.Context, id int64) (*User, error)
}

// OAuthTokenService implements the device_code and refresh_token grants of
// POST /api/oauth/token. It mints access tokens itself — RS256, signed by
// OAuthKeyService's key — and deliberately does NOT call
// AuthService.GenerateTokenPair, which signs Inferno's separate HMAC panel
// session token. The resource-server middleware accepts RS256 only;
// an HMAC-signed token here would be silently rejected.
//
// Refresh tokens ARE persisted through the same RefreshTokenCache used by
// panel sessions (Redis-backed, SHA256-hashed at rest) rather than a
// parallel store, so the reuse-detection machinery (DeleteTokenFamily)
// stays in one place.
type OAuthTokenService struct {
	entClient    *dbent.Client
	keySvc       *OAuthKeyService
	deviceSvc    *OAuthDeviceService
	clientSvc    *OAuthClientService
	refreshCache RefreshTokenCache
	userRepo     OAuthUserLookup
	issuer       string
}

// NewOAuthTokenService constructs the token-endpoint service. issuer is the
// "iss" claim on minted access tokens — pass the server's own public base
// URL (the JWKS lives at {issuer}/.well-known/jwks.json).
func NewOAuthTokenService(entClient *dbent.Client, keySvc *OAuthKeyService, deviceSvc *OAuthDeviceService, refreshCache RefreshTokenCache, userRepo OAuthUserLookup, issuer string) *OAuthTokenService {
	return &OAuthTokenService{
		entClient: entClient,
		keySvc:    keySvc,
		deviceSvc: deviceSvc,
		// Constructed here rather than injected, matching OAuthDeviceService:
		// both live in package service over the same ent client, and threading
		// a fourth wire dependency through for one lookup buys nothing.
		clientSvc:    NewOAuthClientService(entClient),
		refreshCache: refreshCache,
		userRepo:     userRepo,
		issuer:       issuer,
	}
}

// assertClientUsable re-checks oauth_client.status on every grant.
//
// Checking it only at RequestCode would make `revoked` a one-way door that
// stops new logins while every already-issued refresh family keeps rotating
// for up to 30 more days — i.e. a kill switch that does not kill anything that
// matters. Both grants therefore consult it.
//
// The error is deliberately returned as the caller's ordinary rejection
// sentinel rather than a distinct one; see ErrClientNotUsable.
func (s *OAuthTokenService) assertClientUsable(ctx context.Context, clientID string) error {
	_, err := s.clientSvc.UsableByClientID(ctx, clientID)
	return err
}

// mintAccessToken signs an RS256 JWT whose audience is the client_id, so an
// agent can verify a token was minted for it and no other instance.
func (s *OAuthTokenService) mintAccessToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	key, err := s.keySvc.Active(ctx)
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":                    s.issuer,
		"sub":                    strconv.FormatInt(userID, 10),
		"aud":                    clientID,
		"scope":                  scope,
		"iat":                    now.Unix(),
		"exp":                    now.Add(oauthAccessTokenTTL).Unix(),
		"oauth_contract_version": 1,
	}
	// agent_instance_id is the client_id suffix, which the gateway
	// cross-checks against its own configured client_id as defense-in-depth
	// (plugins/dashboard_auth/nous/__init__.py:33-35 in the read-only client
	// repo).
	if rest, ok := strings.CutPrefix(clientID, "agent:"); ok {
		claims["agent_instance_id"] = rest
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = key.Kid

	signed, err := tok.SignedString(key.Private)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}
	return signed, nil
}

func newOAuthRefreshToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return oauthRefreshTokenPrefix + hex.EncodeToString(raw), nil
}

func newFamilyID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// issueRefreshToken mints a new raw refresh token, stores only its SHA256
// hash (via hashToken, shared with the panel-session refresh flow in
// auth_service.go) in RefreshTokenCache, and returns the raw value — which
// exists only in this return value and the caller's TLS response body,
// never in Redis or a log line.
// tokenVersion is the credential-invalidation fingerprint from
// resolvedTokenVersion(user) — a hash of the user's email + password_hash.
// Stamping it here (and comparing it in ExchangeRefreshToken) is what makes an
// in-app password or email change kill an OAuth agent credential, exactly as
// AuthService.RefreshTokenPair does for panel sessions.
func (s *OAuthTokenService) issueRefreshToken(ctx context.Context, userID int64, clientID, scope, familyID string, tokenVersion int64) (string, error) {
	raw, err := newOAuthRefreshToken()
	if err != nil {
		return "", fmt.Errorf("refresh token entropy: %w", err)
	}
	tokenHash := hashToken(raw)

	now := time.Now()
	data := &RefreshTokenData{
		UserID:       userID,
		TokenVersion: tokenVersion,
		FamilyID:     familyID,
		ClientID:     clientID,
		Scope:        scope,
		CreatedAt:    now,
		ExpiresAt:    now.Add(oauthRefreshTokenTTL),
	}

	if err := s.refreshCache.StoreRefreshToken(ctx, tokenHash, data, oauthRefreshTokenTTL); err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}
	// Shares the user's token set with panel sessions, so an explicit
	// logout-all-devices / forgot-password RESET (both of which call
	// AuthService.RevokeAllUserSessions) kills agent-issued OAuth sessions too,
	// not just browser ones. Fatal, not just logged: a token silently missing
	// from the user set would survive exactly that kind of revocation for the
	// rest of its 30-day life.
	//
	// Note this set does NOT cover an in-app password change:
	// UserService.ChangePassword writes the new hash and returns without
	// calling RevokeAllUserSessions. Panel sessions still die there because
	// AuthService.RefreshTokenPair compares TokenVersion, a fingerprint of
	// email+password_hash. The TokenVersion stamped above, and the matching
	// comparison in ExchangeRefreshToken, is what gives the OAuth path the
	// same property — without it a stolen ~/.hermes/auth.json kept
	// self-rotating for its full 30 days after the victim changed their
	// password, with no UI anywhere listing OAuth grants to tell them.
	if err := s.refreshCache.AddToUserTokenSet(ctx, userID, tokenHash, oauthRefreshTokenTTL); err != nil {
		return "", fmt.Errorf("add refresh token to user set: %w", err)
	}
	if err := s.refreshCache.AddToFamilyTokenSet(ctx, familyID, tokenHash, oauthRefreshTokenTTL); err != nil {
		return "", fmt.Errorf("add refresh token to family: %w", err)
	}

	return raw, nil
}

// ExchangeDeviceCode implements the device_code grant (RFC 8628 §3.5).
func (s *OAuthTokenService) ExchangeDeviceCode(ctx context.Context, clientID, deviceCode string) (*OAuthTokens, error) {
	row, err := s.deviceSvc.byDeviceCode(ctx, deviceCode)
	if err != nil {
		return nil, err
	}

	// Bind the code to the client that requested it — otherwise any
	// registered agent could redeem another agent's pending login.
	if row.ClientID != clientID {
		return nil, ErrAccessDenied
	}

	// A client revoked between requesting the code and redeeming it must not
	// get a token. Collapsed into access_denied, the same code a mismatched
	// client already gets, so this adds no new observable signal.
	if err := s.assertClientUsable(ctx, clientID); err != nil {
		return nil, ErrAccessDenied
	}

	if time.Now().After(row.ExpiresAt) {
		return nil, ErrExpiredToken
	}

	switch row.Status {
	case "pending":
		// Rate-limit the poll loop: a client polling faster than the
		// advertised interval gets slow_down rather than a free retry. Only
		// a poll that is NOT rejected as slow_down advances LastPolledAt —
		// resetting it on every attempt would let a client evade the
		// backoff by polling continuously.
		if row.LastPolledAt != nil && time.Since(*row.LastPolledAt) < time.Duration(devicePollInterval)*time.Second {
			return nil, ErrSlowDown
		}
		if _, uerr := row.Update().SetLastPolledAt(time.Now()).Save(ctx); uerr != nil {
			return nil, fmt.Errorf("update poll timestamp: %w", uerr)
		}
		return nil, ErrAuthorizationPending
	case "denied":
		return nil, ErrAccessDenied
	case "approved":
		// fall through to token issuance below
	default:
		// "expired" (already consumed — see the CAS below) or any other
		// value: RFC 8628 has no "already used" code, so this collapses to
		// expired_token, same as a code past its expires_at.
		return nil, ErrExpiredToken
	}

	if row.ApprovedUserID == nil {
		return nil, ErrAccessDenied
	}
	userID := *row.ApprovedUserID

	// Load the approving user BEFORE consuming the code, for two reasons.
	// First, the refresh token this grant is about to mint must carry the
	// user's current TokenVersion fingerprint (email+password_hash) or an
	// in-app password change can never invalidate it — see issueRefreshToken.
	// Second, an approval is not a session: a user banned or deleted in the
	// window between approving in the browser and the CLI's next poll must not
	// receive a 30-day credential. Doing this before the consuming UPDATE also
	// means a transient user-lookup failure does not burn the human's approval.
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrAccessDenied
		}
		return nil, fmt.Errorf("load user for device grant: %w", err)
	}
	if !user.IsActive() {
		return nil, ErrAccessDenied
	}

	// Single use, race-safe: consume the row with a status-guarded UPDATE
	// instead of an unconditional one. Two concurrent redemptions of the
	// same approved code both read status="approved" above, but only one
	// UPDATE ... WHERE status='approved' can affect a row — the loser sees
	// affected=0 and fails instead of both callers minting a token pair
	// from a single human approval.
	affected, err := s.entClient.OAuthDeviceAuthorization.Update().
		Where(oauthdeviceauthorization.ID(row.ID), oauthdeviceauthorization.Status("approved")).
		SetStatus("expired").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("consume device code: %w", err)
	}
	if affected == 0 {
		return nil, ErrExpiredToken
	}

	familyID, err := newFamilyID()
	if err != nil {
		return nil, fmt.Errorf("family id entropy: %w", err)
	}

	access, err := s.mintAccessToken(ctx, userID, clientID, row.Scope)
	if err != nil {
		return nil, err
	}
	refresh, err := s.issueRefreshToken(ctx, userID, clientID, row.Scope, familyID, resolvedTokenVersion(user))
	if err != nil {
		return nil, err
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		Scope:        row.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}

// ExchangeRefreshToken implements the refresh_token grant (RFC 6749 §6),
// with rotation and family-wide reuse detection.
//
// The rotation itself is a SINGLE atomic Redis operation
// (RefreshTokenCache.MarkRotated) done BEFORE any minting, not a separate
// Go-side read-then-write. Two concurrent presentations of the exact same
// refresh token — an attacker replaying a stolen token at the same moment
// the legitimate client refreshes on schedule — must not both be able to
// observe "not yet rotated" and both mint a token pair from it: that would
// fork the family into two live, independently-rotating branches that never
// again present an already-rotated token to each other, so the reuse
// detector below would never fire. MarkRotated is the single serialization
// point that prevents that: exactly one caller can ever be told it won: see
// TestExchangeRefreshTokenConcurrentPresentationsOnlyOneWins.
//
// The presented token is NOT deleted on success — MarkRotated re-stores it
// under the same hash with Rotated set, for the remainder of its original
// lifetime. This is deliberate: a hard delete would make a later replay of
// that exact raw token indistinguishable from "never existed", which loses
// the FamilyID needed to revoke the rest of the session. Keeping a
// tombstone is what makes the reuse branch below possible at all.
func (s *OAuthTokenService) ExchangeRefreshToken(ctx context.Context, clientID, refreshToken string) (*OAuthTokens, error) {
	if clientID == "" || !strings.HasPrefix(refreshToken, oauthRefreshTokenPrefix) {
		return nil, ErrInvalidGrant
	}

	tokenHash := hashToken(refreshToken)
	data, err := s.refreshCache.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}

	// Checked BEFORE any mutation: a caller presenting the right raw token
	// value but the wrong client_id (or an expired one) must not be able to
	// trigger a rotation attempt at all — doing so would let it tombstone
	// (or expire-delete) the real owner's still-live token as a side effect
	// of a request that was never going to succeed for it, denying service
	// to the legitimate client.
	if data.ClientID != clientID {
		return nil, ErrInvalidGrant
	}
	// Revoking a client stops its existing refresh families at their next
	// rotation, not just new logins. Checked here — after the token has been
	// resolved and bound to this client_id — rather than at the top of the
	// function, so an unauthenticated caller cannot use this endpoint to probe
	// which client_ids are revoked. Collapsed into invalid_grant, the same
	// code every other credential rejection returns.
	if err := s.assertClientUsable(ctx, clientID); err != nil {
		return nil, ErrInvalidGrant
	}
	if time.Now().After(data.ExpiresAt) {
		_ = s.refreshCache.DeleteRefreshToken(ctx, tokenHash)
		return nil, ErrInvalidGrant
	}

	// Re-validate the account on every refresh, mirroring
	// AuthService.RefreshTokenPair (auth_service.go:1779-1802): a banned or
	// deleted user must not keep minting fresh 15-minute access tokens off a
	// refresh token issued before the ban, for the rest of that token
	// family's 30-day life — UserService.UpdateStatus/Delete invalidate an
	// auth cache but never call RevokeAllUserSessions, so nothing else stops
	// this token family on its own.
	user, err := s.userRepo.GetByID(ctx, data.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_ = s.refreshCache.DeleteTokenFamily(ctx, data.FamilyID)
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("load user for refresh: %w", err)
	}
	if !user.IsActive() {
		_ = s.refreshCache.DeleteTokenFamily(ctx, data.FamilyID)
		return nil, ErrInvalidGrant
	}

	// Credential-invalidation check, mirroring auth_service.go:1798-1802 for
	// panel sessions. resolvedTokenVersion is a fingerprint over the user's
	// email + password_hash, so this single comparison covers BOTH an in-app
	// password change and an email change.
	//
	// It is load-bearing rather than belt-and-braces: UserService.ChangePassword
	// writes the new hash and returns without calling RevokeAllUserSessions, so
	// nothing else stops this family. Without this branch the attack is: laptop
	// compromised, ~/.hermes/auth.json exfiltrated, victim changes their
	// password — panel access dies, the agent credential does not, and it
	// self-rotates for the rest of its 30 days with no UI listing OAuth grants
	// to tell them. Killing the whole family (not just this token) matches what
	// the panel path does and means the attacker's in-flight rotations die too.
	if data.TokenVersion != resolvedTokenVersion(user) {
		_ = s.refreshCache.DeleteTokenFamily(ctx, data.FamilyID)
		return nil, ErrInvalidGrant
	}

	// Atomically claim the right to rotate THIS token, before minting
	// anything: a crash between here and the end of this function costs at
	// most the one token being rotated (the client must re-authenticate),
	// never a forked family with two live tokens.
	tomb := *data
	tomb.Rotated = true
	winner, alreadyRotated, err := s.refreshCache.MarkRotated(ctx, tokenHash, &tomb)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			// Deleted between the GetRefreshToken above and here (e.g. a
			// concurrent expiry sweep) — same outward behavior as "never
			// existed".
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("mark refresh token rotated: %w", err)
	}
	if alreadyRotated {
		// REPLAY, or the loser of a concurrent double-presentation of this
		// exact token: someone else's mint may already be complete, or
		// still in flight. Either way, kill the whole family so whatever
		// token that caller receives (or received) also dies — the
		// legitimate client is forced to notice and re-authenticate rather
		// than an attacker silently riding along on a stolen token.
		slog.Warn("oauth: refresh token reuse detected, revoking family", "client_id", clientID)
		if delErr := s.refreshCache.DeleteTokenFamily(ctx, winner.FamilyID); delErr != nil {
			return nil, fmt.Errorf("revoke token family after reuse: %w", delErr)
		}
		return nil, ErrRefreshTokenReused
	}

	// Scope is carried forward unchanged — a refresh must never silently
	// widen (privilege escalation) or narrow (surprise downgrade) it. Uses
	// winner (the value MarkRotated observed atomically), not the earlier
	// data read, though in practice they agree — winner is simply the freshest.
	access, err := s.mintAccessToken(ctx, winner.UserID, clientID, winner.Scope)
	if err != nil {
		return nil, err
	}
	newRefresh, err := s.issueRefreshToken(ctx, winner.UserID, clientID, winner.Scope, winner.FamilyID, resolvedTokenVersion(user))
	if err != nil {
		return nil, err
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: newRefresh,
		Scope:        winner.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}
