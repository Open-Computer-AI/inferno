package routes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestOAuthTokenRouteIsUnauthenticated proves POST /api/oauth/token is
// reachable with no Authorization header at all. This endpoint IS the
// credential-issuance step (device_code and refresh_token grants) — a
// caller has no session yet by definition, which is the entire reason it
// lives on RegisterOAuthDeviceRoutes and not the jwtAuth-gated
// RegisterOAuthAPIRoutes.
//
// This test registers ONLY RegisterOAuthDeviceRoutes on a bare gin engine —
// no jwtAuth middleware anywhere — mirroring router.go's actual wiring. A
// future accidental move of "/token" onto the authenticated group would make
// this request 401 before ever reaching the handler; a request with
// grant_type=<garbage> is used because it reaches Token()'s final default
// branch without touching h.tokenSvc, so a zero-value *handler.OAuthHandler
// is enough — the point of this test is the ROUTE's auth gating, not the
// grant logic (covered in internal/handler/oauth_handler_test.go and
// internal/service/oauth_token_service_test.go).
func TestOAuthTokenRouteIsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterOAuthDeviceRoutes(router, &handler.OAuthHandler{})

	form := url.Values{"grant_type": {"not_a_real_grant_type"}}
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Deliberately no Authorization header — that is the entire point.
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"expected a normal 400 from the handler; a 401 here would mean this route picked up jwtAuth")
	require.JSONEq(t, `{"error":"unsupported_grant_type"}`, rec.Body.String())
}

// TestOAuthTokenRouteNotRegisteredOnAuthenticatedGroup is the inverse check:
// RegisterOAuthAPIRoutes (the jwtAuth-gated group) must NOT also expose
// "/token" — if a future change duplicated the registration there, an
// unauthenticated caller would still succeed today (gin matches whichever
// group's route wins), silently masking the regression until the
// authenticated-only group's behavior was relied upon elsewhere. Asserting
// there is exactly one registered handler for this path keeps that failure
// mode visible.
func TestOAuthTokenRouteNotRegisteredOnAuthenticatedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterOAuthAPIRoutes(router, &handler.OAuthHandler{}, func(c *gin.Context) {
		// If this middleware runs at all for "/token", the route is wrongly
		// dual-registered on the authenticated group too.
		t.Error("jwtAuth middleware must never run for POST /api/oauth/token")
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	RegisterOAuthDeviceRoutes(router, &handler.OAuthHandler{})

	form := url.Values{"grant_type": {"not_a_real_grant_type"}}
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"unsupported_grant_type"}`, rec.Body.String())
}
