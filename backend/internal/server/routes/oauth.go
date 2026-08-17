package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterOAuthWellKnown mounts unauthenticated discovery endpoints at the
// server root (NOT under /api) — JWKS must live at the well-known path.
func RegisterOAuthWellKnown(r gin.IRouter, h *handler.OAuthHandler) {
	r.GET("/.well-known/jwks.json", h.JWKS)
}
