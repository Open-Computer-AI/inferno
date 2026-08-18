package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthOrAPIKeyAuth runs the OAuth branch when the Authorization credential
// looks like a JWT, and otherwise delegates to the existing API-key middleware
// unchanged.
//
// # Why this exists at all
//
// Inferno mints OAuth access tokens that its own /v1 inference endpoints
// refused with 401 INVALID_API_KEY. That was reproduced end-to-end against a
// real device-code grant driven by the real hermes client before any of this
// was designed (.superpowers/sdd/2026-08-18-authorization-code-pkce/v1-oauth-gap-evidence.md).
// This middleware is the join between the two halves that already existed:
// VerifyOAuthBearer (which proves who the caller is) and
// OAuthBackingKeyService.Resolve (which produces the api_keys row the billing
// and usage pipeline is structurally unable to work without).
//
// # THE ORDERING IS THE POINT (evidence finding F-A)
//
// maxAPIKeyAuthorizationHeaderBytes is MaxAPIKeyCredentialBytes + 128 = 256,
// and apiKeyHeadersTooLarge (api_key_auth.go) aborts on it as the FIRST thing
// the API-key middleware does. A real RS256 access token's Authorization header
// is ~628 bytes. So every OAuth token was rejected on header size before
// apiKeyService.GetByKey was ever called -- which is exactly why the
// reproduction's access log shows latency_ms: 0 on every rejection.
//
// The shape test below therefore runs BEFORE the delegate is invoked, i.e.
// before that cap. A version of this middleware that verified OAuth tokens but
// sat behind the size check would 401 every single real token while looking
// perfectly healthy under tests that use short fake ones.
// TestOAuthTokenIsNotSizeCapped is the regression guard, and it uses a
// genuinely token-sized credential for that reason.
//
// The 256-byte cap is NOT raised. It is a cheap DoS guard on the API-key path
// and stays exactly as it is; the OAuth branch simply never reaches it.
//
// # THE TWO CREDENTIAL UNIVERSES MUST NOT BLUR (evidence finding F-B)
//
// A JWT-shaped credential that fails verification is REJECTED here. It is never
// retried as an API key. Falling through would let one request probe both
// credential universes, and it would also feed the API-key path's invalid-auth
// abuse limiter -- which is IP-keyed, so one OAuth-only agent can 429 every
// legitimately-keyed client behind the same egress IP. The reproduction drove
// the real client's 401 -> force-refresh loop to attempt 114 and reached exactly
// that IP-wide 429. That block is pre-existing and out of scope to fix; the
// obligation here is narrower and absolute: do not add a second path into it.
// Nothing on the OAuth branch calls recordInvalidAuthFailure.
//
// The corollary is the error mapping below. The real client
// (NousPortalAdapter.get_retry_credential) treats 401 -- and ONLY 401 -- as
// "the bearer is stale, force-refresh and retry". So 401 is reserved for the
// one case a refresh can actually fix: the token itself is bad. Everything a
// refresh cannot fix (insufficient scope, no group policy, a disabled backing
// row, a deactivated user) answers 403, which the client surfaces instead of
// retrying. Server faults answer 500. And the OAuth 401 body is
// {"error":"invalid_token"} -- deliberately distinguishable from the API-key
// path's {"code":"INVALID_API_KEY","message":"Invalid API key"}, so a client
// and an operator can both tell "your token is wrong" from "your key is wrong".
//
// # What it publishes, and why each entry is load-bearing
//
// Exactly what api_key_auth.go's success path publishes, because the /v1 chain
// downstream reads all of it:
//
//   - ctxkey.UserID on the REQUEST context (not just the gin context) --
//     gateway_profit_control.go, openai_profit_control.go and
//     openai_gateway_request_body.go read it there.
//   - ContextKeyAPIKey -- effectively the first line of every gateway handler;
//     absent, the handler answers 401 itself. AlphaSearch is stricter still and
//     also requires apiKey.Group != nil.
//   - ContextKeyUser (AuthSubject) -- absent, handlers answer
//     500 "User context not found".
//   - ContextKeyUserRole -- read by nothing on /v1 today; set for parity so the
//     two paths do not silently differ.
//   - group context, via the same setGroupContext the API-key path calls.
//   - SetOpsFallbackAPIKey, so ops error logs keep user/group/platform
//     attribution even on an early abort.
//
// ContextKeySubscription is deliberately NOT set: it is nil-tolerated
// downstream (a nil subscription bills the wallet), and loading it would mean
// duplicating the API-key path's subscription branch for no behaviour the
// handlers require.
//
// # Billing enforcement is downstream, not here
//
// The API-key path performs a balance/quota/subscription block of its own, but
// every gateway handler independently calls
// BillingCacheService.CheckBillingEligibility with (user, apiKey, group,
// subscription) before doing any work -- that is where balance, subscription
// windows, user x platform quota, per-key rate limits and RPM are enforced for
// BOTH paths. So this middleware does not duplicate that block. What it does
// enforce are the checks CheckBillingEligibility does NOT make and that are the
// only per-agent kill switches an operator has: the backing row's status,
// expiry and quota, and the owner's account status.
//
// # Nil dependencies fail closed, on purpose
//
// There is no "OAuth is not wired, so let JWTs through to the API-key path"
// branch. A nil keySvc makes VerifyOAuthBearer answer ErrOAuthInvalidToken; a
// nil clientSvc or an empty issuer makes it answer ErrOAuthServerFault; a nil
// backingSvc makes Resolve answer ErrInvalidBackingKeyRequest. Each of those
// maps to a visible failure below rather than to a silent fallthrough, which is
// the safe direction for an auth check and keeps F-B true even under
// misconfiguration.
func OAuthOrAPIKeyAuth(
	apiKeyAuth APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	keySvc *service.OAuthKeyService,
	clientSvc *service.OAuthClientService,
	backingSvc *service.OAuthBackingKeyService,
	issuer string,
) gin.HandlerFunc {
	delegate := gin.HandlerFunc(apiKeyAuth)
	return func(c *gin.Context) {
		// F-A: the shape test runs here, ahead of the delegate and therefore
		// ahead of apiKeyHeadersTooLarge's 256-byte cap.
		raw, ok := bearerFromAuthorizationHeader(c.GetHeader("Authorization"))
		if !ok || !looksLikeJWT(raw) {
			delegate(c)
			return
		}
		authenticateOAuthInference(c, apiKeyService, keySvc, clientSvc, backingSvc, issuer, raw)
	}
}

// authenticateOAuthInference is the OAuth branch itself. It never calls the
// API-key delegate on any path -- see F-B in OAuthOrAPIKeyAuth's doc comment.
func authenticateOAuthInference(
	c *gin.Context,
	apiKeyService *service.APIKeyService,
	keySvc *service.OAuthKeyService,
	clientSvc *service.OAuthClientService,
	backingSvc *service.OAuthBackingKeyService,
	issuer string,
	raw string,
) {
	bearer, err := VerifyOAuthBearer(c.Request.Context(), keySvc, clientSvc, issuer, raw)
	if err != nil {
		if errors.Is(err, ErrOAuthInvalidToken) {
			abortOAuthInference(c, http.StatusUnauthorized, "invalid_token", IngressRejectOAuthInvalidToken)
			return
		}
		// ErrOAuthServerFault, and anything that somehow satisfies neither
		// sentinel, is a server fault. Deliberately NOT 401: reporting an
		// outage as a dead credential would drive every agent in the fleet
		// into the refresh loop at once. Already logged inside
		// VerifyOAuthBearer at the point of failure. Not marked as an ingress
		// reject, so it stays visible in ops_error_logs as the operational
		// error it is.
		abortOAuthInference(c, http.StatusInternalServerError, "server_error", "")
		return
	}

	// Exact whitespace-split equality (scopeSatisfies), never strings.Contains:
	// a substring match would let "inference:invoke_nothing" satisfy this.
	if !scopeSatisfies(bearer.Scope, service.ScopeInferenceInvoke) {
		abortOAuthInference(c, http.StatusForbidden, "insufficient_scope", IngressRejectOAuthInsufficientScope)
		return
	}

	row, err := backingSvc.Resolve(c.Request.Context(), bearer.UserID, bearer.ClientID)
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// The client hung up (or its deadline expired) mid-resolve. That is
			// not a failed request and must not be reported as a 500 --
			// OAuthBackingKeyService.sanitizeBackingKeyError preserves both
			// sentinels through its error flattening precisely so this branch
			// can exist. Abort without writing a status: gin/net-http discard
			// the response for a dead connection anyway, and inventing a 5xx
			// here would pollute ops error logs with client disconnects.
			c.Abort()
			return
		case errors.Is(err, service.ErrNoGroupForOAuthKey):
			// An operator configuration problem, not a client one. Logged at
			// ERROR with the reason so it is diagnosable, and answered 403 so
			// the client does not read it as a stale credential and retry.
			slog.Error("oauth inference: no group for OAuth backing key; set oauth_backing_key.group_name to an active group",
				"error", err, "client_id", bearer.ClientID, "user_id", bearer.UserID)
			abortOAuthInferenceMessage(c, http.StatusForbidden, "oauth_backing_group_unavailable",
				"No group is configured for OAuth agents; set oauth_backing_key.group_name to an active group.",
				IngressRejectOAuthBackingGroupUnavailable)
			return
		default:
			// Includes ErrInvalidBackingKeyRequest, which means WE passed bad
			// input, not that the client did -- a 500, never a 4xx.
			slog.Error("oauth inference: cannot resolve backing api key",
				"error", err, "client_id", bearer.ClientID, "user_id", bearer.UserID)
			abortOAuthInference(c, http.StatusInternalServerError, "server_error", "")
			return
		}
	}

	// The row Resolve returns is the ent persistence entity; the /v1 pipeline
	// reads *service.APIKey off the context. repository.APIKeyEntityToService is
	// the one converter for that mapping (it also maps the Group's ~60 pricing
	// and routing fields), so this reuses it rather than growing a second copy
	// that would silently drift from the API-key path's view of a group.
	//
	// Its Key stays blank: OAuthBackingKeyService redacts the secret on every
	// row it returns, because a backing row's credential is never handed out.
	// Nothing here re-reads the row by any other means, so the redaction
	// survives into the gin context -- which is what keeps a stray %v or a
	// c.JSON of the context from being a credential leak.
	apiKey := repository.APIKeyEntityToService(row)
	if apiKey == nil || apiKey.User == nil || apiKey.Group == nil {
		// Resolve's contract guarantees all three. If it ever stops holding,
		// a nil Group reaches platform routing and panics, so fail here.
		slog.Error("oauth inference: backing api key is missing its owner or group",
			"client_id", bearer.ClientID, "user_id", bearer.UserID)
		abortOAuthInference(c, http.StatusInternalServerError, "server_error", "")
		return
	}

	// Ops error logs get user/group/platform attribution even if one of the
	// checks below aborts. Set before them, as the API-key path does.
	SetOpsFallbackAPIKey(c, apiKey)

	if !apiKey.User.IsActive() {
		// A deactivated owner must not keep serving inference for the
		// remaining lifetime of an already-minted token.
		abortOAuthInference(c, http.StatusForbidden, "account_inactive", IngressRejectUserInactive)
		return
	}
	// The backing row's own state is the only per-agent kill switch an operator
	// has -- an access token cannot be revoked inside its window, and
	// CheckBillingEligibility downstream checks balance and rate limits but not
	// the key's status, expiry or quota. 403 rather than 401 throughout: none of
	// these is fixable by refreshing the token.
	if !apiKey.IsActive() {
		abortOAuthInference(c, http.StatusForbidden, "backing_key_disabled", IngressRejectAPIKeyDisabled)
		return
	}
	if apiKey.IsExpired() {
		abortOAuthInference(c, http.StatusForbidden, "backing_key_expired", IngressRejectAPIKeyDisabled)
		return
	}
	if apiKey.IsQuotaExhausted() {
		abortOAuthInference(c, http.StatusForbidden, "backing_key_quota_exhausted", IngressRejectAPIKeyDisabled)
		return
	}

	// ctxkey.UserID goes on the REQUEST context, not only the gin context:
	// profit control and request-body personalisation read it from there.
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, apiKey.User.ID))

	c.Set(string(ContextKeyAPIKey), apiKey)
	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      apiKey.User.ID,
		Concurrency: apiKey.User.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), apiKey.User.Role)
	setGroupContext(c, apiKey.Group)

	if apiKeyService != nil {
		// Throttled in APIKeyService (L1 + singleflight), so this is not a
		// write per request. It is the only per-agent activity signal an
		// operator has on a backing row.
		_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
	}

	c.Next()
}

// abortOAuthInference writes an RFC 6750-shaped error body and, when reason is
// non-empty, marks the request as an ingress rejection.
//
// The body shape matches RequireOAuthScope's, so the whole OAuth surface of
// this server answers the same way, and it is distinguishable by construction
// from the API-key path's {"code":...,"message":...} body.
//
// The ingress mark matters for more than tidiness: an unmarked 4xx is captured
// into ops_error_logs as an operational request error. A client presenting a
// bad or under-scoped token is not an operational error, and the reasons here
// are OAuth-specific so the operator complaint the evidence records -- "an agent
// holding a perfectly valid OAuth credential is indistinguishable from a script
// spraying garbage API keys" -- is answered in the access log. Server faults
// pass reason == "" so they stay in ops_error_logs.
func abortOAuthInference(c *gin.Context, status int, code string, reason IngressRejectReason) {
	if reason != "" {
		MarkIngressRejected(c, reason)
	}
	setOAuthWWWAuthenticate(c, status, code)
	c.AbortWithStatusJSON(status, gin.H{"error": code})
}

// abortOAuthInferenceMessage is abortOAuthInference plus an
// error_description, for the one case whose audience is an operator rather
// than a client: the group policy resolving to nothing.
func abortOAuthInferenceMessage(c *gin.Context, status int, code, description string, reason IngressRejectReason) {
	if reason != "" {
		MarkIngressRejected(c, reason)
	}
	setOAuthWWWAuthenticate(c, status, code)
	c.AbortWithStatusJSON(status, gin.H{"error": code, "error_description": description})
}

// setOAuthWWWAuthenticate adds the RFC 6750 §3 challenge header to 401/403
// responses. It is not decoration: it is the machine-readable half of "this is
// a bearer-token failure, not an API-key failure", for a client that only looks
// at headers and status.
func setOAuthWWWAuthenticate(c *gin.Context, status int, code string) {
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		return
	}
	c.Header("WWW-Authenticate", `Bearer error="`+code+`"`)
}

// looksLikeJWT reports whether raw is exactly three non-empty, base64url
// segments separated by dots -- the shape test that decides which credential
// universe a request belongs to.
//
// It is deliberately a SHAPE test, not a verification: verification is
// VerifyOAuthBearer's job, and something that looks like a JWT but does not
// verify must be rejected (F-B), not handed to the API-key path. The shape
// alone has to be enough to make that routing decision.
//
// It cannot misroute a real API key: keys are prefix + hex
// (GenerateAPIKeySecret) and contain no '.' at all, so no API key has three
// dot-separated segments. Conversely a credential this rejects -- including an
// alg=none token, whose signature segment is empty -- goes to the API-key path
// and fails there as an unknown key; it cannot authenticate, because no API key
// contains a dot.
//
// '=' is excluded from the alphabet: JWS uses unpadded base64url (RFC 7515 §2).
func looksLikeJWT(raw string) bool {
	if raw == "" {
		return false
	}
	segments := strings.Split(raw, ".")
	if len(segments) != 3 {
		return false
	}
	for _, segment := range segments {
		if segment == "" || !isBase64URLSegment(segment) {
			return false
		}
	}
	return true
}

func isBase64URLSegment(s string) bool {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'A' && ch <= 'Z',
			ch >= 'a' && ch <= 'z',
			ch >= '0' && ch <= '9',
			ch == '-', ch == '_':
		default:
			return false
		}
	}
	return true
}
