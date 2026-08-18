package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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

	// ErrIssuerNotConfigured is returned by mintAccessToken when this
	// service was constructed with an empty issuer (cfg.Server.FrontendURL
	// unset or blank). It is the authorization_code/refresh_token analogue
	// of OAuthDeviceService.ErrPortalNotConfigured, and exists because the
	// two grants disagreed about that one missing setting: the device grant
	// refused loudly at RequestCode, while this one happily minted tokens
	// carrying `iss: ""`.
	//
	// Found by Task 6's conformance run against the real client. An empty
	// `iss` is not a cosmetic defect: the gateway plugin verifies with
	// PyJWT's `issuer=` check (plugins/dashboard_auth/nous/__init__.py's
	// _verify_jwt, read-only client repo), so every token minted under this
	// misconfiguration is rejected — but rejected as "Invalid issuer" on
	// the CLIENT, with nothing at all in OUR logs, which points an operator
	// at the agent's HERMES_DASHBOARD_PORTAL_URL rather than at the server
	// setting that is actually wrong. Failing the mint instead puts the
	// diagnosis where the misconfiguration is: the handler's default arm
	// logs it and answers server_error.
	//
	// No handler maps this to a specific OAuth error code on purpose — it
	// is an operator misconfiguration, not anything the client did, so it
	// belongs on the logged 500 that every unmatched error already takes.
	ErrIssuerNotConfigured = errors.New("oauth token service: issuer (server.frontend_url) not configured")
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

	// rollbackConsumedCodeTimeout bounds rollbackConsumedCode's detached,
	// best-effort UPDATE — long enough for an ordinary write under load,
	// short enough that a rollback attempt against a genuinely wedged
	// database does not linger indefinitely after the request that
	// triggered it is long gone. 2s, not longer: this runs on the
	// synchronous request path (every rollback call site blocks its
	// goroutine on this call), so during a database outage every failed
	// redemption would otherwise hold a request goroutine and a pool
	// connection for the full timeout. Matches the precedent this idiom was
	// borrowed from, accountRepository.syncSchedulerAccountSnapshotDetached
	// (internal/repository/account_repo.go), which uses the same 2s bound
	// for the same reason.
	rollbackConsumedCodeTimeout = 2 * time.Second

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
	// now is this service's only clock. Injected as a field rather than read
	// from the wall clock at each site so the refresh-reuse grace window
	// (RefreshReuseGracePeriod) can be crossed in tests deterministically,
	// by moving the clock rather than by sleeping through a real minute.
	// Production never replaces it.
	now func() time.Time
}

// NewOAuthTokenService constructs the token-endpoint service. issuer is the
// "iss" claim on minted access tokens — pass the server's own public base
// URL (the JWKS lives at {issuer}/.well-known/jwks.json).
//
// The issuer is normalised here — trimmed of surrounding whitespace and of
// any trailing "/" — for the same reason and in the same way
// NewOAuthDeviceService normalises the identical cfg.Server.FrontendURL
// value it is handed (strings.TrimRight(portalBaseURL, "/")). Until Task
// 6's conformance run this constructor stored the raw string, and the two
// consumers of one config value therefore disagreed about its canonical
// form. That is not a style nit: `SERVER_FRONTEND_URL=https://portal/`
// passes config validation (internal/config/config.go only rejects a
// query string or userinfo), is the exact shape a browser address bar
// yields on copy/paste, and produced `iss: "https://portal/"` — which the
// real gateway plugin rejects on EVERY token, because it verifies with
// PyJWT's `issuer=` against a portal_url it has already rstrip("/")-ed
// (plugins/dashboard_auth/nous/__init__.py, read-only client repo). One
// trailing slash silently broke the entire dashboard grant, and no unit
// test saw it because every test passes an already-canonical issuer.
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
		issuer:       strings.TrimRight(strings.TrimSpace(issuer), "/"),
		now:          time.Now,
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
	// Refuse rather than mint an unverifiable token. See
	// ErrIssuerNotConfigured for why a blank "iss" is a hard failure and
	// not a tolerable default, and why this check lives here (covering all
	// three grants at the single point that stamps the claim) rather than
	// in each grant's entry point.
	if s.issuer == "" {
		return "", ErrIssuerNotConfigured
	}

	key, err := s.keySvc.Active(ctx)
	if err != nil {
		return "", err
	}

	now := s.now()
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
	// No predecessor: this is the root of a new family, issued by the
	// device_code or authorization_code grant.
	prepared, err := s.prepareRefreshToken(userID, clientID, scope, familyID, tokenVersion, "")
	if err != nil {
		return "", err
	}
	if err := s.persistRefreshToken(ctx, prepared); err != nil {
		return "", err
	}
	return prepared.raw, nil
}

// preparedRefreshToken is a refresh token that exists but has not been
// persisted yet. The split exists for ExchangeRefreshToken, which must have
// the successor token IN HAND before it claims the rotation (the claim
// stores the successor pair for the grace window) but must not have STORED
// it, so that a caller which loses the claim leaves no second live token
// behind — the family fork MarkRotated exists to prevent.
type preparedRefreshToken struct {
	raw  string
	hash string
	data *RefreshTokenData
}

// prepareRefreshToken mints a raw refresh token and the record that would
// describe it, without writing anything anywhere.
// predecessorHash is the hash of the token being rotated, or "" for the root
// of a family. It is what later lets that predecessor's grace window be
// closed the moment THIS token is itself rotated.
func (s *OAuthTokenService) prepareRefreshToken(userID int64, clientID, scope, familyID string, tokenVersion int64, predecessorHash string) (*preparedRefreshToken, error) {
	raw, err := newOAuthRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("refresh token entropy: %w", err)
	}
	now := s.now()
	return &preparedRefreshToken{
		raw:  raw,
		hash: hashToken(raw),
		data: &RefreshTokenData{
			UserID:          userID,
			TokenVersion:    tokenVersion,
			FamilyID:        familyID,
			ClientID:        clientID,
			Scope:           scope,
			CreatedAt:       now,
			ExpiresAt:       now.Add(oauthRefreshTokenTTL),
			PredecessorHash: predecessorHash,
		},
	}, nil
}

// persistRefreshToken writes a prepared token: only its SHA256 hash (via
// hashToken, shared with the panel-session refresh flow in auth_service.go)
// reaches Redis as a key, never the raw value.
func (s *OAuthTokenService) persistRefreshToken(ctx context.Context, p *preparedRefreshToken) error {
	if err := s.refreshCache.StoreRefreshToken(ctx, p.hash, p.data, oauthRefreshTokenTTL); err != nil {
		return fmt.Errorf("store refresh token: %w", err)
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
	// email+password_hash. The TokenVersion stamped in prepareRefreshToken,
	// and the matching comparison in ExchangeRefreshToken, is what gives the
	// OAuth path the same property — without it a stolen ~/.hermes/auth.json
	// kept self-rotating for its full 30 days after the victim changed their
	// password, with no UI anywhere listing OAuth grants to tell them.
	if err := s.refreshCache.AddToUserTokenSet(ctx, p.data.UserID, p.hash, oauthRefreshTokenTTL); err != nil {
		return fmt.Errorf("add refresh token to user set: %w", err)
	}
	if err := s.refreshCache.AddToFamilyTokenSet(ctx, p.data.FamilyID, p.hash, oauthRefreshTokenTTL); err != nil {
		return fmt.Errorf("add refresh token to family: %w", err)
	}
	return nil
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

	if s.now().After(row.ExpiresAt) {
		return nil, ErrExpiredToken
	}

	switch row.Status {
	case "pending":
		// Rate-limit the poll loop: a client polling faster than the
		// advertised interval gets slow_down rather than a free retry. Only
		// a poll that is NOT rejected as slow_down advances LastPolledAt —
		// resetting it on every attempt would let a client evade the
		// backoff by polling continuously.
		// s.now(), not time.Since: LastPolledAt is WRITTEN with s.now() a few
		// lines below, so reading it against the wall clock would compare two
		// different clocks the moment a test freezes one of them.
		if row.LastPolledAt != nil && s.now().Sub(*row.LastPolledAt) < time.Duration(devicePollInterval)*time.Second {
			return nil, ErrSlowDown
		}
		if _, uerr := row.Update().SetLastPolledAt(s.now()).Save(ctx); uerr != nil {
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
// (RefreshTokenCache.MarkRotated), not a Go-side read-then-write. The
// successor pair is minted just before that call — see the comment at the
// mint site for why it has to be, and why minting early is not the same as
// committing early — but nothing is STORED until the claim has been won. Two concurrent presentations of the exact same
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
//
// Not every replay is an attack, so there is a 60-second grace
// (RefreshReuseGracePeriod): a replay of a token rotated less than a minute
// ago is answered with the pair that rotation minted — the SAME pair,
// verbatim — instead of revoking the family. Without it a desktop agent
// retrying a refresh whose response was lost in flight, or two windows of
// one session refreshing together, would kill a healthy session; Portal
// documents the same window and the hermes client is written against it.
//
// The grace is kept as narrow as it can be, because every widening of it is
// a narrowing of reuse detection, which is the only reason rotation exists.
// Forgiveness requires BOTH of:
//
//   - the token's own rotation was less than RefreshReuseGracePeriod ago, and
//   - the successor that rotation minted is still the family head.
//
// The second condition is not implied by the first. A family that rotates
// twice inside a minute leaves an ancestor whose own window is still open
// but which is no longer the immediately previous token, and forgiving it
// would hand a thief the intermediate token to walk forward to the live one.
// So a claim closes its predecessor's window in the same atomic operation
// that makes it the head — see RefreshReplaySupersededMarker and
// TestSupersededAncestorIsNotForgiven.
//
// Beyond that: a forgiven replay returns the already-minted pair rather than
// minting another (two live tokens in one family is the fork that
// permanently disables the detector), and it writes nothing at all — so the
// window never restarts and never extends. Anything else revokes the family
// exactly as before.
//
// The window is decided inside MarkRotated's single atomic operation, not
// here: reading a rotation timestamp in one round trip and deciding on it in
// a second is the same read-then-write split this function's whole design
// exists to avoid.
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
	if s.now().After(data.ExpiresAt) {
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

	// Mint the successor pair BEFORE claiming the rotation — but store
	// nothing yet, and only when a claim is actually possible.
	//
	// The claim below has to carry the pair with it: a replay that lands
	// inside the grace window is answered with the pair the rotation already
	// minted, so that pair must be written by the SAME atomic operation that
	// writes the tombstone. Were it written afterwards, a second
	// presentation arriving in the gap would find a rotated record with no
	// pair to hand back and would revoke the family — precisely the
	// healthy-session kill this grace exists to prevent, and precisely the
	// two-window / lost-response case it is aimed at.
	//
	// Minting early is not the same as committing early: prepareRefreshToken
	// writes nothing, so a caller that goes on to LOSE the claim leaves no
	// second live token behind and the family cannot fork. Only the winner
	// reaches persistRefreshToken. A crash between the claim and that
	// persist costs the one token being rotated (the client must
	// re-authenticate), the same worst case as before this change.
	//
	// Scope, user and family are carried forward from data unchanged — a
	// refresh must never silently widen (privilege escalation) or narrow
	// (surprise downgrade) scope. data is the pre-rotation record, and on
	// the winning path it is necessarily identical to what MarkRotated
	// observed: a non-rotated record is only ever written by a rotation, and
	// there can be only one of those.
	now := s.now()

	// A record that is ALREADY rotated can only produce a replay verdict —
	// records never un-rotate — so there is nothing to mint for it, and
	// minting anyway would put the signing key on the path of every
	// presentation of a stolen token. If keySvc.Active were unavailable
	// (DB blip, key rotation window), reuse detection would return 500 with
	// the family unrevoked and nothing logged, and an attacker could simply
	// retry later. Theft response must not depend on the availability of the
	// minting path, so this branch skips it entirely; MarkRotated refuses to
	// claim without a pair, so the skip cannot turn into a silent claim.
	var (
		access    string
		successor *preparedRefreshToken
		sealed    []byte
	)
	if !data.Rotated {
		access, err = s.mintAccessToken(ctx, data.UserID, clientID, data.Scope)
		if err != nil {
			return nil, err
		}
		successor, err = s.prepareRefreshToken(data.UserID, clientID, data.Scope, data.FamilyID, resolvedTokenVersion(user), tokenHash)
		if err != nil {
			return nil, err
		}
		// Sealed under the token being rotated, so the cache stores
		// ciphertext rather than a live token pair — see sealRefreshReplay
		// for why the presented token is the right key.
		sealed, err = sealRefreshReplay(refreshToken, &RefreshReplayPair{
			AccessToken:         access,
			RefreshToken:        successor.raw,
			Scope:               data.Scope,
			AccessExpiresAtUnix: now.Add(oauthAccessTokenTTL).Unix(),
		})
		if err != nil {
			return nil, err
		}
	}

	tomb := *data
	tomb.Rotated = true
	tomb.RotatedAtUnix = now.Unix()

	// The single serialization point: exactly one caller is told it claimed
	// the rotation, and every other caller is told, in the same round trip,
	// whether it is a forgivable replay or reuse.
	res, err := s.refreshCache.MarkRotated(ctx, tokenHash, &tomb, sealed, now)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			// Deleted between the GetRefreshToken above and here (e.g. a
			// concurrent expiry sweep) — same outward behavior as "never
			// existed".
			return nil, ErrInvalidGrant
		}
		return nil, fmt.Errorf("mark refresh token rotated: %w", err)
	}

	switch res.Outcome {
	case RefreshRotationClaimed:
		if successor == nil {
			// Unreachable: the script refuses to claim without a pair, and a
			// pair only exists when a successor was prepared. Guarded so a
			// future reordering cannot silently return an empty token.
			return nil, fmt.Errorf("mark refresh token rotated: claimed a rotation with no prepared successor")
		}
		// Fall through to the persist below.
	case RefreshRotationGraceReplay:
		// A replay of a token rotated less than RefreshReuseGracePeriod ago:
		// an agent retrying a refresh whose response never arrived, or a
		// second window of the same session refreshing at the same time.
		// Hand back the pair that rotation minted — the SAME pair, not a new
		// one. Minting again here would put two live tokens in one family,
		// which is the fork that permanently disables reuse detection; and
		// revoking here would kill a session that did nothing wrong.
		//
		// Nothing is written on this path, so the grace neither restarts nor
		// extends: it stays anchored to the one rotation that opened it.
		if len(res.SealedReplay) == 0 {
			// Only reachable from an implementation that breaks
			// MarkRotated's contract. An infrastructure fault, so it must
			// surface as a 500 rather than as an invalid_grant that would
			// blame the client.
			return nil, fmt.Errorf("mark refresh token rotated: grace replay without a stored token pair")
		}
		// The key is derived from the token this caller just presented, so
		// this cannot fail for a well-formed request; a failure is
		// corruption, and it is an internal error rather than a rejection.
		pair, err := openRefreshReplay(refreshToken, res.SealedReplay)
		if err != nil {
			return nil, err
		}
		slog.Info("oauth: refresh token replayed inside reuse grace, returning current pair", "client_id", clientID)
		return &OAuthTokens{
			AccessToken:  pair.AccessToken,
			RefreshToken: pair.RefreshToken,
			Scope:        pair.Scope,
			// The access token was minted when the rotation happened, so
			// what is left of its 15 minutes is less than a full TTL.
			// Reporting the full TTL would leave a client holding a token it
			// believes is live for longer than it is.
			ExpiresIn: remainingSeconds(pair.AccessExpiresAtUnix, now),
		}, nil
	case RefreshRotationGraceUnavailable:
		// Inside the window, the chain has not moved on, and yet the sealed
		// pair is gone. The slot's TTL is strictly longer than the window,
		// so this is not expiry — it is eviction or corruption. Everything
		// observable about this presentation says benign, so it must NOT be
		// treated as theft: revoking a family because a cache entry
		// disappeared would kill a healthy session over an infrastructure
		// event. Surfaces as a 500, which is what it is.
		return nil, fmt.Errorf("refresh token replay inside the reuse grace, but the sealed pair is no longer in cache")
	case RefreshRotationReuse:
		// REPLAY outside the grace: someone else's mint completed more than
		// a minute ago, and this presentation is either an attacker with a
		// stolen token or a client that has fallen far enough behind to be
		// indistinguishable from one. Kill the whole family so whatever
		// token the earlier caller received also dies — the legitimate
		// client is forced to notice and re-authenticate rather than an
		// attacker silently riding along on a stolen token.
		slog.Warn("oauth: refresh token reuse detected, revoking family", "client_id", clientID)
		if delErr := s.refreshCache.DeleteTokenFamily(ctx, res.Data.FamilyID); delErr != nil {
			return nil, fmt.Errorf("revoke token family after reuse: %w", delErr)
		}
		return nil, ErrRefreshTokenReused
	default:
		// Includes RefreshRotationUnknown, the zero value: an unpopulated
		// result must never be read as permission to mint.
		return nil, fmt.Errorf("mark refresh token rotated: unexpected rotation outcome %d", res.Outcome)
	}

	if err := s.persistRefreshToken(ctx, successor); err != nil {
		return nil, err
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: successor.raw,
		Scope:        data.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}

// refreshReplaySealInfo domain-separates the replay-sealing key from any
// other key that might ever be derived from a refresh token. Versioned: if
// the sealed format changes, bump it and old blobs stop opening (they are
// worthless after 120s anyway).
const refreshReplaySealInfo = "inferno/oauth/refresh-replay/v1"

// sealRefreshReplay encrypts the pair a rotation minted, under a key derived
// from the refresh token being rotated.
//
// The point is that RefreshTokenCache — and anyone who can read it — must not
// hold a usable refresh token. Everything else in this system is stored as a
// SHA256 hash for exactly that reason, and the grace window would have
// punched a hole in it: to answer a replay with the same pair, that pair has
// to be persisted somewhere.
//
// The key is the predecessor token itself. That works because the only party
// entitled to receive the successor is the party that can present the
// predecessor, and that party is necessarily holding the predecessor's raw
// value at the moment it asks — it just sent it. So the decryption key is in
// hand exactly when it is needed and at no other time. A Redis-only
// compromise yields a hash and a ciphertext, which is precisely what it
// yielded before the grace existed.
//
// Deriving a key from a token is only sound because an OAuth refresh token
// here is 32 bytes of crypto/rand rendered as hex (newOAuthRefreshToken) —
// uniform, unguessable, and not a structured JWT. HKDF-SHA256 with a fixed
// info string turns that into a 32-byte AES key; no salt, because the input
// is already a uniformly random secret and a salt would have to be stored
// alongside the ciphertext anyway.
//
// No additional authenticated data is bound in: the key is already
// one-to-one with the record's hash, so a blob moved to a different key by
// an attacker with Redis write access simply fails to open.
func sealRefreshReplay(rawRefreshToken string, pair *RefreshReplayPair) ([]byte, error) {
	plaintext, err := json.Marshal(pair)
	if err != nil {
		return nil, fmt.Errorf("marshal refresh replay pair: %w", err)
	}
	gcm, err := refreshReplayAEAD(rawRefreshToken)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("refresh replay nonce entropy: %w", err)
	}
	// Seal appends to its first argument, so the nonce ends up prefixed to
	// the ciphertext: nonce || ciphertext || tag.
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// openRefreshReplay reverses sealRefreshReplay. A failure here is never the
// presenting client's fault — the key is derived from the very token whose
// hash selected this record, so the only ways to land here are corruption or
// a format change. Callers must surface it as an internal error, not as an
// OAuth rejection.
func openRefreshReplay(rawRefreshToken string, sealed []byte) (*RefreshReplayPair, error) {
	gcm, err := refreshReplayAEAD(rawRefreshToken)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, fmt.Errorf("sealed refresh replay pair is too short (%d bytes)", len(sealed))
	}
	nonce, body := sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("open sealed refresh replay pair: %w", err)
	}
	var pair RefreshReplayPair
	if err := json.Unmarshal(plaintext, &pair); err != nil {
		return nil, fmt.Errorf("unmarshal refresh replay pair: %w", err)
	}
	return &pair, nil
}

func refreshReplayAEAD(rawRefreshToken string) (cipher.AEAD, error) {
	key, err := hkdf.Key(sha256.New, []byte(rawRefreshToken), nil, refreshReplaySealInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("derive refresh replay key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("refresh replay cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("refresh replay GCM: %w", err)
	}
	return gcm, nil
}

// remainingSeconds reports how many whole seconds are left until a unix
// timestamp, floored at zero so an already-expired stamp never reports a
// negative expires_in.
func remainingSeconds(deadlineUnix int64, now time.Time) int {
	remaining := deadlineUnix - now.Unix()
	if remaining < 0 {
		return 0
	}
	return int(remaining)
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
	if s.now().After(row.ExpiresAt) {
		return nil, ErrInvalidGrant
	}
	if !pkceChallengeMatches(codeVerifier, row.CodeChallenge) {
		return nil, ErrInvalidGrant
	}
	// A client revoked between /oauth/authorize and this redemption must not
	// get a token, mirroring assertClientUsable's use in the other two
	// grants.
	//
	// The two failure shapes here are NOT the same, and must not be treated
	// the same. ErrClientNotUsable is a genuine, deliberate, permanent
	// policy decision (the client was revoked) — left un-rolled-back, same
	// as every other credential rejection above. Anything else reaching
	// this branch is NOT that: assertClientUsable's call chain bottoms out
	// in entClient.OAuthClient.Query().Only(ctx), which returns a raw ent
	// error on ANY database fault (a transient blip, a connection-pool
	// exhaustion, ...), not just "not found". Folding that into
	// ErrInvalidGrant without rolling back is exactly the I1 failure mode
	// this method was fixed for once already, one call site over: a
	// transient backend fault burns the code and forces a full browser
	// re-login for a problem that had nothing to do with the caller's
	// credential.
	if err := s.assertClientUsable(ctx, clientID); err != nil {
		if !errors.Is(err, ErrClientNotUsable) {
			s.rollbackConsumedCode(ctx, code, clientID)
			return nil, fmt.Errorf("check client usability for authorization code grant: %w", err)
		}
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
//
// Runs on a DETACHED context, not the caller's ctx — deliberately. The
// caller's ctx is the very thing that was failing (or, for the handler's
// entry point, c.Request.Context(), which is CANCELED on client disconnect
// or a request timeout: see oauth_handler.go's Token). A client that hung
// up or timed out is exactly the scenario this rollback exists to recover
// from, so issuing the rollback's UPDATE on that same, already-canceling
// context would make it fail immediately with "context canceled" — the fix
// silently not firing for precisely the fault class that motivated it.
// context.WithoutCancel preserves any request-scoped values (for tracing/
// logging) while detaching cancellation and deadline; rollbackTimeout then
// bounds how long this best-effort write is allowed to run on its own,
// mirroring the same idiom already used for other post-response,
// caller-context-outlives writes in this codebase (e.g.
// accountRepository.syncSchedulerAccountSnapshotDetached,
// systemUpdateContext).
func (s *OAuthTokenService) rollbackConsumedCode(ctx context.Context, code, clientID string) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	rollbackCtx, cancel := context.WithTimeout(base, rollbackConsumedCodeTimeout)
	defer cancel()

	// ClearIssuedTokenFamily undoes the family-recording update the caller
	// may have already made before hitting whatever internal failure
	// triggered this rollback (see ExchangeAuthorizationCode's
	// family-record-before-mint ordering). Leaving a stale family on a row
	// that is once again "pending" is harmless in itself — a later
	// successful redemption of the same code overwrites it with a fresh
	// family — but a replay landing in the narrow window before that retry
	// would otherwise read a non-nil IssuedTokenFamily, log "replay
	// detected, revoking issued token family", and call DeleteTokenFamily
	// on a family nothing was ever actually stored under: a misleading
	// operator-facing signal for an event that didn't happen. Clearing it
	// here removes that false signal entirely.
	if _, err := s.entClient.OAuthAuthorizationCode.Update().
		Where(
			oauthauthorizationcode.Code(code),
			oauthauthorizationcode.Status(authCodeStatusConsumed),
		).
		SetStatus(authCodeStatusPending).
		ClearIssuedTokenFamily().
		Save(rollbackCtx); err != nil {
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
