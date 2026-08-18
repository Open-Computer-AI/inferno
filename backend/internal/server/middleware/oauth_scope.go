package middleware

import (
	"context"
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
// VerifyOAuthBearer's doc comment).
const (
	OAuthContextKeyUserID   = "oauth_user_id"
	OAuthContextKeyClientID = "oauth_client_id"
	OAuthContextKeyScope    = "oauth_scope"
)

// OAuthBearer is the identity carried by a successfully verified OAuth
// access token: the caller (UserID), the client it was minted for
// (ClientID, the token's audience), and the whitespace-delimited scope
// string it was granted (Scope). Returned by VerifyOAuthBearer.
type OAuthBearer struct {
	UserID   int64
	ClientID string
	Scope    string
}

// ErrOAuthInvalidToken means the credential itself is bad: unparseable,
// wrongly signed, expired, missing a required claim, or naming an audience
// that is not (or no longer) a usable client. Callers map this to
// 401 {"error":"invalid_token"}.
var ErrOAuthInvalidToken = errors.New("oauth: invalid token")

// ErrOAuthServerFault means verification could not be completed — the
// signing-key store or the client registry was unreachable — and says
// nothing about whether the token itself is good or bad. Callers map this
// to 500 {"error":"server_error"}, never to ErrOAuthInvalidToken: folding
// an infrastructure outage into the same response as a bad credential would
// make a Postgres blip indistinguishable, in logs and metrics, from every
// agent's credential dying at once.
var ErrOAuthServerFault = errors.New("oauth: cannot verify token")

// VerifyOAuthBearer validates rawToken as an OAuth-issued RS256 bearer
// token and, if it is good, returns the identity it carries. It is the one
// verifier for OAuth access tokens in this server — every resource-server
// surface that accepts an OAuth bearer (the panel-session-parallel
// GET /api/oauth/account path via RequireOAuthScope today, and any future
// caller) must go through this function rather than growing its own JWT
// parsing, so the properties documented below are enforced exactly once.
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
//
// Scope is deliberately NOT checked here — VerifyOAuthBearer proves who the
// caller is, not what they may do. Returning the granted scope on
// OAuthBearer lets each caller (RequireOAuthScope today) enforce whatever
// requirement is appropriate for its own surface via scopeSatisfies.
//
// EVERY ERROR THIS FUNCTION RETURNS SATISFIES EXACTLY ONE OF THE TWO
// SENTINELS — that is a structural guarantee, not a convention callers must
// remember. verifyOAuthBearer (below) does the actual work and, like any
// function with many return statements, could in principle grow a return
// path that returns some other error; classifyOAuthVerificationError is the
// single choke point every result passes through before it leaves this
// package, and it maps anything that is neither sentinel to
// ErrOAuthServerFault — never to ErrOAuthInvalidToken. That direction
// matters: a caller (RequireOAuthScope today, the inference middleware
// next) that trusts the two-sentinel contract and defaults its own
// fallthrough to 401 would otherwise turn an unclassified internal bug into
// "your credential is dead" instead of a visible, alertable 500. See Task 1
// review findings 2 and 3.
func VerifyOAuthBearer(ctx context.Context, keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, issuer string, rawToken string) (*OAuthBearer, error) {
	bearer, err := verifyOAuthBearer(ctx, keySvc, clientSvc, issuer, rawToken)
	return bearer, classifyOAuthVerificationError(err)
}

// classifyOAuthVerificationError is VerifyOAuthBearer's structural backstop
// (Task 1 review, findings 2/3): it normalizes whatever
// verifyOAuthBearer returns into exactly ErrOAuthInvalidToken,
// ErrOAuthServerFault, or nil. Every return path inside verifyOAuthBearer
// already returns one of the two sentinels directly, so today this never
// changes anything observable — it exists so that stays true even if a
// future edit adds a return path that forgets to. An error that satisfies
// neither sentinel fails closed as ErrOAuthServerFault, the safe direction
// for an auth check: it must never be silently reported as a bad credential.
func classifyOAuthVerificationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrOAuthInvalidToken) || errors.Is(err, ErrOAuthServerFault) {
		return err
	}
	slog.Error("oauth: bearer verification returned an unclassified error; treating it as a server fault, not a bad credential", "error", err)
	return ErrOAuthServerFault
}

// verifyOAuthBearer is VerifyOAuthBearer's actual verification body — kept
// separate so classifyOAuthVerificationError has a single choke point to
// normalize through (see VerifyOAuthBearer's doc comment).
func verifyOAuthBearer(ctx context.Context, keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, issuer string, rawToken string) (*OAuthBearer, error) {
	if rawToken == "" || keySvc == nil {
		return nil, ErrOAuthInvalidToken
	}
	if issuer == "" || clientSvc == nil {
		// Misconfigured server, not a bad token. Fail closed and say so
		// where an operator will see it.
		slog.Error("oauth: bearer verifier is missing its issuer or client registry; refusing every token")
		return nil, ErrOAuthServerFault
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
	token, err := parser.ParseWithClaims(rawToken, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no kid")
		}
		resolvedKid = kid
		key, err := keySvc.ByKid(ctx, kid)
		if err != nil {
			if !errors.Is(err, service.ErrUnknownKid) {
				// Not "this kid doesn't exist" — a real failure to reach
				// the key store. Captured for the fault branch below;
				// still returned here so ParseWithClaims fails closed.
				keyResolutionErr = err
			}
			return nil, err
		}
		return &key.Private.PublicKey, nil
	})
	if keyResolutionErr != nil {
		slog.Error("oauth: cannot resolve signing key for bearer verification", "error", keyResolutionErr, "kid", resolvedKid)
		return nil, ErrOAuthServerFault
	}
	// A bearer credential must never be logged, on any path — success or
	// failure. Only kid (the key that verified it) and client_id (public by
	// design, see oauth_client_service.go) are ever logged in this
	// function.
	if err != nil || !token.Valid {
		return nil, ErrOAuthInvalidToken
	}

	granted, _ := claims["scope"].(string)

	// sub is always a decimal user id string — mintAccessToken writes it
	// via strconv.FormatInt (oauth_token_service.go). A token whose sub
	// doesn't parse must be rejected outright, not defaulted to 0: a
	// zero-valued oauth_user_id reaching a handler would read/act on
	// another user's (id 0, or whatever a handler treats as "no user")
	// data instead of failing closed.
	sub, _ := claims["sub"].(string)
	uid, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return nil, ErrOAuthInvalidToken
	}
	// Single-valued, non-empty aud only. `aud` is permitted by RFC 7519
	// to be an array, and mintAccessToken never writes one — a token
	// carrying an array here did not come from this server's mint, and
	// the bare type assertion that used to read it would have quietly
	// produced "" and handed that on as the caller's client identity.
	aud, ok := claims["aud"].(string)
	if !ok || aud == "" {
		return nil, ErrOAuthInvalidToken
	}
	if _, err := clientSvc.UsableByClientID(ctx, aud); err != nil {
		if !errors.Is(err, service.ErrClientNotUsable) && !dbent.IsNotFound(err) {
			// Could not check — not "the check failed". Reporting this
			// as invalid_token would make an outage indistinguishable
			// from every agent's credential dying at once.
			slog.Error("oauth: cannot resolve token audience for bearer verification", "error", err, "client_id", aud)
			return nil, ErrOAuthServerFault
		}
		return nil, ErrOAuthInvalidToken
	}

	return &OAuthBearer{UserID: uid, ClientID: aud, Scope: granted}, nil
}

// bearerFromAuthorizationHeader extracts the raw bearer credential from an
// HTTP Authorization header value, or reports ok == false if the header is
// missing or is not a well-formed Bearer-scheme credential. Gin-free and
// package-level (not a RequireOAuthScope-local closure) so any caller that
// reads a bearer credential off an Authorization header — RequireOAuthScope
// today, the inference middleware next — shares this exact parsing instead
// of growing its own copy that can drift from it (Task 1 review, finding
// 6).
//
// RFC 6750 §2.1 / RFC 7235 §2.1 make the auth-scheme token case-insensitive,
// so `bearer <tok>` is a well-formed credential and must not be rejected.
// The real client sends "Bearer", so this is interop hygiene — but a
// resource server that rejects a spec-conformant header is wrong regardless
// of who is calling it today.
func bearerFromAuthorizationHeader(authz string) (string, bool) {
	const bearerPrefix = "Bearer "
	if len(authz) < len(bearerPrefix) || !strings.EqualFold(authz[:len(bearerPrefix)], bearerPrefix) {
		return "", false
	}
	return strings.TrimSpace(authz[len(bearerPrefix):]), true
}

// RequireOAuthScope validates an OAuth-issued RS256 bearer token and
// enforces a required scope, for the OAuth resource-server surface
// (currently just GET /api/oauth/account).
//
// It is a thin wrapper around VerifyOAuthBearer: extract the bearer
// credential from the Authorization header, verify it, apply scopeSatisfies
// against the granted scope, and publish the resulting identity on the gin
// context. All of the actual token-verification properties — algorithm
// allowlist, kid dispatch, issuer/audience/expiry requirements, the
// key-store and client-registry fault splits — live in VerifyOAuthBearer's
// doc comment, not here, because this function no longer performs them
// itself.
//
// required == "" means "any validly-signed, unexpired OAuth token" — no
// specific scope needed, just proof of a real OAuth-issued identity.
//
// VerifyOAuthBearer returns one of two sentinel errors on failure, and this
// wrapper maps each to a distinct response: ErrOAuthInvalidToken (the
// credential is bad) becomes 401 {"error":"invalid_token"};
// EVERYTHING ELSE — ErrOAuthServerFault, and anything that somehow is
// neither sentinel — becomes 500 {"error":"server_error"}. The default arm
// is deliberately the safe one: for an auth check, "we could not tell"
// must fail closed as a server fault, never as a bad credential. The real
// Hermes Desktop client force-refreshes and retries on invalid_token, so an
// unclassified server-side fault reported as 401 would both mislabel an
// outage as every credential dying at once AND drive the exact retry loop
// that has previously driven an IP-wide 429 (Task 1 review, finding 2).
// VerifyOAuthBearer's own doc comment describes the matching structural
// guarantee on the producing side — this wrapper's default is the second,
// belt-and-suspenders layer, not the only one. Both faults are already
// logged inside VerifyOAuthBearer at the point of failure, with only kid
// and client_id — never the credential itself, on any path, success or
// failure.
func RequireOAuthScope(keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, issuer string, required string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := bearerFromAuthorizationHeader(c.GetHeader("Authorization"))
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
			return
		}

		bearer, err := VerifyOAuthBearer(c.Request.Context(), keySvc, clientSvc, issuer, raw)
		if err != nil {
			if errors.Is(err, ErrOAuthInvalidToken) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
				return
			}
			// Anything that isn't specifically a bad credential — including
			// ErrOAuthServerFault and any error that reaches here without
			// matching either sentinel — is reported as a server fault.
			// See this function's doc comment and Task 1 review finding 2.
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}

		// Scope is checked AFTER identity verification, deliberately — this
		// is a change from the pre-extraction code, which checked scope
		// first. VerifyOAuthBearer proves identity; only a token it accepts
		// as genuine reaches this line. Reporting 403 insufficient_scope
		// for a token whose identity was never established would tell the
		// caller "your scope was the only problem" about a credential this
		// server never verified belongs to anyone — identity first,
		// authorization second, is the correct order. The observable
		// consequence: a token that is BOTH scope-insufficient AND
		// identity-invalid (e.g. a revoked client, or an unparseable sub)
		// now gets 401 invalid_token where the pre-extraction code
		// returned 403 insufficient_scope. A valid-identity token with
		// insufficient scope is unaffected and still gets 403 here — it
		// never reaches the credential-refresh path a 401 triggers. Ruled
		// an intentional improvement, not reverted (Task 1 review,
		// finding 1).
		if !scopeSatisfies(bearer.Scope, required) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_scope"})
			return
		}

		c.Set(OAuthContextKeyUserID, bearer.UserID)
		c.Set(OAuthContextKeyClientID, bearer.ClientID)
		c.Set(OAuthContextKeyScope, bearer.Scope)
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
