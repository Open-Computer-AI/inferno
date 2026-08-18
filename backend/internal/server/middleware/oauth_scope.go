package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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
// A failure to resolve the key is NOT automatically "the token is invalid".
// A missing kid header, or a kid that ByKid reports as unknown
// (service.ErrUnknownKid), really is a bad token: 401 invalid_token. But any
// OTHER error out of ByKid — e.g. Postgres unreachable — means this
// middleware could not even check the signature, and folding that into the
// same 401 would make an infrastructure outage look, in logs and metrics,
// identical to "every client's credential is dead". That case is reported
// as 500 server_error instead, logged with the error and the kid (never the
// token). jwt.ParseWithClaims only ever surfaces its own parse/verify error
// from a failing keyFunc, so the underlying cause is captured out of the
// closure into keyResolutionErr and inspected after ParseWithClaims returns.
//
// required == "" means "any validly-signed, unexpired OAuth token" — no
// specific scope needed, just proof of a real OAuth-issued identity.
//
// ISSUER AND AUDIENCE ARE BOTH VERIFIED, and neither was before.
//
// issuer is the same canonicalised string OAuthTokenService stamps into
// `iss` — passed from the token service's own accessor rather than re-read
// from config, so the minting side and the verifying side cannot drift the
// way the two consumers of cfg.Server.FrontendURL did (Task 6, defect #1).
// jwt.WithIssuer makes the claim REQUIRED as well as matched
// (golang-jwt/v5 validator.go passes required=true whenever an expected
// issuer is set), so a token minted before Task 6 with `iss: ""` is
// rejected here rather than accepted as unattributed. An empty issuer is a
// server misconfiguration and every request 500s — the same fail-closed,
// loudly-diagnosable treatment ErrIssuerNotConfigured gives the mint, and
// deliberately not "skip the check when unset", which would make the
// verification vanish exactly when the configuration is wrong.
//
// The audience is the client_id the token was minted for
// (mintAccessToken's doc comment: "so an agent can verify a token was
// minted for it and no other instance"). This middleware then publishes it
// as OAuthContextKeyClientID — the CALLER'S IDENTITY, which handlers act
// on. Before this change it was read with a bare type assertion, so a token
// with no aud claim, or an array-valued one, yielded "" and every handler
// downstream was handed an empty client identity as though that were a
// fact. It is now required to be a single, non-empty string, and it is
// resolved against the client registry: a token whose audience names a
// client that has since been revoked or deleted is refused HERE, at the
// resource server, instead of remaining good for the rest of its 15
// minutes. That is what turns the audience from a decorative claim into
// one that carries a checkable property.
//
// A failure to resolve the audience is split the same way key resolution
// is, and for the same reason: ErrClientNotUsable (or a missing row) is a
// real credential rejection and 401s, while any OTHER error means the check
// could not be performed — a Postgres blip must not be reported to every
// agent in the fleet as "your token is invalid".
func RequireOAuthScope(keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, issuer string, required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		// RFC 6750 §2.1 / RFC 7235 §2.1 make the auth-scheme token
		// case-insensitive, so `bearer <tok>` is a well-formed credential
		// and must not 401. The real client sends "Bearer", so this is
		// interop hygiene — but a resource server that rejects a
		// spec-conformant header is wrong regardless of who is calling it
		// today.
		const bearerPrefix = "Bearer "
		if len(authz) < len(bearerPrefix) || !strings.EqualFold(authz[:len(bearerPrefix)], bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		raw := strings.TrimSpace(authz[len(bearerPrefix):])
		if raw == "" || keySvc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		if issuer == "" || clientSvc == nil {
			// Misconfigured server, not a bad token. Fail closed and say so
			// where an operator will see it.
			slog.Error("oauth: resource-server middleware is missing its issuer or client registry; refusing every token")
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		var (
			keyResolutionErr error
			resolvedKid      string
		)
		claims := jwt.MapClaims{}
		// WithExpirationRequired: golang-jwt/v5 only validates exp when the
		// claim is PRESENT (validator.go's verifyExpiresAt defaults
		// `required=false`) — a validly RS256-signed token that simply omits
		// exp would otherwise be accepted as non-expiring forever. Today's
		// only signer, mintAccessToken (oauth_token_service.go), always sets
		// exp, but that is a fact about one caller, not a property this
		// middleware enforces; a future RS256-signing path that forgets exp
		// must fail closed here, not mint an eternal credential.
		parser := jwt.NewParser(
			jwt.WithValidMethods([]string{"RS256"}),
			jwt.WithExpirationRequired(),
			jwt.WithIssuer(issuer),
		)
		token, err := parser.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, errors.New("token has no kid")
			}
			resolvedKid = kid
			key, err := keySvc.ByKid(c.Request.Context(), kid)
			if err != nil {
				if !errors.Is(err, service.ErrUnknownKid) {
					// Not "this kid doesn't exist" — a real failure to reach
					// the key store. Captured for the 500 branch below;
					// still returned here so ParseWithClaims fails closed.
					keyResolutionErr = err
				}
				return nil, err
			}
			return &key.Private.PublicKey, nil
		})
		if keyResolutionErr != nil {
			slog.Error("oauth: cannot resolve signing key for scope check", "error", keyResolutionErr, "kid", resolvedKid)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
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
		// Single-valued, non-empty aud only. `aud` is permitted by RFC 7519
		// to be an array, and mintAccessToken never writes one — a token
		// carrying an array here did not come from this server's mint, and
		// the bare type assertion that used to read it would have quietly
		// produced "" and handed that on as the caller's client identity.
		aud, ok := claims["aud"].(string)
		if !ok || aud == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}
		if _, err := clientSvc.UsableByClientID(c.Request.Context(), aud); err != nil {
			if !errors.Is(err, service.ErrClientNotUsable) && !dbent.IsNotFound(err) {
				// Could not check — not "the check failed". Reporting this
				// as invalid_token would make an outage indistinguishable
				// from every agent's credential dying at once.
				slog.Error("oauth: cannot resolve token audience for scope check", "error", err, "client_id", aud)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

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
