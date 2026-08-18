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

// newOAuthMountTestRouter registers the real gateway routes with a delegate
// that records every entry into the API-key middleware, so a test can tell
// which of the two credential paths a request took.
func newOAuthMountTestRouter(t *testing.T) (*gin.Engine, *atomic.Int32) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
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

func postWithBearer(t *testing.T, router *gin.Engine, path, credential string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"gpt-5","input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestOAuthCredentialBranchIsMountedOnTheInferenceRoutes is the mount
// assertion for Task 4's declared route set: the /v1 group (a group-level
// Use, so it covers every /v1 endpoint) plus the three root-level aliases the
// gap evidence reproduced against.
//
// It fails if any of those mounts is reverted to the bare apiKeyAuth, because
// the request would then reach the delegate and be authenticated as an API key.
func TestOAuthCredentialBranchIsMountedOnTheInferenceRoutes(t *testing.T) {
	for _, path := range []string{
		"/v1/responses",
		"/v1/responses/compact",
		"/v1/alpha/search",
		"/v1/messages",
		"/responses",
		"/responses/compact",
		"/alpha/search",
	} {
		t.Run(path, func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			w := postWithBearer(t, router, path, jwtShapedCredential)

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
	for _, path := range []string{
		"/v1/responses",
		"/v1/alpha/search",
		"/responses",
		"/alpha/search",
	} {
		t.Run(path, func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			w := postWithBearer(t, router, path, "sk-an-ordinary-api-key")

			require.Equal(t, int32(1), delegateCalls.Load(),
				"a credential with no JWT shape belongs to the API-key path")
			require.NotEqual(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
		})
	}
}

// TestOAuthCredentialBranchIsNotMountedOutsideTask4sScope pins the declared
// scope boundary rather than leaving it implicit. /v1beta is an explicit
// non-goal (it runs APIKeyAuthWithSubscriptionGoogle, a different middleware
// entirely), and the root-level aliases outside the evidence's reproduction set
// still run the bare apiKeyAuth. If a later task widens the branch, this test
// is the one that should be updated deliberately -- it is not an assertion that
// the current boundary is correct, only that it is where it is documented to be.
func TestOAuthCredentialBranchIsNotMountedOutsideTask4sScope(t *testing.T) {
	for _, path := range []string{
		"/chat/completions",
		"/messages/count_tokens",
		"/backend-api/codex/responses",
	} {
		t.Run(path, func(t *testing.T) {
			router, delegateCalls := newOAuthMountTestRouter(t)

			postWithBearer(t, router, path, jwtShapedCredential)

			require.Equal(t, int32(1), delegateCalls.Load(),
				"this route is outside Task 4's declared mount set and still runs apiKeyAuth")
		})
	}
}
