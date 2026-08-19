package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// jwtShapedCredential is three non-empty base64url segments -- the shape that
// routes a request to the OAuth branch. It is deliberately NOT a real token:
// this file asserts WHERE the branch is mounted, not what it does with a valid
// credential (middleware/oauth_inference_auth_test.go covers that). With no
// OAuth key service wired, the branch answers 401 {"error":"invalid_token"},
// which is exactly the observable that distinguishes "the OAuth branch ran"
// from "the API-key middleware ran".
const jwtShapedCredential = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"

type mountRoute struct {
	method string
	path   string
}

func (r mountRoute) name() string { return r.method + " " + r.path }

// newOAuthMountTestRouter registers the real gateway routes with a delegate
// that records every entry into the API-key middleware, so a test can tell
// which of the two credential paths a request took.
func newOAuthMountTestRouter(t *testing.T) (*gin.Engine, *atomic.Int32) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// The handlers in this fixture are zero-value stubs with no services wired,
	// so a request that gets PAST auth can panic inside them. That is fine and
	// expected -- these tests assert which of the two credential paths ran, not
	// what the handler does -- but the panic has to be contained or it takes
	// the whole test binary down. Registered before RegisterGatewayRoutes so
	// every route and group it creates inherits it.
	router.Use(gin.Recovery())
	var delegateCalls atomic.Int32

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			MaxBodySize:     1024 * 1024,
			TextMaxBodySize: 1024 * 1024,
		},
	}

	RegisterGatewayRoutes(
		router,
		&handler.Handlers{
			Gateway:       &handler.GatewayHandler{},
			OpenAIGateway: &handler.OpenAIGatewayHandler{},
			AsyncImage:    handler.NewAsyncImageHandler(nil, nil),
		},
		servermiddleware.APIKeyAuthMiddleware(func(c *gin.Context) {
			delegateCalls.Add(1)
			groupID := int64(1)
			c.Set(string(servermiddleware.ContextKeyAPIKey), &service.APIKey{
				GroupID: &groupID,
				Group:   &service.Group{Platform: service.PlatformOpenAI},
			})
			c.Next()
		}),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	return router, &delegateCalls
}

func requestWithBearer(t *testing.T, router *gin.Engine, route mountRoute, credential string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// oauthMountedRoutes is Task 4's declared mount set after fix round 1.
//
// The /v1 group carries the branch through a single group-level Use, so every
// /v1 endpoint is covered -- including the two that are not inference
// (/v1/usage, /v1/sub2api/billing), which the middleware gates on billing:read
// instead.
//
// The root-level aliases are the ones the real hermes client can actually
// reach. Its adapter allowlists {"/chat/completions", "/completions",
// "/embeddings", "/models"} and appends them to a base URL whose default is
// /v1-suffixed -- but NOUS_INFERENCE_BASE_URL bypasses the host allowlist and
// receives no normalisation that would append /v1, so an operator who sets it
// without the suffix moves the client onto exactly these paths. Leaving them on
// the bare apiKeyAuth would make a total, silent 401 outage depend on a config
// string ending in the right four characters. /responses and /alpha/search are
// here because they are what the gap evidence reproduced against.
//
// /completions is absent because this server does not serve that route at all
// -- a pre-existing 404, not an auth gap.
var oauthMountedRoutes = []mountRoute{
	{http.MethodPost, "/v1/responses"},
	{http.MethodPost, "/v1/responses/compact"},
	{http.MethodPost, "/v1/alpha/search"},
	{http.MethodPost, "/v1/messages"},
	{http.MethodPost, "/v1/chat/completions"},
	{http.MethodPost, "/v1/embeddings"},
	{http.MethodGet, "/v1/models"},
	{http.MethodGet, "/v1/usage"},
	{http.MethodGet, "/v1/sub2api/billing"},
	{http.MethodPost, "/responses"},
	{http.MethodPost, "/responses/compact"},
	{http.MethodGet, "/responses"},
	{http.MethodPost, "/alpha/search"},
	{http.MethodPost, "/chat/completions"},
	{http.MethodPost, "/embeddings"},
	{http.MethodGet, "/models"},
}

// oauthUnmountedRoutes is the declared boundary. These are other platforms'
// surfaces and none of them appears in the client's path allowlist:
// /v1beta runs APIKeyAuthWithSubscriptionGoogle (a different middleware
// entirely, and an explicit non-goal), and the rest still run the bare
// apiKeyAuth.
//
// This is not an assertion that the boundary is in the right place -- it is an
// assertion that it is where it is documented to be, so widening it later is a
// deliberate edit to this list rather than an accident.
var oauthUnmountedRoutes = []mountRoute{
	{http.MethodPost, "/messages/count_tokens"},
	{http.MethodPost, "/backend-api/codex/responses"},
	{http.MethodPost, "/antigravity/v1/messages"},
	{http.MethodGet, "/antigravity/models"},
	{http.MethodPost, "/web_search"},
}

// TestOAuthCredentialBranchIsMountedOnTheInferenceRoutes fails if any mount is
// reverted to the bare apiKeyAuth, because the request would then reach the
// delegate and be authenticated as an API key.
func TestOAuthCredentialBranchIsMountedOnTheInferenceRoutes(t *testing.T) {
	for _, route := range oauthMountedRoutes {
		t.Run(route.name(), func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			w := requestWithBearer(t, router, route, jwtShapedCredential)

			require.Equal(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
			require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String(),
				"a JWT-shaped credential must be answered by the OAuth branch, not the API-key path")
			require.Zero(t, delegateCalls.Load(),
				"the OAuth branch must not fall through to the API-key middleware")
		})
	}
}

// TestOrdinaryAPIKeyStillReachesTheAPIKeyPathOnTheInferenceRoutes is the other
// half: mounting the branch must not change how a non-JWT credential is
// handled on those same routes.
func TestOrdinaryAPIKeyStillReachesTheAPIKeyPathOnTheInferenceRoutes(t *testing.T) {
	for _, route := range oauthMountedRoutes {
		t.Run(route.name(), func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			w := requestWithBearer(t, router, route, "sk-an-ordinary-api-key")

			require.Equal(t, int32(1), delegateCalls.Load(),
				"a credential with no JWT shape belongs to the API-key path")
			// What the stub handlers do afterwards is not this test's business
			// -- several of them answer their own 401/500 with no services
			// wired. The property is that the OAuth branch did not answer.
			require.NotEqual(t, `{"error":"invalid_token"}`, strings.TrimSpace(w.Body.String()),
				"the OAuth branch must not have answered an API-key credential")
		})
	}
}

// TestOAuthCredentialBranchIsNotMountedOutsideTask4sScope pins the declared
// boundary. See oauthUnmountedRoutes.
func TestOAuthCredentialBranchIsNotMountedOutsideTask4sScope(t *testing.T) {
	for _, route := range oauthUnmountedRoutes {
		t.Run(route.name(), func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			requestWithBearer(t, router, route, jwtShapedCredential)

			require.Equal(t, int32(1), delegateCalls.Load(),
				"this route is outside Task 4's declared mount set and still runs apiKeyAuth")
		})
	}
}

// TestEveryClientReachablePathIsMounted states the client-coverage argument as
// an executable claim rather than as prose in a report.
//
// The hermes nous adapter allowlists these four relative paths and joins them
// to its configured base URL. Both spellings -- with and without the /v1 suffix
// the default base URL carries -- must reach the OAuth branch, because
// NOUS_INFERENCE_BASE_URL can legitimately be set either way and the difference
// is invisible until every request 401s in production.
func TestEveryClientReachablePathIsMounted(t *testing.T) {
	// "/completions" is deliberately absent: this server has no such route, so
	// it 404s on both spellings regardless of auth.
	clientPaths := []mountRoute{
		{http.MethodPost, "/chat/completions"},
		{http.MethodPost, "/embeddings"},
		{http.MethodGet, "/models"},
	}
	for _, base := range []string{"", "/v1"} {
		for _, route := range clientPaths {
			full := mountRoute{route.method, base + route.path}
			t.Run(full.name(), func(t *testing.T) {
				router, delegateCalls := newOAuthMountTestRouter(t)

				w := requestWithBearer(t, router, full, jwtShapedCredential)

				require.Equal(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
				require.JSONEq(t, `{"error":"invalid_token"}`, w.Body.String(),
					"a client path must not depend on NOUS_INFERENCE_BASE_URL carrying a /v1 suffix")
				require.Zero(t, delegateCalls.Load())
			})
		}
	}
}
