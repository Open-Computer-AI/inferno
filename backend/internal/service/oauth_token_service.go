package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oauthauthorizationcode"
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

// pkceChallengeMatches reports whether verifier hashes (RFC 7636 S256:
// BASE64URL-NOPAD(SHA256(verifier))) to challenge, in constant time.
// subtle.ConstantTimeCompare is used rather than == so that comparing a
// forged code_verifier against the stored challenge does not leak, via
// response timing, how many leading bytes of the hash it happened to get
// right.
func pkceChallengeMatches(verifier, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

// ExchangeAuthorizationCode implements the authorization_code grant (RFC
// 6749 §4.1.3), PKCE-verified (RFC 7636 §4.6). Every rejection returns the
// single sentinel ErrInvalidGrant, deliberately — distinguishing "unknown
// code" from "wrong verifier" from "wrong client" on the wire would let a
// caller enumerate which check a guess failed, the same anti-probing
// reasoning ExchangeRefreshToken and ExchangeDeviceCode already apply to
// their own failure sets.
//
// THE CODE IS CONSUMED — via a status-predicated UPDATE ... WHERE code = ?
// AND status = 'pending', exactly the CAS shape ExchangeDeviceCode already
// uses — BEFORE any of the PKCE/client/redirect_uri/expiry checks below run,
// not after. This is deliberate, not an oversight of "validate first,
// consume on success": the property that actually matters is that no two
// concurrent presentations of the same code can both reach the minting call
// below, and a read-then-validate-then-write ordering is exactly the race
// that took two review rounds to close on the refresh_token path (see
// ExchangeRefreshToken's docs and MarkRotated) — two goroutines could both
// read status="pending", both pass every check, and both mint a token pair
// from one authorization. Claiming the row FIRST closes that window
// unconditionally: at most one caller ever wins the CAS, so at most one
// caller ever reaches mintAccessToken/issueRefreshToken for a given code,
// regardless of what the subsequent checks decide. A side effect of this
// ordering is that a request with a wrong PKCE verifier or mismatched
// client_id also permanently burns the code (issued_token_family stays nil,
// since nothing was minted) — a deliberate fail-safe, not a bug: an
// authorization code is meant to be presented exactly once, and treating a
// failed presentation as "still available, try again with different
// parameters" would hand a guessing attacker unlimited free attempts against
// one code instead of exactly one.
func (s *OAuthTokenService) ExchangeAuthorizationCode(ctx context.Context, clientID, code, redirectURI, codeVerifier string) (*OAuthTokens, error) {
	if clientID == "" || code == "" {
		return nil, ErrInvalidGrant
	}

	affected, err := s.entClient.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.Code(code),
			oauthauthorizationcode.Status(authCodeStatusPending),
		).
		SetStatus(authCodeStatusConsumed).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("consume authorization code: %w", err)
	}
	if affected == 0 {
		return nil, s.handleAuthorizationCodeReplay(ctx, code, clientID)
	}

	row, err := s.entClient.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.Code(code)).
		Only(ctx)
	if err != nil {
		// Vanishingly unlikely: we just won the CAS above, so the row must
		// exist. An internal error, not a credential problem — roll the
		// code back to pending so a retry (the caller minted nothing) can
		// still redeem it.
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, fmt.Errorf("query authorization code after consume: %w", err)
	}

	// Defense in depth: IssueCode never persists anything but "S256" (it
	// rejects "plain" outright), so this should be unreachable, but a
	// redemption path must not silently treat a hypothetical future
	// non-S256 row as S256-verified.
	//
	// Everything from here through the PKCE check below is a CREDENTIAL
	// rejection, not an internal fault — the code is deliberately left
	// `consumed` (no rollback) on every one of these branches. Rolling
	// back here would let a guessing attacker retry the same code with
	// different client_id/redirect_uri/verifier combinations indefinitely;
	// see ExchangeAuthorizationCode's docs above the CAS for the full
	// reasoning. Only genuine infrastructure failures downstream (user
	// lookup, entropy, signing, token persistence) roll back.
	if row.CodeChallengeMethod != "S256" {
		return nil, ErrInvalidGrant
	}
	// Bind check: the code is only redeemable by the exact client_id and
	// exact redirect_uri it was issued to. A code redeemable by a different
	// client, or delivered to a different URI, is the classic authorization
	// code interception vector RFC 6749 §10.5 / RFC 7636 exist to close.
	if row.ClientID != clientID {
		return nil, ErrInvalidGrant
	}
	if row.RedirectURI != redirectURI {
		return nil, ErrInvalidGrant
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, ErrInvalidGrant
	}
	if !pkceChallengeMatches(codeVerifier, row.CodeChallenge) {
		return nil, ErrInvalidGrant
	}
	// A client revoked between /oauth/authorize and this redemption must not
	// get a token, mirroring assertClientUsable's use in the other two
	// grants. Also left un-rolled-back: revocation is itself a deliberate,
	// permanent policy decision, not a transient fault.
	if err := s.assertClientUsable(ctx, clientID); err != nil {
		return nil, ErrInvalidGrant
	}

	// Load the user BEFORE minting, for the same two reasons ExchangeDeviceCode
	// documents: the refresh token must carry the user's CURRENT TokenVersion
	// fingerprint, and an authorization is not a session — a user banned or
	// deleted between the browser redirect and this token call must not
	// receive a 30-day credential.
	user, err := s.userRepo.GetByID(ctx, row.UserID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// The user is genuinely gone — a credential rejection, not an
			// infrastructure fault. No rollback: retrying buys nothing.
			return nil, ErrInvalidGrant
		}
		// A real lookup failure (DB blip, etc.) — internal, not credential.
		// Roll back so a retry (the caller minted nothing) is not forced
		// through a full browser re-login for a fault unrelated to its
		// authorization.
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, fmt.Errorf("load user for authorization code grant: %w", err)
	}
	if !user.IsActive() {
		return nil, ErrInvalidGrant
	}

	familyID, err := newFamilyID()
	if err != nil {
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, fmt.Errorf("family id entropy: %w", err)
	}

	// Record the family on the (already-consumed) row BEFORE minting
	// anything, not after. This is the opposite order from a first draft of
	// this method, and deliberately so: recording a family that then fails
	// to be minted is harmless (DeleteTokenFamily on a family nothing was
	// ever stored under is a no-op), whereas minting first and recording
	// after leaves two live windows in which a replay cannot be revoked —
	// (a) if this recording update itself fails after a successful mint,
	// the resulting 30-day family is unrevocable forever, since
	// handleAuthorizationCodeReplay only revokes what IssuedTokenFamily
	// names; (b) even when it succeeds, a replay landing in the gap between
	// the mint completing and this update committing reads
	// IssuedTokenFamily == nil and skips revocation entirely, defeating RFC
	// 6749 §4.1.2 for that window. Recording first closes both.
	if _, err := s.entClient.OAuthAuthorizationCode.Update().
		Where(oauthauthorizationcode.ID(row.ID)).
		SetIssuedTokenFamily(familyID).
		Save(ctx); err != nil {
		// Nothing has been minted yet — safe to roll back.
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, fmt.Errorf("record issued token family: %w", err)
	}

	access, err := s.mintAccessToken(ctx, row.UserID, clientID, row.Scope)
	if err != nil {
		// The family record above is now orphaned (harmless — see the
		// comment on that update) but nothing was minted or returned to any
		// caller. Roll back so a retry can redeem the same code.
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, err
	}
	refresh, err := s.issueRefreshToken(ctx, row.UserID, clientID, row.Scope, familyID, resolvedTokenVersion(user))
	if err != nil {
		// The access token minted above was never returned to any caller —
		// it is a live but unheld 15-minute JWT, harmless by construction
		// (nothing holds it to present). Roll back so a retry can redeem
		// the same code.
		s.rollbackConsumedCode(ctx, code, clientID)
		return nil, err
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: refresh,
		Scope:        row.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}

// rollbackConsumedCode reverts a just-consumed authorization code back to
// "pending" after an INTERNAL failure downstream of the consuming CAS in
// ExchangeAuthorizationCode — never after a credential-rejecting validation
// check (wrong PKCE verifier, mismatched client/redirect_uri, expiry, a
// revoked client, or a genuinely missing/inactive user — see the call
// sites' comments for exactly which branches call this and which
// deliberately do not).
//
// Safe by construction: only the single goroutine that won the original CAS
// can ever reach one of the internal-error call sites (the CAS itself
// serializes that), so the caller performing the rollback has, by
// definition, minted and returned nothing to anyone — there is no
// concurrent redemption for this rollback to race against. Without it, a
// transient fault with nothing to do with the caller's credential (a
// signing-key store blip, a Redis hiccup) permanently burns the code and
// forces a full browser re-login — worse, an HTTP client's default 5xx
// retry behavior would itself trigger the CAS-affects-zero replay path on
// its very next attempt, since the code is already `consumed`.
//
// Best-effort: a failure here is logged, not propagated — the caller is
// already returning the original internal error, and a rollback that also
// fails just means the code stays burned, exactly the pre-fix behavior.
func (s *OAuthTokenService) rollbackConsumedCode(ctx context.Context, code, clientID string) {
	if _, err := s.entClient.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.Code(code),
			oauthauthorizationcode.Status(authCodeStatusConsumed),
		).
		SetStatus(authCodeStatusPending).
		Save(ctx); err != nil {
		slog.Error("oauth: failed to roll back authorization code after internal error", "client_id", clientID, "error", err)
	}
}

// handleAuthorizationCodeReplay is called when the consuming CAS in
// ExchangeAuthorizationCode affects zero rows: either the code never
// existed, or it did and was already consumed by an earlier presentation.
// RFC 6749 §4.1.2 requires the latter case to revoke whatever that earlier
// presentation minted — a code arriving twice means one of the two arrivals
// is an attacker, and since the server cannot tell which, the safe move is
// to kill the credential family the first arrival walked away with, not
// merely reject the second.
//
// Revocation is gated on clientID matching the family's owner (row.ClientID)
// — the caller presenting the replay is NOT authenticated (public client, no
// secret; this is unavoidable and RFC-consistent for this grant), but a
// client_id it supplies is still worth checking before acting on it: without
// this gate, a registered client B that merely observed client A's leaked
// authorization code (proxy access logs, a Referer header, browser history —
// codes travel in a query string) could revoke A's freshly-minted family by
// replaying it with B's own client_id, a self-inflicted denial-of-service A
// never triggered. Gating on client_id closes that cross-client vector while
// keeping the same unauthenticated-replay posture RFC 6749 requires: the
// wire response is ErrInvalidGrant either way, so this reveals nothing about
// which client_id actually owns the code.
//
// Note what this does NOT revoke: the access token minted by the redemption
// being replayed survives for the rest of its 15-minute TTL regardless — it
// is a stateless, self-verifying JWT with no server-side record to delete
// (DeleteTokenFamily only ever touches Redis-backed refresh-token state).
// That is inherent to how every access token in this codebase works, and is
// the same property ExchangeRefreshToken's own reuse-detection path already
// lives with — not a gap specific to this grant.
func (s *OAuthTokenService) handleAuthorizationCodeReplay(ctx context.Context, code, clientID string) error {
	row, err := s.entClient.OAuthAuthorizationCode.Query().
		Where(oauthauthorizationcode.Code(code)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		// Never existed at all — indistinguishable on the wire from any
		// other invalid_grant.
		return ErrInvalidGrant
	}
	if err != nil {
		return fmt.Errorf("query authorization code after failed consume: %w", err)
	}

	if row.IssuedTokenFamily != nil && row.ClientID == clientID {
		// A token family WAS minted from this code by an earlier
		// presentation, AND the caller presenting this replay is the same
		// client_id the code was bound to at issue — revoke it. Logged
		// (client_id only, never the code or the family id — a family id is
		// as sensitive as the refresh tokens it names) so an operator can
		// see reuse happening, mirroring ExchangeRefreshToken's
		// reuse-detection log line.
		slog.Warn("oauth: authorization code replay detected, revoking issued token family", "client_id", clientID)
		if delErr := s.refreshCache.DeleteTokenFamily(ctx, *row.IssuedTokenFamily); delErr != nil {
			return fmt.Errorf("revoke token family after authorization code replay: %w", delErr)
		}
	}
	// Two cases silently skip revocation, both deliberately:
	//   - IssuedTokenFamily is nil: the row was consumed by an earlier
	//     presentation that itself failed validation (bad PKCE verifier,
	//     mismatched client/redirect_uri, or expiry) before minting
	//     anything — nothing was ever issued, so there is nothing to
	//     revoke.
	//   - IssuedTokenFamily is set but row.ClientID != clientID: a
	//     DIFFERENT client_id than the one the code was bound to is
	//     presenting it — see the client_id-gating note above. The correct
	//     response is still the ordinary ErrInvalidGrant below, not a
	//     distinct error: this must not become an oracle for "does this
	//     code belong to a different client than the one I guessed."

	return ErrInvalidGrant
}
