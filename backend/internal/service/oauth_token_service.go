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
	// oauthAccessTokenTTL bounds how long an ES256 access token is valid.
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

// OAuthTokenService implements the device_code and refresh_token grants of
// POST /api/oauth/token. It mints access tokens itself — ES256, signed by
// OAuthKeyService's key — and deliberately does NOT call
// AuthService.GenerateTokenPair, which signs Inferno's separate HMAC panel
// session token. Task 6's resource-server middleware accepts ES256 only;
// an HMAC-signed token here would be silently rejected two tasks later.
//
// Refresh tokens ARE persisted through the same RefreshTokenCache used by
// panel sessions (Redis-backed, SHA256-hashed at rest) rather than a
// parallel store, so the reuse-detection machinery (DeleteTokenFamily)
// stays in one place.
type OAuthTokenService struct {
	entClient    *dbent.Client
	keySvc       *OAuthKeyService
	deviceSvc    *OAuthDeviceService
	refreshCache RefreshTokenCache
	issuer       string
}

// NewOAuthTokenService constructs the token-endpoint service. issuer is the
// "iss" claim on minted access tokens — pass the server's own public base
// URL (the JWKS lives at {issuer}/.well-known/jwks.json).
func NewOAuthTokenService(entClient *dbent.Client, keySvc *OAuthKeyService, deviceSvc *OAuthDeviceService, refreshCache RefreshTokenCache, issuer string) *OAuthTokenService {
	return &OAuthTokenService{
		entClient:    entClient,
		keySvc:       keySvc,
		deviceSvc:    deviceSvc,
		refreshCache: refreshCache,
		issuer:       issuer,
	}
}

// mintAccessToken signs an ES256 JWT whose audience is the client_id, so an
// agent can verify a token was minted for it and no other instance.
func (s *OAuthTokenService) mintAccessToken(ctx context.Context, userID int64, clientID, scope string) (string, error) {
	key, err := s.keySvc.Active(ctx)
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   s.issuer,
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   clientID,
		"scope": scope,
		"iat":   now.Unix(),
		"exp":   now.Add(oauthAccessTokenTTL).Unix(),
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
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
func (s *OAuthTokenService) issueRefreshToken(ctx context.Context, userID int64, clientID, scope, familyID string) (string, error) {
	raw, err := newOAuthRefreshToken()
	if err != nil {
		return "", fmt.Errorf("refresh token entropy: %w", err)
	}
	tokenHash := hashToken(raw)

	now := time.Now()
	data := &RefreshTokenData{
		UserID:    userID,
		FamilyID:  familyID,
		ClientID:  clientID,
		Scope:     scope,
		CreatedAt: now,
		ExpiresAt: now.Add(oauthRefreshTokenTTL),
	}

	if err := s.refreshCache.StoreRefreshToken(ctx, tokenHash, data, oauthRefreshTokenTTL); err != nil {
		return "", fmt.Errorf("store refresh token: %w", err)
	}
	// Shares the user's token set with panel sessions: a password-change /
	// logout-all-devices revocation (AuthService.RevokeAllUserSessions) kills
	// agent-issued OAuth sessions too, not just browser sessions.
	if err := s.refreshCache.AddToUserTokenSet(ctx, userID, tokenHash, oauthRefreshTokenTTL); err != nil {
		slog.Warn("oauth: failed to add refresh token to user set", "client_id", clientID, "error", err)
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
	refresh, err := s.issueRefreshToken(ctx, userID, clientID, row.Scope, familyID)
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
// On success the presented token is NOT deleted — it is re-stored under the
// same hash with Rotated set, for the remainder of its original lifetime.
// This is deliberate: a hard delete would make a later replay of that exact
// raw token indistinguishable from "never existed", which loses the
// FamilyID needed to revoke the rest of the session. Keeping a tombstone is
// what makes the reuse branch below possible at all.
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

	if data.ClientID != clientID {
		return nil, ErrInvalidGrant
	}

	if time.Now().After(data.ExpiresAt) {
		_ = s.refreshCache.DeleteRefreshToken(ctx, tokenHash)
		return nil, ErrInvalidGrant
	}

	if data.Rotated {
		// REPLAY: this exact token was already redeemed once. The
		// legitimate client has a newer token from that rotation, in the
		// same family — kill the whole family so that token dies too and
		// the legitimate client is forced to notice and re-authenticate,
		// rather than an attacker silently riding along on a stolen token.
		slog.Warn("oauth: refresh token reuse detected, revoking family", "client_id", clientID)
		if delErr := s.refreshCache.DeleteTokenFamily(ctx, data.FamilyID); delErr != nil {
			return nil, fmt.Errorf("revoke token family after reuse: %w", delErr)
		}
		return nil, ErrRefreshTokenReused
	}

	// Scope is carried forward unchanged — a refresh must never silently
	// widen (privilege escalation) or narrow (surprise downgrade) it.
	access, err := s.mintAccessToken(ctx, data.UserID, clientID, data.Scope)
	if err != nil {
		return nil, err
	}
	newRefresh, err := s.issueRefreshToken(ctx, data.UserID, clientID, data.Scope, data.FamilyID)
	if err != nil {
		return nil, err
	}

	remaining := time.Until(data.ExpiresAt)
	if remaining > 0 {
		tomb := *data
		tomb.Rotated = true
		if err := s.refreshCache.StoreRefreshToken(ctx, tokenHash, &tomb, remaining); err != nil {
			return nil, fmt.Errorf("tombstone rotated refresh token: %w", err)
		}
	} else {
		_ = s.refreshCache.DeleteRefreshToken(ctx, tokenHash)
	}

	return &OAuthTokens{
		AccessToken:  access,
		RefreshToken: newRefresh,
		Scope:        data.Scope,
		ExpiresIn:    int(oauthAccessTokenTTL.Seconds()),
	}, nil
}
