package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This file covers ApproveDevice/DenyDevice — the device approval screen's
// two panel-only endpoints. Unlike oauth_handler_test.go above (which pins
// POST /api/oauth/token to a bare, unwrapped body), these two are the
// declared exception: they MUST use internal/pkg/response's
// {code,message,data} envelope, because inferno-frontend's axios
// interceptor reads apiData.code/apiData.message to build the error surfaced
// to the user. requireResponseEnvelopeBody below asserts the opposite shape
// from requireBareErrorBody on purpose.

// newDeviceDecisionTestHandler mirrors newOAuthHandlerTestHandler (above,
// oauth_handler_test.go) but also hands back the ent client, since these
// tests need to seed oauth_device_authorizations rows directly and assert
// on them after the handler call — newOAuthHandlerTestHandler's client is
// unexported inside the *service.OAuthDeviceService it builds.
func newDeviceDecisionTestHandler(t *testing.T) (*OAuthHandler, *dbent.Client) {
	t.Helper()
	client := newOAuthHandlerTestEntClient(t)
	keySvc := service.NewOAuthKeyService(client)
	clientSvc := service.NewOAuthClientService(client)
	deviceSvc := service.NewOAuthDeviceService(client, "https://portal.example.com")
	tokenSvc := service.NewOAuthTokenService(client, keySvc, deviceSvc, newOAuthHandlerTestRefreshCache(), oauthHandlerTestUserLookup{}, "https://portal.example.com")
	return NewOAuthHandler(keySvc, clientSvc, nil, deviceSvc, tokenSvc, nil), client
}

// authedOAuthDeviceRouter wires ApproveDevice/DenyDevice behind a stand-in
// for jwtAuth that injects an AuthSubject, mirroring how
// routes.RegisterOAuthAPIRoutes actually gates these two routes in
// production (gin.HandlerFunc(jwtAuth) ahead of the handler).
func authedOAuthDeviceRouter(t *testing.T, h *OAuthHandler, userID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
		c.Next()
	})
	router.POST("/api/oauth/device/approve", h.ApproveDevice)
	router.POST("/api/oauth/device/deny", h.DenyDevice)
	return router
}

// unauthedOAuthDeviceRouter wires the same two routes with no auth
// middleware at all, exercising the "no session" branch every other
// authenticated handler in this package also has to fail closed on.
func unauthedOAuthDeviceRouter(t *testing.T, h *OAuthHandler) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/oauth/device/approve", h.ApproveDevice)
	router.POST("/api/oauth/device/deny", h.DenyDevice)
	return router
}

func postDeviceDecision(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// requireResponseEnvelopeBody asserts the internal/pkg/response
// {code,message,data} shape and returns the decoded envelope for further
// assertions on data. wantCode is the ENVELOPE's code field, not the HTTP
// status — response.Success always writes code:0 regardless of the HTTP
// status, and response.Error/NotFound/etc. mirror the HTTP status into code
// (see internal/pkg/response/response.go); callers assert rec.Code
// separately for the HTTP status.
func requireResponseEnvelopeBody(t *testing.T, rec *httptest.ResponseRecorder, wantCode int, wantMessage string) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "code")
	require.Contains(t, body, "message")
	require.Equal(t, float64(wantCode), body["code"])
	require.Equal(t, wantMessage, body["message"])
	// Bare RFC-8628/6749 bodies never carry these keys — their presence (or,
	// for "code"/"message", their exact values above) is what distinguishes
	// this pair from every other handler in oauth_handler.go.
	require.NotContains(t, body, "error")
	return body
}

// seedDeviceAuthorization inserts an oauth_device_authorizations row
// directly via ent, bypassing OAuthDeviceService.RequestCode (which also
// requires a registered oauth_client) — Approve/Deny only ever look up by
// user_code, so a registered client is not part of what this file tests.
func seedDeviceAuthorization(t *testing.T, client *dbent.Client, userCode string, expiresAt time.Time) {
	t.Helper()
	_, err := client.OAuthDeviceAuthorization.Create().
		SetDeviceCode("device-code-" + userCode).
		SetUserCode(userCode).
		SetClientID("agent:test-client").
		SetScope("").
		SetStatus("pending").
		SetExpiresAt(expiresAt).
		Save(t.Context())
	require.NoError(t, err)
}

func TestApproveDeviceRejectsMissingAuth(t *testing.T) {
	h := newOAuthHandlerTestHandler(t)
	router := unauthedOAuthDeviceRouter(t, h)

	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{"user_code":"ABCD-EFGH"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestDenyDeviceRejectsMissingAuth(t *testing.T) {
	h := newOAuthHandlerTestHandler(t)
	router := unauthedOAuthDeviceRouter(t, h)

	rec := postDeviceDecision(router, "/api/oauth/device/deny", `{"user_code":"ABCD-EFGH"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestApproveDeviceRejectsMissingUserCode(t *testing.T) {
	h := newOAuthHandlerTestHandler(t)
	router := authedOAuthDeviceRouter(t, h, 42)

	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestApproveDeviceUnknownCodeReturnsNotFound(t *testing.T) {
	h := newOAuthHandlerTestHandler(t)
	router := authedOAuthDeviceRouter(t, h, 42)

	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{"user_code":"NOPE-NOPE"}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusNotFound, "device code not found")
}

func TestApproveDeviceExpiredCodeReturnsGone(t *testing.T) {
	h, entClient := newDeviceDecisionTestHandler(t)
	router := authedOAuthDeviceRouter(t, h, 42)

	seedDeviceAuthorization(t, entClient, "EXPI-RED1", time.Now().Add(-time.Minute))

	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{"user_code":"EXPI-RED1"}`)

	require.Equal(t, http.StatusGone, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusGone, "device code expired")
}

func TestApproveDeviceSuccessNormalizesCodeAndRecordsApprover(t *testing.T) {
	h, entClient := newDeviceDecisionTestHandler(t)
	router := authedOAuthDeviceRouter(t, h, 99)

	seedDeviceAuthorization(t, entClient, "APPR-OVE1", time.Now().Add(10*time.Minute))

	// Lowercase and stray whitespace: the human may have typed or pasted it
	// either way — the handler passes it straight through to
	// OAuthDeviceService.Approve, which trims/uppercases before the lookup.
	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{"user_code":"  appr-ove1  "}`)

	require.Equal(t, http.StatusOK, rec.Code)
	body := requireResponseEnvelopeBody(t, rec, 0, "success")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "expected a data object, got %v", body["data"])
	require.Equal(t, "approved", data["status"])

	row, err := entClient.OAuthDeviceAuthorization.Query().Only(t.Context())
	require.NoError(t, err)
	require.Equal(t, "approved", row.Status)
	require.NotNil(t, row.ApprovedUserID)
	require.Equal(t, int64(99), *row.ApprovedUserID)
}

func TestDenyDeviceSuccess(t *testing.T) {
	h, entClient := newDeviceDecisionTestHandler(t)
	router := authedOAuthDeviceRouter(t, h, 99)

	seedDeviceAuthorization(t, entClient, "DENY-CODE", time.Now().Add(10*time.Minute))

	rec := postDeviceDecision(router, "/api/oauth/device/deny", `{"user_code":"DENY-CODE"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	body := requireResponseEnvelopeBody(t, rec, 0, "success")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "expected a data object, got %v", body["data"])
	require.Equal(t, "denied", data["status"])

	row, err := entClient.OAuthDeviceAuthorization.Query().Only(t.Context())
	require.NoError(t, err)
	require.Equal(t, "denied", row.Status)
}

// authedPendingRouter wires GET /api/oauth/device/pending behind the same
// AuthSubject stand-in the approve/deny routes use in production.
func authedPendingRouter(t *testing.T, h *OAuthHandler, userID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
		c.Next()
	})
	router.GET("/api/oauth/device/pending", h.PendingDeviceAuthorization)
	return router
}

func getPending(router *gin.Engine, userCode string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/oauth/device/pending?user_code="+url.QueryEscape(userCode), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestPendingDeviceAuthorizationRequiresSession: an unauthenticated caller
// must not be able to learn whether a user_code exists. Without this, anyone
// who phished (or guessed) a code could confirm it is live before using it.
func TestPendingDeviceAuthorizationRequiresSession(t *testing.T) {
	h, client := newDeviceDecisionTestHandler(t)
	seedDeviceAuthorization(t, client, "ABCD-EFGH", time.Now().Add(10*time.Minute))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/oauth/device/pending", h.PendingDeviceAuthorization)

	rec := getPending(router, "ABCD-EFGH")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusUnauthorized, "unauthorized")
}

// TestPendingDeviceAuthorizationReturnsClientAndScopes is the endpoint that
// makes the approval screen say WHO is asking and FOR WHAT — RFC 8628 §5.4.
// It is on the response envelope like its approve/deny neighbours, not bare:
// it is a panel endpoint the Vue app consumes, never part of the hermes wire
// contract.
func TestPendingDeviceAuthorizationReturnsClientAndScopes(t *testing.T) {
	h, client := newDeviceDecisionTestHandler(t)
	ctx := t.Context()

	_, err := client.OAuthClient.Create().
		SetClientID("agent:test-client").
		SetKind("SELF_HOSTED").
		SetName("my-laptop").
		SetOwnerUserID(7).
		SetOrgID(1).
		SetStatus(service.ClientActive).
		SetRedirectURIOrigin("https://agent.example.com").
		Save(ctx)
	require.NoError(t, err)

	_, err = client.OAuthDeviceAuthorization.Create().
		SetDeviceCode("device-code-WXYZ-2345").
		SetUserCode("WXYZ-2345").
		SetClientID("agent:test-client").
		SetScope("inference:invoke billing:read").
		SetStatus(service.DeviceStatusPending).
		SetExpiresAt(time.Now().Add(10 * time.Minute)).
		Save(ctx)
	require.NoError(t, err)

	router := authedPendingRouter(t, h, 7)
	rec := getPending(router, "wxyz-2345")

	require.Equal(t, http.StatusOK, rec.Code)
	body := requireResponseEnvelopeBody(t, rec, 0, "success")
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "data must be an object, got %T", body["data"])
	require.Equal(t, "my-laptop", data["client_name"])
	require.Equal(t, "agent:test-client", data["client_id"])
	require.Equal(t, []any{"inference:invoke", "billing:read"}, data["scopes"])
	require.NotEmpty(t, data["expires_at"])

	// The user_code is a credential until redeemed and must never be echoed.
	require.NotContains(t, rec.Body.String(), "WXYZ-2345")
}

// TestPendingDeviceAuthorizationRejectsDecidedCode: once a decision exists the
// screen must not re-offer the Approve button, so the endpoint reports the
// state rather than returning stale details.
func TestPendingDeviceAuthorizationRejectsDecidedCode(t *testing.T) {
	h, client := newDeviceDecisionTestHandler(t)
	seedDeviceAuthorization(t, client, "ABCD-EFGH", time.Now().Add(10*time.Minute))
	_, err := client.OAuthDeviceAuthorization.Update().
		SetStatus(service.DeviceStatusApproved).Save(t.Context())
	require.NoError(t, err)

	router := authedPendingRouter(t, h, 7)
	rec := getPending(router, "ABCD-EFGH")

	require.Equal(t, http.StatusConflict, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusConflict, "device code already decided")
}

// TestApproveDeviceRejectsAlreadyDecidedCode is the handler-level half of the
// terminal-state fix: re-approving a consumed code must surface as a distinct
// 409, not a success that silently re-arms the row.
func TestApproveDeviceRejectsAlreadyDecidedCode(t *testing.T) {
	h, client := newDeviceDecisionTestHandler(t)
	seedDeviceAuthorization(t, client, "ABCD-EFGH", time.Now().Add(10*time.Minute))
	_, err := client.OAuthDeviceAuthorization.Update().
		SetStatus(service.DeviceStatusExpired).Save(t.Context())
	require.NoError(t, err)

	router := authedOAuthDeviceRouter(t, h, 7)
	rec := postDeviceDecision(router, "/api/oauth/device/approve", `{"user_code":"ABCD-EFGH"}`)

	require.Equal(t, http.StatusConflict, rec.Code)
	requireResponseEnvelopeBody(t, rec, http.StatusConflict, "device code already decided")

	row, err := client.OAuthDeviceAuthorization.Query().Only(t.Context())
	require.NoError(t, err)
	require.Equal(t, service.DeviceStatusExpired, row.Status,
		"a consumed authorization must not be re-armed — the device_code would be redeemable twice")
}
