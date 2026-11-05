package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokVoiceRequestBodyMemoryOverload(t *testing.T) {
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	// Inferno's admission path needs a real concurrency helper; mirror the
	// existing grok_audio_concurrency_test.go fixture (acquisition succeeds).
	cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: billing,
		apiKeyService:       &service.APIKeyService{},
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond),
		cfg:                 cfg,
	}
	for _, endpoint := range []string{"tts", "stt", "custom-voices"} {
		t.Run(endpoint, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.RequestBodyLimit(1024, pkghttputil.NewBodyMemoryBudget(1)))
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					Group: &service.Group{Platform: service.PlatformGrok},
					User:  &service.User{ID: 1},
				})
				// Inferno's GrokVoice requires the auth subject before reading the
				// body (production api_key_auth middleware always sets it); upstream's
				// fixture injected only the API key. Subject mirrors the API key's user.
				live, requests := 4, 2
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{
					UserID:                      1,
					Concurrency:                 8,
					LiveConcurrency:             &live,
					RequestConcurrency:          &requests,
					ConcurrencyPolicyGeneration: 1,
				})
				c.Next()
			})
			// A normal Grok group without an allowlist reaches the handler before
			// any middleware reads its body.
			router.Use(middleware.GroupModelAllowlist())
			router.POST("/v1/"+endpoint, func(c *gin.Context) { h.GrokVoice(c, endpoint) })
			req := httptest.NewRequest(http.MethodPost, "/v1/"+endpoint, strings.NewReader(`{"input":"hello"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Equal(t, "1", rec.Header().Get("Retry-After"))
			require.Contains(t, rec.Body.String(), "Request body buffering capacity is exhausted")
		})
	}
}
