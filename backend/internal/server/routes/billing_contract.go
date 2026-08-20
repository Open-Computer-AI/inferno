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
// SCOPE: service.ScopeBillingRead, per the design. See task-1-report.md F2 --
// no shipping hermes client requests this scope, so an unmodified client's
// token is refused here today; that is a plan-level decision escalated rather
// than silently changed.
func RegisterBillingContractRoutes(r gin.IRouter, oauthH *handler.OAuthHandler, h *handler.BillingContractHandler) {
	scoped := r.Group("/api/billing")
	{
		scoped.GET("/state", middleware.RequireOAuthScope(
			oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), service.ScopeBillingRead,
		), h.State)
	}
}
