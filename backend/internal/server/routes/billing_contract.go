package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// RegisterBillingContractRoutes mounts the Nous-shaped billing contract
// adapter at /api/billing -- at the server ROOT's /api prefix, deliberately
// NOT under /api/v1.
//
// Same reasoning as RegisterOAuthAccountRoutes, which this mirrors: the
// consumer is the hermes CLI, which hardcodes these exact paths
// (hermes_cli/nous_billing.py), not the panel's versioned API. Inferno's own
// /api/v1/payment/* surface is untouched and keeps its envelope; this group
// sits beside it and translates for one client.
//
// oauthH is threaded in only to build the bearer middleware: it already holds
// the signing keys, the client registry and the exact issuer string the mint
// stamped, and routing the verifier through the minter's own accessors is what
// keeps the two from drifting on a second read of the same config value. See
// OAuthHandler.TokenIssuer's doc comment.
//
// SCOPE: required == "", i.e. any validly-signed, unexpired OAuth token whose
// issuer and audience check out -- the same gate GET /api/oauth/account uses,
// and NOT billing:read.
//
// This was billing:read in the design, and billing:read is a scope no token can
// ever carry. The client requests inference:invoke at login
// (hermes_cli/auth.py's DEFAULT_NOUS_SCOPE), its only escalation asks for
// billing:manage, billing:read appears nowhere in it, and it documents this very
// endpoint as "no scope required" (nous_billing.py:480). Our AS stores the
// requested scope verbatim and adds no defaults, so no token the UNMODIFIED
// client can obtain will ever satisfy billing:read (the AS would mint one on
// request -- ScopeBillingRead is in knownScopes -- but nothing in the client
// ever asks). Gating on it did not restrict access, it removed the endpoint:
// every real call would 403, and the client would fail open to the same "not
// logged in" screen this adapter exists to remove. Ruled and changed
// (task-1-report.md F2 and §10).
//
// Dropping the SCOPE does not drop AUTHENTICATION. RequireOAuthScope still runs
// and still verifies signature, algorithm, kid, issuer, audience-against-the-
// registry and expiry; only the scope predicate is satisfied by anything. The
// data behind it is the caller's OWN balance and org, and a token minted for
// that user is already a statement that its holder acts as that user.
//
// The boundary that matters is unchanged: WRITE endpoints (POST /charge,
// /subscription/upgrade, PATCH /auto-top-up) keep billing:manage, a scope
// obtainable ONLY through the device grant's human approval screen and never
// through /oauth/authorize -- and nothing here relaxes that.
//
// NOTE for whoever builds /auto-top-up: the design doc says PUT; the client
// sends PATCH (nous_billing.py:497). The client wins.
// GET /subscription -- the CLI's plan picker (nous_billing.py:548-555,
// "no scope required"). Same gate as /state, same reasoning: any
// validly-signed, unexpired token, not billing:read.
//
// Deliberately NOT registered here: GET /auto-top-up and
// GET /subscription/pending-change. The client never sends either --
// PATCH /auto-top-up (nous_billing.py:480) and PUT/DELETE
// /subscription/pending-change (:594, :631) are the only requests it makes
// against those two paths, and it reads both states out of /api/billing/state
// instead. Adding a GET here would be exactly F-10 again: a route answering a
// question nobody asks. server/routes/billing_contract_route_test.go pins
// both at 404 so a future addition is a deliberate act, not drift.
// Task 5's 7 writes are mounted here too, every one on
// service.ScopeBillingManage.
//
// THESE ARE REACHABLE IN PRODUCTION TODAY. An earlier version of this comment
// claimed the opposite ("no other issuance path today; the device-flow step-up
// is a separate, not-yet-built change") and that was FALSE -- it read only
// OAuthAuthorizeService.IssueCode's refusal (oauth_authorize_service.go:221)
// and never the device grant. OAuthDeviceService.RequestCode
// (oauth_device_service.go:144) validates the requested scope with
// ValidateScope alone, knownScopes contains ScopeBillingManage
// (oauth_scope_vocabulary.go:56-64), and ExchangeDeviceCode mints the access
// token with that scope verbatim (oauth_token_service.go:513,518,535). The
// client already drives exactly that flow: hermes_cli/auth.py:9093
// step_up_nous_billing_scope re-runs the device flow with billing:manage
// appended (:9119-9128), invoked from hermes_cli/cli_billing_mixin.py:1303-1305
// when a write returns 403 insufficient_scope. So a real user CAN hold this
// scope and CAN call every handler below.
//
// That is intended (ruling D-1): the elevation is a real feature, gated behind
// the device grant's RFC 8628 human approval screen, which renders the
// requested scopes before the user approves -- see RequestCode's and
// IssueCode's doc comments for why the two grants differ. What follows from it
// is that the HANDLERS matter as much as the gate: each one is answered on its
// own merits (ruling R-5.1 as amended by D-2), not treated as dead code behind
// an unreachable scope. Methods are exactly the client's own
// (hermes_cli/nous_billing.py), each cited on its handler.
func RegisterBillingContractRoutes(r gin.IRouter, oauthH *handler.OAuthHandler, h *handler.BillingContractHandler) {
	scoped := r.Group("/api/billing")
	{
		scoped.GET("/state", middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), "",
		), h.State)
		scoped.GET("/subscription", middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), "",
		), h.Subscription)

		manage := middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), service.ScopeBillingManage,
		)
		scoped.POST("/charge", manage, h.Charge)
		scoped.GET("/charge/:id", manage, h.ChargeStatus)
		scoped.POST("/subscription/preview", manage, h.SubscriptionPreview)
		scoped.POST("/subscription/upgrade", manage, h.SubscriptionUpgrade)
		scoped.PUT("/subscription/pending-change", manage, h.PendingChangeSet)
		scoped.DELETE("/subscription/pending-change", manage, h.PendingChangeClear)
		scoped.PATCH("/auto-top-up", manage, h.AutoTopUp)
	}
}
