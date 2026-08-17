package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestOAuthAccountRouteUsesScopeMiddlewareNotJWTAuth proves GET
// /api/oauth/account is gated by Task 6's middleware.RequireOAuthScope, not
// jwtAuth (the panel-session HMAC middleware used by RegisterOAuthAPIRoutes).
// Registers ONLY RegisterOAuthAccountRoutes — no jwtAuth anywhere — mirroring
// router.go's actual wiring, then asserts a request with no Authorization
// header gets RequireOAuthScope's own "invalid_token" body (not a 404,
// meaning the route resolved) rather than reaching h.Account.
func TestOAuthAccountRouteUsesScopeMiddlewareNotJWTAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterOAuthAccountRoutes(router, handler.NewOAuthHandler(nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/account", nil)
	// Deliberately no Authorization header.
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestOAuthAccountRouteNotRegisteredOnJWTAuthGroup is the inverse check:
// RegisterOAuthAPIRoutes (the jwtAuth-gated group) must NOT also expose
// "/account" — if it were dual-registered there, jwtAuth (which accepts
// Inferno's HMAC panel-session tokens) would run for a request that should
// only ever be verified by the ES256-only RequireOAuthScope, defeating the
// entire point of Task 6's algorithm restriction.
func TestOAuthAccountRouteNotRegisteredOnJWTAuthGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterOAuthAPIRoutes(router, &handler.OAuthHandler{}, func(c *gin.Context) {
		t.Error("jwtAuth middleware must never run for GET /api/oauth/account")
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	RegisterOAuthAccountRoutes(router, handler.NewOAuthHandler(nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/api/oauth/account", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}
