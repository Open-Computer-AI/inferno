package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterAgentRoutes mounts the desktop's agent discovery/registration
// surface at /api/agents -- at the server ROOT's /api prefix, deliberately
// NOT under /api/v1. Same reasoning as RegisterBillingContractRoutes, which
// this mirrors: the consumer is the desktop (apps/desktop/electron/main.ts),
// which hardcodes these exact paths, not the panel's versioned API.
//
// oauthH is threaded in only to build the bearer middleware, exactly like
// RegisterBillingContractRoutes -- it already holds the signing keys, the
// client registry and the issuer string the mint stamped.
//
// SCOPE: required == "", i.e. any validly-signed, unexpired OAuth token
// whose issuer and audience check out. Do NOT gate on agents:read or
// agents:manage -- both exist in the scope vocabulary, but no shipping
// client ever requests either, and gating on them would repeat the
// billing:read defect (see RegisterBillingContractRoutes's doc comment):
// the endpoint would 403 for every real caller rather than restricting
// anything. Dropping the scope requirement does not drop authentication --
// RequireOAuthScope("") still verifies signature, algorithm, kid, issuer,
// audience and expiry; only the scope predicate is satisfied by anything.
func RegisterAgentRoutes(r gin.IRouter, oauthH *handler.OAuthHandler, h *handler.AgentHandler) {
	scoped := middleware.RequireOAuthScope(
		oauthH.KeyService(), oauthH.ClientService(), oauthH.TokenIssuer(), "",
	)

	agents := r.Group("/api/agents")
	{
		agents.GET("", scoped, h.List)
		agents.POST("/register", scoped, h.Register)
	}

	// Task 4's Chronos scheduler surface: an agent arms, cancels and
	// cold-start-reconciles its own one-shot fires. Same gate as
	// /api/agents -- any validly-signed, unexpired token, no
	// agents:read/agents:manage requirement -- because this IS the wire
	// contract plugins/cron_providers/chronos/_nas_client.py's
	// NasCronClient hardcodes (paths, body keys, GET with no params for
	// list), and that client carries the same bearer token it already uses
	// for the rest of the portal API, nothing scope-specific.
	agentCron := r.Group("/api/agent-cron")
	{
		agentCron.POST("/provision", scoped, h.ProvisionCron)
		agentCron.POST("/cancel", scoped, h.CancelCron)
		agentCron.GET("/list", scoped, h.ListCron)
	}
}
