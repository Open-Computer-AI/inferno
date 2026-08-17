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
	}
}
