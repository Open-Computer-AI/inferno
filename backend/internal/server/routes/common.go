package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
)

// RegisterCommonRoutes 注册通用路由（健康检查、状态等），并挂载 OAuth 2.0
// 的 well-known 发现端点（服务器根路径，非 /api 前缀）。
func RegisterCommonRoutes(r *gin.Engine, oauthHandler *handler.OAuthHandler) {
	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// OAuth 2.0 discovery — mounted at the server ROOT, not under /api. The
	// well-known JWKS path is fixed by RFC 8414 / RFC 7517 conventions and
	// must not be prefixed.
	RegisterOAuthWellKnown(r, oauthHandler)

	// Claude Code 遥测日志（忽略，直接返回200）
	r.POST("/api/event_logging/batch", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Setup status endpoint (always returns needs_setup: false in normal mode)
	// This is used by the frontend to detect when the service has restarted after setup
	r.GET("/setup/status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"needs_setup": false,
				"step":        "completed",
			},
		})
	})
}
