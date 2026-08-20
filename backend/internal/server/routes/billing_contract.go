package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

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
// requested scope verbatim and adds no defaults, so nothing on either side can
// produce a token that satisfies billing:read. Gating on it did not restrict
// access, it removed the endpoint: every real call would 403, the client would
// fail open to the same "not logged in" screen this adapter exists to remove,
// and the 403 would trigger a billing:manage step-up that /oauth/authorize
// refuses outright. Ruled and changed (task-1-report.md F2 and §10).
//
// Dropping the SCOPE does not drop AUTHENTICATION. RequireOAuthScope still runs
// and still verifies signature, algorithm, kid, issuer, audience-against-the-
// registry and expiry; only the scope predicate is satisfied by anything. The
// data behind it is the caller's OWN balance and org, and a token minted for
// that user is already a statement that its holder acts as that user.
//
// The boundary that matters is unchanged: WRITE endpoints (POST /charge,
// /subscription/upgrade, PATCH /auto-top-up) keep billing:manage -- the scope
// /oauth/authorize refuses outright -- and nothing here relaxes that.
//
// NOTE for whoever builds /auto-top-up: the design doc says PUT; the client
// sends PATCH (nous_billing.py:497). The client wins.
//
// GET /api/analytics/usage mounts on the SAME "" gate as /api/billing/state,
// not billing:read.
//
// The design doc's original Step 6 said billing:read here too, and it is
// wrong for exactly the reason /api/billing/state's F2 was: no real client
// ever requests billing:read (DEFAULT_NOUS_SCOPE is inference:invoke, the
// client's only step-up asks for billing:manage, and billing:read appears
// nowhere in it), so gating on it would not restrict this endpoint, it would
// remove it. Ruled (task-2-report.md), superseding the design doc's Step 6.
// The data is the caller's OWN usage history, and a token minted for that
// user is already a statement that its holder acts as that user -- the same
// reasoning /api/billing/state's route comment gives in full above.
func RegisterBillingContractRoutes(r gin.IRouter, oauthH *handler.OAuthHandler, h *handler.BillingContractHandler) {
	scoped := r.Group("/api/billing")
	{
		scoped.GET("/state", middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), "",
		), h.State)
	}

	analytics := r.Group("/api/analytics")
	{
		analytics.GET("/usage", middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), "",
		), h.Usage)
	}
}
