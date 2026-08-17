package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterOAuthWellKnown mounts unauthenticated discovery endpoints at the
// server root (NOT under /api) — JWKS must live at the well-known path.
func RegisterOAuthWellKnown(r gin.IRouter, h *handler.OAuthHandler) {
	r.GET("/.well-known/jwks.json", h.JWKS)
}

// RegisterOAuthAPIRoutes mounts the authenticated OAuth self-service surface
// under /api/oauth — deliberately NOT /api/v1, since this path is shared
// with the unauthenticated OAuth endpoints (device/code, token, etc. — later
// tasks in this plan) and is consumed by the hermes CLI / dashboard, not the
// panel's versioned API.
func RegisterOAuthAPIRoutes(r gin.IRouter, h *handler.OAuthHandler, jwtAuth middleware.JWTAuthMiddleware) {
	authenticated := r.Group("/api/oauth")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	{
		authenticated.POST("/self-hosted-client", h.RegisterSelfHostedClient)
		// Device approval screen (Task 7): a logged-in human confirming or
		// rejecting a device-flow login in inferno-frontend's browser UI.
		// Session-authenticated like self-hosted-client above, unlike
		// device/code and token below (RegisterOAuthDeviceRoutes) which have
		// no session by design.
		authenticated.POST("/device/approve", h.ApproveDevice)
		authenticated.POST("/device/deny", h.DenyDevice)
	}
}

// RegisterOAuthDeviceRoutes mounts the UNAUTHENTICATED /api/oauth device-flow
// endpoints. Deliberately a separate group from RegisterOAuthAPIRoutes (which
// requires jwtAuth): a headless CLI calling POST /api/oauth/device/code has
// no session yet — client_id is the identity for this request, not a bearer
// token. Both groups share the /api/oauth prefix; gin allows two distinct
// *gin.RouterGroup values rooted at the same path as long as their route
// patterns don't collide.
//
// POST /api/oauth/token belongs here for the same reason: it is the
// credential-issuance step itself (device_code and refresh_token grants) —
// the caller by definition has no session/bearer token yet, so it cannot be
// gated behind jwtAuth.
func RegisterOAuthDeviceRoutes(r gin.IRouter, h *handler.OAuthHandler) {
	unauthenticated := r.Group("/api/oauth")
	{
		unauthenticated.POST("/device/code", h.DeviceCode)
		unauthenticated.POST("/token", h.Token)
	}
}

// RegisterOAuthAccountRoutes mounts GET /api/oauth/account behind Task 6's
// middleware.RequireOAuthScope — a THIRD distinct /api/oauth group,
// deliberately not merged into RegisterOAuthAPIRoutes (gated by jwtAuth,
// Inferno's HMAC panel-session middleware) or RegisterOAuthDeviceRoutes
// (unauthenticated). A hermes agent calling this endpoint presents an
// OAuth-issued ES256 bearer token, not a panel session and not a bare
// client_id — a third auth shape needs its own group. required scope is ""
// (any validly-signed, unexpired OAuth token) since this endpoint only
// resolves which org(s) the token's holder belongs to; it grants no
// elevated capability itself.
func RegisterOAuthAccountRoutes(r gin.IRouter, h *handler.OAuthHandler) {
	scoped := r.Group("/api/oauth")
	{
		scoped.GET("/account", middleware.RequireOAuthScope(h.KeyService(), ""), h.Account)
	}
}
