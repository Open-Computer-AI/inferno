package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// statusClientClosedRequest is nginx's non-standard 499 "client closed
// request". Used when the caller's context is already cancelled: the request
// must not answer 200 (a client parses that as a malformed success and the ops
// error logger stays quiet), and it must not answer 500 either, because a
// client that hung up is not a failed request.
const statusClientClosedRequest = 499

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
// row, a deactivated user, an IP the key's ACL denies) answers 403, which the
// client surfaces instead of retrying. Server faults answer 500. And the OAuth
// 401 body is {"error":"invalid_token"} -- deliberately distinguishable from
// the API-key path's {"code":"INVALID_API_KEY","message":"Invalid API key"}, so
// a client and an operator can both tell "your token is wrong" from "your key
// is wrong".
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
//   - ContextKeySubscription -- set when the backing row's group is a
//     subscription-type group. See the subscription section below: leaving it
//     nil is NOT benign there.
//   - group context, via the same setGroupContext the API-key path calls.
//   - SetOpsFallbackAPIKey, so ops error logs keep user/group/platform
//     attribution even on an early abort.
//
// # Scope is per endpoint, not per chain
//
// The /v1 mount is a group-level Use, so it also covers GET /v1/usage and
// GET /v1/sub2api/billing -- which read the backing key's usage history and the
// group's billing multipliers, not inference. The scope vocabulary defines
// billing:read as a distinct scope precisely so that line can be drawn
// (oauth_scope_vocabulary.go), and letting inference:invoke reach those two
// endpoints would erase it. requiredOAuthInferenceScope draws it instead.
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
// expiry, quota and IP ACL, and the owner's account status.
//
// # DOCUMENTED DIVERGENCES FROM THE API-KEY PATH
//
// These are deliberate, and each is here rather than left for a reader to
// discover:
//
//  1. expired / quota_exhausted are rejected at AUTH time. The API-key path
//     lets those two statuses through auth on purpose, so an exhausted key can
//     still read /v1/usage, /v1/sub2api/billing and async image-task results.
//     An OAuth agent does not get that: those endpoints now require
//     billing:read anyway (see above), so the carve-out would only apply to a
//     token holding both scopes, and admitting a known-exhausted credential to
//     the inference chain to preserve that narrow case is the worse trade.
//  2. Status codes differ. The API-key path answers quota exhaustion 429 and
//     expiry 403 API_KEY_EXPIRED; this branch answers both 403. It also answers
//     403 account_inactive where the API-key path answers 401 USER_INACTIVE /
//     401 USER_NOT_FOUND, for a deactivated and for a DELETED owner alike.
//     429 would also stay out of the client's refresh loop, but 403 keeps every
//     "a refresh cannot fix this" condition on one code -- and 401 is reserved,
//     absolutely, for the one thing a token refresh can fix, because the real
//     client force-refreshes on 401 and only on 401.
//  3. Balance diverges in BOTH directions, and the earlier version of this note
//     claimed only one of them. The API-key path rejects balance <= 0 at auth.
//     This branch leaves balance entirely to CheckBillingEligibility, which is
//     STRICTER on every billable endpoint (its MinimumBalanceReserve floor is
//     above zero) and LOOSER on the TWELVE routes that reach no handler calling
//     CheckBillingEligibility at all and that are not in the API-key path's
//     skipBilling set -- GET /v1/models and GET /models, the batch-image job
//     read/cancel/delete surface, and GET /v1/live/:call_id. On those, a
//     zero-balance user is refused with an API key and passes with an OAuth
//     token.
//     ADJUDICATED, twice, and deliberately not "fixed" in either direction: an
//     auth-time balance gate would stop an agent at zero balance from even
//     DISCOVERING it is at zero balance (model listing is on the client's
//     startup path), and removing the gate from the API-key path would break
//     "API-key auth keeps working everywhere it works today". No billable work
//     is reachable at zero balance via OAuth -- POST /v1/images/batches is the
//     one that looked like a bypass and is not, because
//     reserveBatchImageBalanceHold enforces balance at the database with
//     `... AND balance >= $1`. The class was re-derived handler by handler for
//     the fix wave, because BOTH whole-branch reviews over-counted it: the
//     video status/content reads, the custom-voice GETs, GET /v1/realtime and
//     the websocket GET /v1/responses all DO reach a CheckBillingEligibility,
//     so they diverge at auth time but not in outcome. The full trace, those
//     four near-misses, and the reasoning that clears POST /v1/images/batches
//     are in backend/scripts/oauth-conformance.md section 10. Behaviour is
//     pinned by oauth_billing_divergence_test.go; route-set membership by
//     routes/gateway_billing_divergence_test.go.
//  4. MarkOpsClientBusinessLimited is not called. Its rejections therefore do
//     not appear in the client-business-limited ops metric that the API-key
//     path's group/IP rejections feed; the OAuth-specific ingress-reject
//     reasons carry that signal instead.
//  5. On skipBilling routes, this branch is STRICTER about a failed
//     subscription load. The API-key path tolerates a subscription lookup that
//     fails on /v1/usage, /v1/sub2api/billing and async image-task reads (its
//     `if !skipBilling` guard lets the request through so the handler can still
//     return data). This branch skips the load only for /v1/sub2api/billing,
//     so on GET /v1/usage and GET /v1/images/tasks/:task_id with a
//     subscription-type policy group and no active subscription, an OAuth token
//     is refused 403 subscription_not_found where an API key is served.
//     Recorded rather than changed: the divergence list is only worth anything
//     if it is complete, and this direction is fail-closed on two read-only
//     endpoints, which is the cheap end of the trade.
//
// # Nil dependencies fail closed, on purpose
//
// There is no "OAuth is not wired, so let JWTs through to the API-key path"
// branch. A nil keySvc, a nil clientSvc or an empty issuer all make
// VerifyOAuthBearer answer ErrOAuthServerFault -> 500; a nil backingSvc makes
// Resolve answer ErrInvalidBackingKeyRequest, also 500. Each of those maps to a
// visible failure below rather than to a silent fallthrough, which is the safe
// direction for an auth check and keeps F-B true even under misconfiguration.
//
// 500 and not 401, for all of them. An unwired server is not a dead credential,
// and answering 401 would drive every agent into the force-refresh loop that
// ends in the IP-wide 429 -- the outcome F-B exists to prevent. (keySvc == nil
// answered 401 until the final review; see verifyOAuthBearer.)
func OAuthOrAPIKeyAuth(
	apiKeyAuth APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService OAuthSubscriptionLoader,
	keySvc *service.OAuthKeyService,
	clientSvc *service.OAuthClientService,
	backingSvc *service.OAuthBackingKeyService,
	issuer string,
	cfg *config.Config,
) gin.HandlerFunc {
	delegate := gin.HandlerFunc(apiKeyAuth)
	deps := oauthInferenceDeps{
		apiKeyService:       apiKeyService,
		subscriptionService: subscriptionService,
		keySvc:              keySvc,
		clientSvc:           clientSvc,
		backingSvc:          backingSvc,
		issuer:              issuer,
		cfg:                 cfg,
	}
	return func(c *gin.Context) {
		// F-A: the shape test runs here, ahead of the delegate and therefore
		// ahead of apiKeyHeadersTooLarge's 256-byte cap.
		raw, ok := bearerFromAuthorizationHeader(c.GetHeader("Authorization"))
		if !ok || !looksLikeJWT(raw) {
			delegate(c)
			return
		}
		authenticateOAuthInference(c, deps, raw)
	}
}

// OAuthSubscriptionLoader is the one thing the OAuth branch needs from the
// subscription service. It is declared here, on the consuming side, so this
// middleware can be tested against a real subscription outcome without
// standing up SubscriptionService's repositories, caches and background
// maintenance queue.
//
// *service.SubscriptionService satisfies it. Callers must pass an untyped nil
// when they have no service rather than a nil *SubscriptionService: the latter
// is a non-nil interface holding a nil pointer, and GetActiveSubscription
// dereferences its receiver.
type OAuthSubscriptionLoader interface {
	GetActiveSubscription(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error)
}

// oauthInferenceDeps is the OAuth branch's dependency set, grouped so the
// branch's own signature stays readable as it grew past the brief's original
// five parameters.
type oauthInferenceDeps struct {
	apiKeyService       *service.APIKeyService
	subscriptionService OAuthSubscriptionLoader
	keySvc              *service.OAuthKeyService
	clientSvc           *service.OAuthClientService
	backingSvc          *service.OAuthBackingKeyService
	issuer              string
	cfg                 *config.Config
}

// authenticateOAuthInference is the OAuth branch itself. It never calls the
// API-key delegate on any path -- see F-B in OAuthOrAPIKeyAuth's doc comment.
func authenticateOAuthInference(c *gin.Context, deps oauthInferenceDeps, raw string) {
	bearer, err := VerifyOAuthBearer(c.Request.Context(), deps.keySvc, deps.clientSvc, deps.issuer, raw)
	if err != nil {
		// Cancellation is checked first and on the REQUEST context, not only on
		// the returned error: VerifyOAuthBearer's classifier collapses every
		// non-credential failure to the bare ErrOAuthServerFault sentinel, so a
		// cancelled signing-key lookup arrives here with its cause already
		// discarded. Without this the most common way a request dies mid-flight
		// -- the client hanging up during token verification -- would be
		// reported as a 500.
		if abortIfOAuthRequestEnded(c, err) {
			return
		}
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
	required := requiredOAuthInferenceScope(c)
	if !scopeSatisfies(bearer.Scope, required) {
		abortOAuthInference(c, http.StatusForbidden, "insufficient_scope", IngressRejectOAuthInsufficientScope)
		return
	}

	row, err := deps.backingSvc.Resolve(c.Request.Context(), bearer.UserID, bearer.ClientID)
	if err != nil {
		switch {
		case abortIfOAuthRequestEnded(c, err):
			// The client hung up (or its deadline expired) mid-resolve. That is
			// not a failed request and must not be reported as a 500 --
			// OAuthBackingKeyService.sanitizeBackingKeyError preserves both
			// sentinels through its error flattening precisely so this branch
			// can exist.
			return
		case errors.Is(err, service.ErrBackingKeyOwnerGone):
			// The owning user has been deleted. That is an authorization
			// outcome, not an infrastructure fault, and it is the same class of
			// answer a DEACTIVATED owner gets a few lines below -- so it gets
			// the same code and the same ingress reason, which also means the
			// response does not disclose whether the account was disabled or
			// removed.
			//
			// 403 rather than the API-key path's 401 USER_NOT_FOUND: the real
			// hermes client treats 401 -- and only 401 -- as "the bearer is
			// stale, force-refresh and retry", and no refresh can resurrect a
			// deleted user. A 401 here would put every agent of that user into
			// the refresh loop that the evidence drove to an IP-wide 429, which
			// F-B commits absolutely to not creating a second path into. This
			// is the same deliberate 401->403 divergence already documented for
			// a deactivated owner, applied to the deleted case for the same
			// reason.
			//
			// Logged at ERROR, not swallowed: the alternative reading -- that
			// this is Resolve refusing to resurrect a row -- is exactly the
			// thing an operator wants to see in the log once, per token
			// lifetime, rather than never.
			slog.Error("oauth inference: backing key owner is no longer live; refusing to provision",
				"error", err, "client_id", bearer.ClientID, "user_id", bearer.UserID)
			abortOAuthInference(c, http.StatusForbidden, "account_inactive", IngressRejectUserInactive)
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
	if abortIfOAuthInferenceGroupNotAllowed(c, apiKey) {
		return
	}
	if abortIfOAuthInferenceIPDenied(c, apiKey, deps.cfg) {
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
		abortOAuthInference(c, http.StatusForbidden, "backing_key_expired", IngressRejectOAuthBackingKeyExpired)
		return
	}
	if apiKey.IsQuotaExhausted() {
		abortOAuthInference(c, http.StatusForbidden, "backing_key_quota_exhausted", IngressRejectOAuthBackingKeyQuotaExhausted)
		return
	}

	billingInfoRequest := c.Request.URL.Path == "/v1/sub2api/billing"
	subscription, ok := resolveOAuthInferenceSubscription(c, deps, apiKey, billingInfoRequest)
	if !ok {
		return
	}

	// ctxkey.UserID goes on the REQUEST context, not only the gin context:
	// profit control and request-body personalisation read it from there.
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), ctxkey.UserID, apiKey.User.ID))

	if subscription != nil {
		c.Set(string(ContextKeySubscription), subscription)
	}
	c.Set(string(ContextKeyAPIKey), apiKey)
	c.Set(string(ContextKeyUser), AuthSubject{
		UserID:      apiKey.User.ID,
		Concurrency: apiKey.User.Concurrency,
	})
	c.Set(string(ContextKeyUserRole), apiKey.User.Role)
	setGroupContext(c, apiKey.Group)

	if deps.apiKeyService != nil && !billingInfoRequest {
		// Throttled in APIKeyService (L1 + singleflight), so this is not a
		// write per request. It is the only per-agent activity signal an
		// operator has on a backing row. Skipped on the billing-info endpoint
		// for the same reason the API-key path skips it: reading your own
		// multipliers is not usage.
		_ = deps.apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
	}

	c.Next()
}

// requiredOAuthInferenceScope returns the scope this endpoint demands.
//
// GET /v1/usage and GET /v1/sub2api/billing are on the /v1 chain but are not
// inference: the first returns the backing key's usage history, the second the
// group's billing multipliers. billing:read exists in the scope vocabulary to
// gate exactly that, and ScopeBillingManage is documented as requiring a second
// device flow to elevate to -- a distinction that would be erased if
// inference:invoke reached them by virtue of sharing a middleware.
//
// The paths are matched exactly, mirroring api_key_auth.go's own literals for
// the same two endpoints.
func requiredOAuthInferenceScope(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return service.ScopeInferenceInvoke
	}
	switch c.Request.URL.Path {
	case "/v1/usage", "/v1/sub2api/billing":
		return service.ScopeBillingRead
	default:
		return service.ScopeInferenceInvoke
	}
}

// resolveOAuthInferenceSubscription loads the active subscription when the
// backing row's group is a subscription-type group, mirroring
// api_key_auth.go's own subscription load. It reports ok == false when it has
// already aborted the request.
//
// Leaving this nil is NOT benign, which is why it exists.
// CheckBillingEligibility computes
// `isSubscriptionMode := group.IsSubscriptionType() && subscription != nil`;
// with a nil subscription that is false even for a subscription group, so the
// request falls through to the wallet-balance check and logs
// usage_logs.subscription_id NULL. A subscription customer typically holds zero
// wallet balance, so the agent would be hard-403'd INSUFFICIENT_BALANCE while
// holding a valid subscription. Nothing stops an operator naming a
// subscription-type group in oauth_backing_key.group_name.
//
// ValidateAndCheckLimits is deliberately NOT run here, unlike the API-key path:
// CheckBillingEligibility's checkSubscriptionEligibility already enforces the
// window limits downstream on every handler, so running it here would be a
// duplicate check with a second set of status codes to keep in sync.
func resolveOAuthInferenceSubscription(
	c *gin.Context,
	deps oauthInferenceDeps,
	apiKey *service.APIKey,
	billingInfoRequest bool,
) (*service.UserSubscription, bool) {
	if apiKey.Group == nil || !apiKey.Group.IsSubscriptionType() {
		return nil, true
	}
	if deps.subscriptionService == nil || billingInfoRequest {
		// Matches the API-key path: reading one's own multipliers does not need
		// subscription data, and an uninjected service is not a client fault.
		return nil, true
	}
	if deps.cfg != nil && deps.cfg.RunMode == config.RunModeSimple {
		// D-2. The API-key path has a SimpleMode early return that fires BEFORE
		// its subscription load, so in SimpleMode a subscription-type group
		// never queries the subscription service and can never answer
		// SUBSCRIPTION_NOT_FOUND. Without this line, a deployment running
		// RUN_MODE=simple with a subscription-type group named in
		// oauth_backing_key.group_name would 403 every OAuth agent while every
		// API key succeeded -- same user, same route, opposite outcome.
		//
		// Skipping the load is the whole of the fix, and it is safe: in
		// SimpleMode BillingCacheService.CheckBillingEligibility returns nil
		// immediately, so a subscription this middleware did not publish is a
		// subscription nothing downstream would have read.
		return nil, true
	}
	subscription, err := deps.subscriptionService.GetActiveSubscription(c.Request.Context(), apiKey.User.ID, apiKey.Group.ID)
	if err != nil {
		if abortIfOAuthRequestEnded(c, err) {
			return nil, false
		}
		if !errors.Is(err, service.ErrSubscriptionNotFound) {
			// F-2. SubscriptionService.GetActiveSubscription passes its
			// repository's already-translated error straight through -- its own
			// comment says so -- so a genuine miss and a Postgres failure
			// arrive here indistinguishable unless they are split. Answering
			// 403 subscription_not_found to a database blip would tell every
			// agent in a subscription-group fleet "you have no subscription",
			// which is the exact inversion (an infrastructure fault becoming an
			// authorization answer) that VerifyOAuthBearer's key-store and
			// client-registry fault splits exist to prevent, and that this
			// branch was written under a global rule against.
			//
			// The API-key path has the identical flaw. That is parity, not a
			// licence: this is the half written under the rule.
			slog.Error("oauth inference: subscription lookup failed; this is an infrastructure fault, not a missing subscription",
				"error", err, "user_id", apiKey.User.ID, "group_id", apiKey.Group.ID)
			abortOAuthInference(c, http.StatusInternalServerError, "server_error", "")
			return nil, false
		}
		abortOAuthInference(c, http.StatusForbidden, "subscription_not_found", IngressRejectOAuthSubscriptionNotFound)
		return nil, false
	}
	return subscription, true
}

// abortIfOAuthInferenceGroupNotAllowed replicates the API-key path's
// exclusive-group entitlement check (abortIfAPIKeyGroupNotAllowed ->
// validateAPIKeyGroupAllowed -> User.CanBindGroup), which is the ONLY runtime
// enforcement of users.allowed_groups anywhere on the request path -- the other
// two CanBindGroup call sites are key-creation-time.
//
// Why the OAuth branch needs it (final review F-1). policyGroup resolves
// oauth_backing_key.group_name on name and active status only; nothing stops an
// operator naming an EXCLUSIVE group. Without this check, every OAuth user of
// every registered client would bill against that group regardless of
// users.allowed_groups, while an ordinary API key bound to the same group got
// 403 GROUP_NOT_ALLOWED for the same user -- and since apiKey.Group is the sole
// input to platform routing, channel-pool selection, model mapping and pricing,
// that is a premium upstream pool reachable by a credential path that never
// checks entitlement.
//
// The counter-argument was considered and rejected: one can argue the operator,
// not the user, binds this group, so a per-user entitlement check is
// inapplicable by design. But `users.allowed_groups` is the platform's answer to
// "may this user use this pool", and an OAuth agent acts for a user. Enforcing
// it makes the two credential paths agree; declining to would have to be
// written into the divergence list, and silence was the only option that was
// certainly wrong.
//
// It CANNOT be added without withBackingKeyAllowedGroups in
// oauth_backing_key.go: AllowedGroups arrives empty otherwise and this denies
// everyone. TestOAuthExclusiveGroupEntitlementIsEnforced asserts both
// directions for that reason.
//
// The mirrored predicate's two exemptions are kept verbatim: a subscription-type
// group is entitlement-free (the subscription IS the entitlement), and a row
// with no GroupID is out of scope. Resolve guarantees a non-nil Group, so the
// nil arms are unreachable here and are kept only so the two functions stay
// diff-able.
//
// abortIfAPIKeyGroupUnavailable (GROUP_DELETED / GROUP_DISABLED) is deliberately
// NOT mirrored: Resolve already rebinds a row whose group is missing or
// inactive, which is the genuine equivalent, and R-3.4 pinned it with mutation
// evidence.
func abortIfOAuthInferenceGroupNotAllowed(c *gin.Context, apiKey *service.APIKey) bool {
	if apiKey == nil || apiKey.GroupID == nil || apiKey.User == nil || apiKey.Group == nil {
		return false
	}
	if apiKey.Group.IsSubscriptionType() {
		return false
	}
	if apiKey.User.CanBindGroup(apiKey.Group.ID, apiKey.Group.IsExclusive) {
		return false
	}
	abortOAuthInference(c, http.StatusForbidden, "group_not_allowed", IngressRejectGroupNotAllowed)
	return true
}

// abortIfOAuthInferenceIPDenied enforces the backing row's IP allow/deny lists,
// the same control api_key_auth.go applies to an ordinary key.
//
// It compiles the rules here because nothing else on this path does.
// APIKeyService.compileAPIKeyIPRules populates CompiledIPWhitelist /
// CompiledIPBlacklist on every load path the API-key middleware uses, but the
// OAuth branch obtains its row through OAuthBackingKeyService +
// repository.APIKeyEntityToService, neither of which compiles anything -- so
// both compiled fields arrive nil and CheckIPRestrictionWithCompiledRules would
// silently allow everything.
//
// That matters because the restriction is reachable and editable: only
// APIKeyService.Delete refuses backing rows (ErrOAuthBackingKeyUndeletable);
// Update does not, so a user or admin can set an IP whitelist on a backing row
// through the ordinary key-management surface and the panel will display it. A
// security control that appears to be applied and is not is worse than one that
// is absent.
//
// The error message deliberately matches the API-key path's, vague about the
// mechanism, and the response is 403 -- not 401 -- so it stays out of the
// client's credential-refresh loop.
func abortIfOAuthInferenceIPDenied(c *gin.Context, apiKey *service.APIKey, cfg *config.Config) bool {
	if len(apiKey.IPWhitelist) == 0 && len(apiKey.IPBlacklist) == 0 {
		return false
	}
	trustForwarded := false
	if cfg != nil {
		trustForwarded = cfg.TrustForwardedIPForAPIKeyACL()
	}
	clientIP := ip.GetSecurityClientIP(c, trustForwarded)
	allowed, _ := ip.CheckIPRestrictionWithCompiledRules(
		clientIP,
		ip.CompileIPRules(apiKey.IPWhitelist),
		ip.CompileIPRules(apiKey.IPBlacklist),
	)
	if allowed {
		return false
	}
	abortOAuthInference(c, http.StatusForbidden, "ip_not_allowed", IngressRejectIPRestricted)
	return true
}

// abortIfOAuthRequestEnded answers a request whose caller is already
// gone, and reports whether it did.
//
// It serves EVERY consumer of VerifyOAuthBearer, not just this middleware --
// RequireOAuthScope (oauth_scope.go) calls it too. It was named
// ...Inference... when it had one caller, and R-4.1 was then recorded as closed
// while it was closed on only one of the two branches that needed it: a client
// that hung up during token verification on GET /api/oauth/account still got
// 500. The name is neutral now so the next surface to adopt VerifyOAuthBearer
// copies the right thing rather than whichever caller it happens to read first.
//
// It consults the REQUEST context first and the error second. VerifyOAuthBearer
// normalises everything that is not a bad credential to the bare
// ErrOAuthServerFault sentinel, discarding the cause, so a cancelled
// signing-key lookup carries no context.Canceled for errors.Is to find --
// checking c.Request.Context().Err() is the only way that case can be told
// apart from a genuine outage. OAuthBackingKeyService's sanitizer, by contrast,
// deliberately preserves both sentinels, so the error check covers Resolve.
//
// A status IS written. c.Abort() alone leaves gin to emit its default 200 with
// an empty body if the connection is in fact still alive, which a client parses
// as a malformed success and which the ops error logger never sees. 499 for a
// cancelled caller (nginx's "client closed request"; discarded outright if the
// socket really is gone) and 504 for an expired deadline.
func abortIfOAuthRequestEnded(c *gin.Context, err error) bool {
	cause := c.Request.Context().Err()
	if cause == nil {
		switch {
		case errors.Is(err, context.Canceled):
			cause = context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			cause = context.DeadlineExceeded
		default:
			return false
		}
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		c.AbortWithStatus(http.StatusGatewayTimeout)
		return true
	}
	c.AbortWithStatus(statusClientClosedRequest)
	return true
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
