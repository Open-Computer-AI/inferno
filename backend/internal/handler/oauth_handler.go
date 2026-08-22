package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// OAuthHandler serves the OAuth 2.0 authorization-server surface.
//
// NOTE: these handlers deliberately do NOT use internal/pkg/response. That
// package wraps bodies in {code,message,data}; the hermes client parses
// RFC-shaped JSON at the top level and hard-fails on a wrapped body.
type OAuthHandler struct {
	keySvc    *service.OAuthKeyService
	clientSvc *service.OAuthClientService
	orgSvc    *service.OrgService
	deviceSvc *service.OAuthDeviceService
	tokenSvc  *service.OAuthTokenService
	// userSvc resolves the bearer's email for the Account payload. Deliberately
	// the narrow OAuthUserLookup (one read method) rather than the full
	// UserRepository, so this handler cannot grow into mutating users. Callers
	// may pass nil; Account then degrades to an email-less payload.
	userSvc service.OAuthUserLookup
	// authorizeSvc issues RFC 6749 §4.1 authorization codes for
	// GET/POST /oauth/authorize (Task 4). Set via SetAuthorizeService rather
	// than threaded through NewOAuthHandler's constructor -- that signature
	// is called directly (not through wire) by a number of existing tests,
	// and adding a required parameter there would break all of them for a
	// dependency most of those tests don't exercise. Mirrors
	// ProvideSettingHandler/ProvideBatchImageHandler's existing
	// construct-then-set pattern in handler/wire.go.
	authorizeSvc *service.OAuthAuthorizeService
	// billingSvc supplies the `subscription` object on Account (Task 4,
	// GET /api/oauth/account). Set via SetBillingContractService for the same
	// reason authorizeSvc is a setter, not a constructor parameter -- see that
	// field's comment. Callers may leave it nil; Account then omits
	// `subscription` entirely, which the client already treats identically to
	// "no subscription" (see billing_contract.go's AccountSubscription doc).
	billingSvc *service.BillingContractService
}

func NewOAuthHandler(keySvc *service.OAuthKeyService, clientSvc *service.OAuthClientService, orgSvc *service.OrgService, deviceSvc *service.OAuthDeviceService, tokenSvc *service.OAuthTokenService, userSvc service.OAuthUserLookup) *OAuthHandler {
	return &OAuthHandler{keySvc: keySvc, clientSvc: clientSvc, orgSvc: orgSvc, deviceSvc: deviceSvc, tokenSvc: tokenSvc, userSvc: userSvc}
}

// SetAuthorizeService wires the authorization-code issuance service used by
// Authorize (GET/POST /oauth/authorize). See the authorizeSvc field comment
// for why this is a setter and not a constructor parameter.
func (h *OAuthHandler) SetAuthorizeService(svc *service.OAuthAuthorizeService) {
	h.authorizeSvc = svc
}

// SetBillingContractService wires the billing contract adapter used by
// Account to populate `subscription`. See the billingSvc field comment for
// why this is a setter and not a constructor parameter.
func (h *OAuthHandler) SetBillingContractService(svc *service.BillingContractService) {
	h.billingSvc = svc
}

// KeyService exposes the OAuth signing key service so
// middleware.RequireOAuthScope (the resource-server bearer-verification
// middleware guarding GET /api/oauth/account) can verify token signatures
// without a separate wire-provided dependency — routes.go already has this
// handler in hand when it builds that middleware.
//
// Nil-receiver safe. RegisterGatewayRoutes now calls this while building the
// /v1 inference credential branch, and Handlers literals in route tests leave
// OAuth unset; a nil service there makes VerifyOAuthBearer fail closed rather
// than panicking at registration time.
func (h *OAuthHandler) KeyService() *service.OAuthKeyService {
	if h == nil {
		return nil
	}
	return h.keySvc
}

// ClientService exposes the OAuth client registry so
// middleware.RequireOAuthScope can resolve a token's audience against a
// real registration. Same rationale as KeyService above: routes.go already
// holds this handler when it builds that middleware, so this avoids a
// second wire-provided dependency.
//
// Nil-receiver safe, for the same reason as KeyService.
func (h *OAuthHandler) ClientService() *service.OAuthClientService {
	if h == nil {
		return nil
	}
	return h.clientSvc
}

// TokenIssuer exposes the exact issuer string OAuthTokenService stamps into
// the `iss` claim, so middleware.RequireOAuthScope verifies against the
// value the mint actually used rather than against a second, independently
// normalised read of cfg.Server.FrontendURL. Task 6's defect #1 was two
// consumers of that one config value disagreeing about its canonical form;
// routing the verifier through the minter's own accessor makes that
// disagreement unrepresentable here.
//
// Returns "" when no token service is wired, which the middleware treats as
// a misconfiguration and fails closed on.
//
// Nil-receiver safe, for the same reason as KeyService.
func (h *OAuthHandler) TokenIssuer() string {
	if h == nil || h.tokenSvc == nil {
		return ""
	}
	return h.tokenSvc.Issuer()
}

// JWKS handles GET /.well-known/jwks.json
func (h *OAuthHandler) JWKS(c *gin.Context) {
	jwks, err := h.keySvc.JWKS(c.Request.Context())
	if err != nil {
		slog.Error("oauth: jwks projection failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	c.JSON(http.StatusOK, jwks)
}

// RegisterSelfHostedClient handles POST /api/oauth/self-hosted-client.
// Bearer-authenticated: the caller must be a logged-in user.
func (h *OAuthHandler) RegisterSelfHostedClient(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	uid := subject.UserID

	var req dto.SelfHostedClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, uid)
	if err != nil {
		slog.Error("oauth: failed to look up orgs for user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if len(orgs) == 0 {
		slog.Error("oauth: no org for user")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	created, err := h.clientSvc.RegisterSelfHosted(ctx, orgs[0].ID, uid, req.RedirectOrigin, req.Name)
	if err != nil {
		if errors.Is(err, service.ErrClientNameTooLong) || errors.Is(err, service.ErrInvalidRedirectURI) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
			return
		}
		slog.Error("oauth: self-hosted client registration failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	slog.Info("oauth: registered self-hosted client",
		"client_id", created.ClientID, "org_id", orgs[0].ID)

	c.JSON(http.StatusOK, dto.SelfHostedClientResponse{
		ClientID: created.ClientID,
		Name:     created.Name,
	})
}

// DeviceCode handles POST /api/oauth/device/code.
// Form-encoded per RFC 8628. Unauthenticated — the client_id is the caller's
// identity and it has no session yet at this point in the flow.
func (h *OAuthHandler) DeviceCode(c *gin.Context) {
	clientID := c.PostForm("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	scope := c.PostForm("scope")

	grant, err := h.deviceSvc.RequestCode(c.Request.Context(), clientID, scope)
	if err != nil {
		if errors.Is(err, service.ErrPortalNotConfigured) {
			slog.Error("oauth: device code request failed, server misconfigured", "client_id", clientID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			return
		}
		if errors.Is(err, service.ErrInvalidScope) {
			// RFC 8628 §3.2 / RFC 6749 §4.1.2.1. Distinct from invalid_client
			// on purpose: an unrecognised scope is a caller bug that a
			// developer must be able to see, and it reveals nothing — the
			// vocabulary is public and fixed.
			slog.Warn("oauth: device code request rejected, unknown scope", "client_id", clientID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_scope"})
			return
		}
		// err is logged (it's an internal wrapped error — "unknown client_id"
		// or a persistence failure — never device_code/user_code, which are
		// never logged anywhere) so an operator can tell a bad client_id
		// apart from a real backend fault. The response body deliberately
		// stays the single generic "invalid_client" for both cases — this is
		// an anti-probing choice on the wire, not a logging one.
		slog.Warn("oauth: device code request rejected", "client_id", clientID, "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client"})
		return
	}

	// Bare RFC 8628 body — NOT wrapped by internal/pkg/response.
	c.JSON(http.StatusOK, gin.H{
		"device_code":               grant.DeviceCode,
		"user_code":                 grant.UserCode,
		"verification_uri":          grant.VerificationURI,
		"verification_uri_complete": grant.VerificationURIComplete,
		"expires_in":                grant.ExpiresIn,
		"interval":                  grant.Interval,
	})
}

// tokenResponse writes a bare RFC 6749 §5.1 token response — NOT wrapped by
// internal/pkg/response. §5.1 also requires Cache-Control: no-store and
// Pragma: no-cache on every token response: the body carries a bearer
// credential, and an intermediary caching it would hand that credential to
// the next requester on the same cache path.
func tokenResponse(c *gin.Context, tokens *service.OAuthTokens) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.JSON(http.StatusOK, gin.H{
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    "Bearer",
		"expires_in":    tokens.ExpiresIn,
		"scope":         tokens.Scope,
	})
}

// nousRefreshTokenHeader is the header the real hermes client sends the
// refresh_token grant's credential in, INSTEAD of the RFC 6749 §6 form
// field: hermes_cli/auth.py's `_refresh_access_token` posts
// `headers={"x-nous-refresh-token": refresh_token}` with only `grant_type`
// and `client_id` in the body (confirmed by reading that function — never
// edited, that repo is read-only). Task 8 conformance-tested the
// device_code grant only and missed this: the refresh grant looked correct
// against RFC 6749 and the wire-contract tests, but against the actual
// client it silently read an always-empty body field and rejected every
// refresh. The client is the specification, so this is what the server
// reads.
const nousRefreshTokenHeader = "X-Nous-Refresh-Token"

// refreshTokenFromRequest resolves the refresh_token grant's credential:
// the header the real client sends, first; the RFC 6749 §6 form field,
// second. Preferring the header (rather than requiring both, or requiring
// the header) keeps this endpoint compatible with any other RFC-conformant
// client that only knows the body field, while still working against the
// one real client this plan exists to serve. An empty-after-trim header is
// treated as absent, not as a present-but-empty credential, so a client
// that sends an empty header alongside a real body value (or omits the
// header entirely — Go's http.Header.Get returns "" either way) still
// falls through to the body. Never log the return value — it's a bearer
// credential exactly like the body field it may have come from (see the
// no-credential-logging rule stated on Token below).
func refreshTokenFromRequest(c *gin.Context) string {
	if header := strings.TrimSpace(c.GetHeader(nousRefreshTokenHeader)); header != "" {
		return header
	}
	return c.PostForm("refresh_token")
}

// Token handles POST /api/oauth/token. Unauthenticated — this endpoint IS
// the credential-issuance step; a caller with a valid session wouldn't need
// it. Form-encoded per RFC 6749/8628. Never logs device_code, code,
// code_verifier, refresh_token, or access_token — client_id only.
//
// Every branch below matches known sentinels explicitly and falls through to
// a logged 500 server_error for anything else (a Redis outage, a signing-key
// read failure, ...). Folding an unmatched internal error into an OAuth
// error code — e.g. treating a transient backend fault as expired_token or
// invalid_grant — would tell every agent hitting it "your credential is
// dead, re-run the device flow", turning a blip into a fleet-wide forced
// re-auth, silently: nothing about that response distinguishes it from a
// real expired/invalid credential, and nothing gets logged for an operator
// to notice. See DeviceCode above for the same pattern already in use here.
func (h *OAuthHandler) Token(c *gin.Context) {
	grantType := c.PostForm("grant_type")
	clientID := c.PostForm("client_id")

	switch grantType {
	case "urn:ietf:params:oauth:grant-type:device_code":
		tokens, err := h.tokenSvc.ExchangeDeviceCode(c.Request.Context(), clientID, c.PostForm("device_code"))
		if err != nil {
			// RFC 8628 §3.5: authorization_pending/slow_down are the normal
			// poll-loop responses, not failures — the hermes client branches
			// on these exact strings. All matched cases are 400s; the OAuth
			// error body, not the HTTP status, carries the meaning.
			switch {
			case errors.Is(err, service.ErrAuthorizationPending):
				c.JSON(http.StatusBadRequest, gin.H{"error": "authorization_pending"})
			case errors.Is(err, service.ErrSlowDown):
				c.JSON(http.StatusBadRequest, gin.H{"error": "slow_down"})
			case errors.Is(err, service.ErrAccessDenied):
				c.JSON(http.StatusBadRequest, gin.H{"error": "access_denied"})
			case errors.Is(err, service.ErrExpiredToken),
				errors.Is(err, service.ErrDeviceCodeNotFound),
				errors.Is(err, service.ErrDeviceCodeExpired):
				c.JSON(http.StatusBadRequest, gin.H{"error": "expired_token"})
			default:
				slog.Error("oauth: device_code token exchange failed", "client_id", clientID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			}
			return
		}
		tokenResponse(c, tokens)

	case "authorization_code":
		tokens, err := h.tokenSvc.ExchangeAuthorizationCode(c.Request.Context(), clientID,
			c.PostForm("code"), c.PostForm("redirect_uri"), c.PostForm("code_verifier"))
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidGrant):
				// Every redemption failure — unknown code, replay, wrong
				// PKCE verifier, mismatched client_id/redirect_uri,
				// expired — collapses to this single RFC 6749 §5.2 code, so
				// the wire response never reveals which check failed.
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
			default:
				slog.Error("oauth: authorization_code token exchange failed", "client_id", clientID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			}
			return
		}
		tokenResponse(c, tokens)

	case "refresh_token":
		tokens, err := h.tokenSvc.ExchangeRefreshToken(c.Request.Context(), clientID, refreshTokenFromRequest(c))
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidGrant), errors.Is(err, service.ErrRefreshTokenReused):
				// RFC 6749 §5.2: invalid_grant covers unknown/expired/wrong-client
				// tokens AND detected reuse — the wire response deliberately
				// does not distinguish reuse from an ordinary invalid token
				// (that would tell a prober which case it hit); the reuse
				// case is logged server-side instead (see ExchangeRefreshToken).
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant"})
			default:
				slog.Error("oauth: refresh_token token exchange failed", "client_id", clientID, "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
			}
			return
		}
		tokenResponse(c, tokens)

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
	}
}

// Account handles GET /api/oauth/account — the account/entitlement summary the
// hermes CLI reads. Authenticated by middleware.RequireOAuthScope (Task 6,
// RS256 OAuth bearer), NOT jwtAuth: a headless agent presents an OAuth-issued
// access token here, not an Inferno panel session, so it is registered on its
// own route group in server/routes/oauth.go rather than under
// RegisterOAuthAPIRoutes.
//
// THE RESPONSE SHAPE IS NOT OURS TO CHOOSE. Its only real consumer is
// hermes_cli/nous_account.py's `_info_from_account_payload`, which reads
// exactly these top-level keys:
//
//	payload["user"]                {email, privy_did}
//	payload["organisation"]        {id, slug, name}
//	payload["subscription"]        {plan, tier, monthly_charge, …}
//	payload["paid_service_access"] {allowed, paid_access, reason, …}
//	payload["tool_access"]         {enabled, coverage{…}}
//
// The first version of this handler returned {"user_id", "orgs"} — a shape
// lifted from the desktop's /api/agents 409 error body, not from this endpoint
// — so not one key matched. It failed SOFT (every .get() misses), which is
// worse than failing loudly: the client concluded "could not find a Nous
// Portal account or organisation" and "does not currently have paid service
// access" on every paid-capability gate, with nothing anywhere pointing at the
// contract mismatch.
//
// WHAT IS REAL AND WHAT IS NOT, so the billing sub-project knows what it
// inherits:
//   - `user.email` and `organisation` {id, slug, name} are REAL, read from
//     this Inferno's users and orgs tables.
//   - `privy_did` is OMITTED, not stubbed. Inferno does not use Privy; there is
//     no honest value, and inventing one would make the client believe it has a
//     wallet identity it does not have.
//   - `subscription` (Task 4) is REAL when h.billingSvc is wired and the caller
//     has an active subscription: service.BillingContractService.AccountSubscription
//     reuses Task 3's exact resolveCurrentSubscription/resolveTiers mappers.
//     `monthly_charge` inside it is DELIBERATELY ALWAYS OMITTED -- see that
//     type's doc comment in billing_contract.go -- because
//     hermes_cli/models.py:685-707's is_nous_free_tier reads `monthly_charge
//     == 0` as "free tier", and Inferno has no honest recurring-charge figure
//     for it. The whole key is omitted (nil, not present) when there is no
//     active subscription or h.billingSvc is nil -- the client's non-dict
//     branch (nous_account.py:706-707) treats that identically either way.
//   - `tool_access` is still OMITTED. There is no honest free-tool-pool model
//     in Inferno to report yet.
//   - `paid_service_access` is STUBBED to a deliberate, explicit "no paid
//     tier": allowed=false, paid_access=false. That is a choice to under-grant.
//     Inferno does have a real balance and subscriptions, but the mapping from
//     those to Nous's entitlement semantics is unverified, and guessing wrong
//     in the permissive direction unlocks paid capabilities nobody paid for.
//     Replace this — not the surrounding shape — when the billing adapter lands.
//
// `orgs` is retained alongside `organisation` for the multi-org case the design
// anticipates (D-1's 409 org_selection_required picker). Nothing consumes it
// today; it costs one array and losing it would mean rebuilding it later.
func (h *OAuthHandler) Account(c *gin.Context) {
	uidVal, ok := c.Get(middleware2.OAuthContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	userID, ok := uidVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}

	ctx := c.Request.Context()
	orgs, err := h.orgSvc.OrgsForUser(ctx, userID)
	if err != nil {
		slog.Error("oauth: account org lookup failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	out := make([]gin.H, 0, len(orgs))
	for _, o := range orgs {
		role, rerr := h.orgSvc.RoleIn(ctx, o.ID, userID)
		if rerr != nil {
			// A per-org role lookup failure should not fail the whole
			// account response (the caller still legitimately belongs to
			// this org — OrgsForUser just proved it via org_members) — but
			// it must never silently claim an elevated role. MEMBER is
			// this codebase's least-privileged org role.
			slog.Error("oauth: account role lookup failed", "org_id", o.ID, "error", rerr)
			role = service.OrgRoleMember
		}
		out = append(out, gin.H{
			"id":         strconv.FormatInt(o.ID, 10),
			"slug":       o.Slug,
			"name":       o.Name,
			"isPersonal": o.IsPersonal,
			"role":       role,
		})
	}

	user := gin.H{"id": strconv.FormatInt(userID, 10)}
	// canPay mirrors what THIS server actually enforces, and defaults to the
	// under-granting answer if the lookup fails (see below).
	canPay := false
	if h.userSvc != nil {
		if u, uerr := h.userSvc.GetByID(ctx, userID); uerr != nil {
			// The bearer verified, so the user existed when the token was
			// minted. Degrade to an email-less payload rather than 500ing the
			// whole account lookup: the org half is still useful and the client
			// treats a missing email as unknown.
			slog.Error("oauth: account user lookup failed", "user_id", userID, "error", uerr)
		} else if u != nil {
			user["email"] = u.Email
			canPay = u.Balance > 0
		}
	}

	// paid_service_access answers one question the client actually asks:
	// is_nous_free_tier() reads `allowed` and, when false, treats the account as
	// FREE TIER -- which both restricts the model list it will offer
	// (partition_nous_models_by_tier) and prints "Credit access paused · run
	// /topup" over every reply.
	//
	// This used to be hardcoded false. That was the right call while the mapping
	// from Inferno's billing model onto Nous's entitlement semantics was
	// unverified -- under-granting is the safe direction. It is no longer a
	// guess: `allowed` now reports exactly what this server already ENFORCES at
	// auth time, `apiKeyBalanceBelowAuthThreshold` (api_key_auth.go), which is
	// `balance <= 0`. Reporting anything else means the gateway and the account
	// endpoint disagree about the same user -- the client is told it cannot pay
	// while inference succeeds, or told it can while every request 403s.
	//
	// `tool_access` stays fully omitted: there is no honest free-tool-pool
	// model in Inferno yet.
	//
	// `has_active_subscription` was hardcoded false, thirteen lines above the
	// code that attaches a real `subscription` object built from the caller's
	// active user_subscription row. Once Task 4 made the fact knowable in this
	// exact function, "an unrelated stub in a different object" stopped being
	// true and the two halves of one response contradicted each other: a user
	// on plan "Pro" with a zero wallet was told, by a payload that names their
	// subscription, that they had none. It now comes from the same value
	// (ruling I-4) -- resolved BEFORE paidAccess is built, which is why the
	// AccountSubscription call moved above it.
	//
	// Client consequence, hermes_cli/nous_account.py's _no_paid_access_message:
	// with has_active_subscription false it prints "no active subscription or
	// usable credits ... Subscribe or add credits" (:305-310); with it true and
	// active_subscription_is_paid ABSENT -- which it is, Inferno has no such
	// concept, and _coerce_bool(None) is None, not False (nous_account.py:791) -- the two
	// intermediate branches (:292, :299) are both skipped and it falls through
	// to "no usable paid credits ... Add credits or update billing", which is
	// the true statement for a subscribed user whose balance ran out.
	//
	// Nothing entitlement-bearing hangs on this: `allowed`/`paid_access` carry
	// that, and both still report what the gateway itself enforces.
	//
	// subscription (Task 4). Nil (billingSvc unset, or the caller has no
	// active subscription) means the key is omitted entirely -- see the
	// Account doc comment and AccountSubscription's own doc comment in
	// billing_contract.go for why that is the correct, and safe, wire shape.
	var accountSub *service.BillingAccountSubscriptionView
	if h.billingSvc != nil {
		accountSub = h.billingSvc.AccountSubscription(ctx, userID)
	}

	paidAccess := gin.H{
		"allowed":                 canPay,
		"paid_access":             canPay,
		"has_active_subscription": accountSub != nil,
	}

	payload := gin.H{
		"user":                user,
		"orgs":                out,
		"paid_service_access": paidAccess,
	}

	if accountSub != nil {
		payload["subscription"] = accountSub
	}

	if len(orgs) > 0 {
		primary := out[0]
		payload["organisation"] = primary
		paidAccess["organisation_id"] = primary["id"]
	} else {
		// Post-C1 this should be unreachable (every session provisions a
		// personal org), but if it ever happens the honest answer is the one
		// the client already has a specific message for.
		slog.Error("oauth: account has no org", "user_id", userID)
		paidAccess["reason"] = "account_missing"
	}

	c.JSON(http.StatusOK, payload)
}

// deviceDecisionRequest is the body both ApproveDevice and DenyDevice bind:
// the user_code the human typed or that verification_uri_complete prefilled.
type deviceDecisionRequest struct {
	UserCode string `json:"user_code" binding:"required"`
}

// PendingDeviceAuthorization handles GET /api/oauth/device/pending?user_code=…
// — what the approval screen shows the human BEFORE the Approve button.
//
// RFC 8628 §5.4 requires the authorization server to display client and
// authorization information at the verification URI. Without it the screen was
// a bare code box and two buttons, identical to a routine login prompt, which
// is what makes a phished user_code work: the victim sees nothing naming the
// requester or the powers being handed over, approves, and the attacker polls
// out an access token plus a 30-day refresh family.
//
// Session-authenticated (jwtAuth) and on the envelope, like its ApproveDevice /
// DenyDevice neighbours and for the same reason — see the asymmetry note on
// ApproveDevice below.
//
// It never reveals whether a user_code exists to a caller WITHOUT a session.
//
// A session-holding caller CAN distinguish the states: this handler maps
// not-found / expired / not-pending to distinct 404 / 410 / 409 responses,
// deliberately, because the human on the approval screen needs to be told which
// one happened — "that code does not exist" and "that code expired, run the
// command again" are different remedies. ApproveDevice and DenyDevice already
// drew the same distinction before this endpoint existed, so it adds no new
// surface. Enumeration is not the threat being defended against here: the code
// space is 31^8 (~8.5e11) with a 15-minute TTL, so guessing is infeasible, and
// a caller who already holds a session gains nothing from probing it.
//
// user_code is read from the query string and, like everywhere else in this
// file, never logged.
func (h *OAuthHandler) PendingDeviceAuthorization(c *gin.Context) {
	if _, ok := middleware2.GetAuthSubjectFromContext(c); !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	userCode := strings.TrimSpace(c.Query("user_code"))
	if userCode == "" {
		response.BadRequest(c, "invalid_request")
		return
	}

	pending, err := h.deviceSvc.PendingByUserCode(c.Request.Context(), userCode)
	switch {
	case err == nil:
		response.Success(c, gin.H{
			"client_name": pending.ClientName,
			"client_id":   pending.ClientID,
			"scopes":      pending.Scopes,
			"expires_at":  pending.ExpiresAt.UTC().Format(time.RFC3339),
		})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		response.NotFound(c, "device code not found")
	case errors.Is(err, service.ErrDeviceCodeExpired):
		response.Error(c, http.StatusGone, "device code expired")
	case errors.Is(err, service.ErrDeviceCodeNotPending):
		response.Error(c, http.StatusConflict, "device code already decided")
	default:
		slog.Error("oauth: pending device authorization lookup failed", "error", err)
		response.InternalError(c, "server_error")
	}
}

// ApproveDevice handles POST /api/oauth/device/approve and DenyDevice handles
// POST /api/oauth/device/deny — the device-flow approval screen's two
// actions, session-authenticated (jwtAuth): a logged-in human is confirming
// or rejecting a device-flow login in inferno-frontend's browser UI, not an
// agent or CLI presenting a bearer token or a bare client_id.
//
// DELIBERATE ASYMMETRY, read before "fixing" this: every other handler in
// this file (JWKS, DeviceCode, Token, Account, RegisterSelfHostedClient)
// emits bare JSON and is barred from internal/pkg/response, because the
// hermes client parses RFC-shaped JSON at the top level. These two handlers
// are the opposite — they MUST use internal/pkg/response — because they are
// NOT part of that wire contract at all. Nothing outside inferno-frontend
// ever calls them; they exist solely for its device approval screen.
// inferno-frontend/src/api/client.ts's response interceptor (its error path)
// reads apiData.code and apiData.message to build the message it surfaces to
// the user. A bare {"error":"not_found"} body has neither field, so every
// failure here would otherwise reach the user as a generic, non-actionable
// axios error instead of "that code doesn't exist" / "that code expired,
// run the command again". Keep this pair on response.*; keep the rest of
// this file bare.
//
// Neither handler ever logs user_code — it is a credential until redeemed,
// same rule as DeviceCode/Token above.
func (h *OAuthHandler) ApproveDevice(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req deviceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request")
		return
	}

	switch err := h.deviceSvc.Approve(c.Request.Context(), req.UserCode, subject.UserID); {
	case err == nil:
		response.Success(c, gin.H{"status": "approved"})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		response.NotFound(c, "device code not found")
	case errors.Is(err, service.ErrDeviceCodeExpired):
		response.Error(c, http.StatusGone, "device code expired")
	case errors.Is(err, service.ErrDeviceCodeNotPending):
		// Already approved, denied, or redeemed. 409 rather than 410: the code
		// has not timed out, a decision was simply already recorded for it, and
		// the screen says something different for each.
		response.Error(c, http.StatusConflict, "device code already decided")
	default:
		slog.Error("oauth: device approval failed", "user_id", subject.UserID, "error", err)
		response.InternalError(c, "server_error")
	}
}

func (h *OAuthHandler) DenyDevice(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req deviceDecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid_request")
		return
	}

	switch err := h.deviceSvc.Deny(c.Request.Context(), req.UserCode); {
	case err == nil:
		response.Success(c, gin.H{"status": "denied"})
	case errors.Is(err, service.ErrDeviceCodeNotFound):
		response.NotFound(c, "device code not found")
	case errors.Is(err, service.ErrDeviceCodeExpired):
		response.Error(c, http.StatusGone, "device code expired")
	case errors.Is(err, service.ErrDeviceCodeNotPending):
		response.Error(c, http.StatusConflict, "device code already decided")
	default:
		slog.Error("oauth: device denial failed", "user_id", subject.UserID, "error", err)
		response.InternalError(c, "server_error")
	}
}
