package middleware

import (
	"errors"
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

// RequireOAuthScope validates an OAuth-issued RS256 bearer token and enforces
// a required scope, for the OAuth resource-server surface (currently just
// GET /api/oauth/account).
//
// It accepts RS256 ONLY — the gateway-brokered flow the Hermes Desktop
// client actually uses hard-codes algorithms=["RS256"] and rejects anything
// else outright, so the signing key moved from ES256 to RS256 rather than
// the constrained consumer changing. Inferno's panel session tokens (see
// jwt_auth.go) are HMAC-signed with a symmetric secret that every server
// process holds; if this middleware accepted HMAC too, anyone holding that
// secret could mint API credentials for any agent — which is exactly why a
// separate asymmetric keypair was introduced for OAuth tokens in the first
// place. Both the algorithm allowlist (jwt.WithValidMethods) and the
// verification key resolving strictly by the token's own kid header (never
// falling back to "the" active key) enforce this — two independent checks
// against algorithm/key confusion, mirroring AuthService.ValidateToken's
// pattern for the HMAC side.
//
// The verification key is resolved by ByKid — never Active() directly — so
// a token with no kid header, or a kid nobody recognizes, is rejected
// outright rather than silently falling back to whatever happens to be the
// active key. This is also what makes key rotation possible later: adding a
// second key only ever requires a change inside ByKid, never here.
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

		claims := jwt.MapClaims{}
		// WithExpirationRequired: golang-jwt/v5 only validates exp when the
		// claim is PRESENT (validator.go's verifyExpiresAt defaults
		// `required=false`) — a validly RS256-signed token that simply omits
		// exp would otherwise be accepted as non-expiring forever. Today's
		// only signer, mintAccessToken (oauth_token_service.go), always sets
		// exp, but that is a fact about one caller, not a property this
		// middleware enforces; a future RS256-signing path that forgets exp
		// must fail closed here, not mint an eternal credential.
		parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithExpirationRequired())
		token, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("token has no kid")
			}
			key, err := keySvc.ByKid(c.Request.Context(), kid)
			if err != nil {
				return nil, err
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
