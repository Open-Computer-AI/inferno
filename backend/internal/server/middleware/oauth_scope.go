package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// OAuth resource-server context keys, set by RequireOAuthScope and read by
// OAuth-consuming handlers (e.g. OAuthHandler.Account). Plain string
// constants, not the typed ContextKey used by the panel-session path
// (ContextKeyUser et al.) — kept separate deliberately, so a handler can
// never accidentally read a panel-session value off an OAuth-bearer request
// or vice versa; the two token types are not interchangeable (see
// RequireOAuthScope's doc comment).
const (
	OAuthContextKeyUserID   = "oauth_user_id"
	OAuthContextKeyClientID = "oauth_client_id"
	OAuthContextKeyScope    = "oauth_scope"
)

// RequireOAuthScope validates an OAuth-issued ES256 bearer token and enforces
// a required scope, for the OAuth resource-server surface (currently just
// GET /api/oauth/account).
//
// It accepts ES256 ONLY. Inferno's panel session tokens (see jwt_auth.go)
// are HMAC-signed with a symmetric secret that every server process holds;
// if this middleware accepted HMAC too, anyone holding that secret could
// mint API credentials for any agent — which is exactly why Task 2
// introduced a separate asymmetric ES256 keypair for OAuth tokens in the
// first place. Both the algorithm allowlist (jwt.WithValidMethods) and the
// keyFunc's own method-type assertion enforce this — two independent checks
// against algorithm-confusion, mirroring AuthService.ValidateToken's pattern
// for the HMAC side.
//
// required == "" means "any validly-signed, unexpired OAuth token" — no
// specific scope needed, just proof of a real OAuth-issued identity.
func RequireOAuthScope(keySvc *service.OAuthKeyService, required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if raw == "" || keySvc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		key, err := keySvc.Active(c.Request.Context())
		if err != nil {
			slog.Error("oauth: cannot load signing key for scope check", "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		claims := jwt.MapClaims{}
		parser := jwt.NewParser(jwt.WithValidMethods([]string{"ES256"}))
		token, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return &key.Private.PublicKey, nil
		})
		// A bearer credential must never be logged, on any path — success or
		// failure. Only kid (the key that verified it) and client_id
		// (public by design, see oauth_client_service.go) are ever logged
		// below.
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		granted, _ := claims["scope"].(string)
		if !scopeSatisfies(granted, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_scope"})
			return
		}

		// sub is always a decimal user id string — mintAccessToken writes it
		// via strconv.FormatInt (oauth_token_service.go). A token whose sub
		// doesn't parse must be rejected outright, not defaulted to 0: a
		// zero-valued oauth_user_id reaching a handler would read/act on
		// another user's (id 0, or whatever a handler treats as "no user")
		// data instead of failing closed.
		sub, _ := claims["sub"].(string)
		uid, err := strconv.ParseInt(sub, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		aud, _ := claims["aud"].(string)

		c.Set(OAuthContextKeyUserID, uid)
		c.Set(OAuthContextKeyClientID, aud)
		c.Set(OAuthContextKeyScope, granted)
		c.Next()
	}
}

// scopeSatisfies reports whether granted (a whitespace-delimited scope
// string, e.g. "inference billing:read") contains required exactly.
//
// Deliberately exact, whitespace-split equality — NOT strings.Contains.
// "billing:manage" must not be satisfied by a token carrying
// "billing:manage_nothing", and a token carrying only "billing" must not
// satisfy a "billing:manage" requirement. A substring match here would be a
// privilege-escalation bug: any scope name that happens to be a prefix (or
// suffix, or infix) of a more powerful one would silently grant it.
func scopeSatisfies(granted, required string) bool {
	if required == "" {
		return true
	}
	for _, s := range strings.Fields(granted) {
		if s == required {
			return true
		}
	}
	return false
}
