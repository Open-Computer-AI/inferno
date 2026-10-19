//go:build unit

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newGrokUserPolicyTestHandler(t *testing.T, cache *helperConcurrencyCacheStub) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	return &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: billing,
		apiKeyService:       service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		concurrencyHelper:   NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond),
	}
}

func newGrokPolicyTestContext(t *testing.T, method, path string, body string, websocket bool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if websocket {
		request.Header.Set("Connection", "Upgrade")
		request.Header.Set("Upgrade", "websocket")
	}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	groupID := int64(7)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      42,
		UserID:  101,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformGrok},
		User:    &service.User{ID: 101},
	})
	live, requests := 4, 2
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{
		UserID:                      101,
		Concurrency:                 8,
		LiveConcurrency:             &live,
		RequestConcurrency:          &requests,
		ConcurrencyPolicyGeneration: 1,
	})
	return c, recorder
}

func TestGrokVoiceRejectsBeforeProviderSelectionWhenUserRequestBudgetIsFull(t *testing.T) {
	for _, route := range []struct {
		endpoint string
		path     string
		body     string
	}{
		{endpoint: "tts", path: "/v1/tts", body: `{"input":"hello"}`},
		{endpoint: "stt", path: "/v1/stt", body: `{}`},
	} {
		t.Run(route.endpoint, func(t *testing.T) {
			cache := &helperConcurrencyCacheStub{userSeq: []bool{false}}
			handler := newGrokUserPolicyTestHandler(t, cache)
			c, recorder := newGrokPolicyTestContext(t, http.MethodPost, route.path, route.body, false)

			handler.GrokVoice(c, route.endpoint)

			require.Equal(t, http.StatusTooManyRequests, recorder.Code)
			require.Equal(t, 1, cache.policyUserAcquireCalls)
			require.Equal(t, 0, cache.userAcquireCalls)
		})
	}
}

func TestGrokVoiceRejectsUnsupportedPlatformBeforeUserPolicyAdmission(t *testing.T) {
	cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
	handler := newGrokUserPolicyTestHandler(t, cache)
	c, recorder := newGrokPolicyTestContext(t, http.MethodPost, "/v1/tts", `{"input":"hello"}`, false)
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	require.True(t, ok)
	apiKey.Group.Platform = service.PlatformOpenAI

	handler.GrokVoice(c, "tts")

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Zero(t, cache.policyUserAcquireCalls)
}

func TestGrokRealtimeRejectsBeforeProviderSelectionWhenUserRequestBudgetIsFull(t *testing.T) {
	cache := &helperConcurrencyCacheStub{userSeq: []bool{false}}
	handler := newGrokUserPolicyTestHandler(t, cache)
	c, recorder := newGrokPolicyTestContext(t, http.MethodGet, "/v1/realtime?model=grok-voice-latest", "", true)

	handler.GrokRealtime(c)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, 1, cache.policyUserAcquireCalls)
	require.Equal(t, 0, cache.userAcquireCalls)
}

func TestGrokUserPolicyUsesSameRequestClassAsOtherProviderAdmission(t *testing.T) {
	cache := &helperConcurrencyCacheStub{userSeq: []bool{true, false}}
	handler := newGrokUserPolicyTestHandler(t, cache)
	other := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Millisecond)
	otherContext, _ := newGrokPolicyTestContext(t, http.MethodPost, "/v1/chat/completions", `{}`, false)
	otherSubject, ok := middleware2.GetAuthSubjectFromContext(otherContext)
	require.True(t, ok)
	streamStarted := false
	release, err := other.AcquireUserPolicySlotWithWait(
		otherContext,
		101,
		concurrencyPolicyFromAuthSubject(otherSubject),
		1,
		false,
		&streamStarted,
	)
	require.NoError(t, err)
	require.NotNil(t, release)
	defer release()

	c, recorder := newGrokPolicyTestContext(t, http.MethodPost, "/v1/tts", `{"input":"hello"}`, false)
	handler.GrokVoice(c, "tts")

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Equal(t, 2, cache.policyUserAcquireCalls)
}

func TestGrokUserPolicyReleaseRunsWhenRequestContextIsCanceled(t *testing.T) {
	cache := &helperConcurrencyCacheStub{userSeq: []bool{true}}
	handler := newGrokUserPolicyTestHandler(t, cache)
	c, _ := newGrokPolicyTestContext(t, http.MethodPost, "/v1/tts", `{"input":"hello"}`, false)
	requestContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(requestContext)

	// This direct helper call models a long-lived audio operation without opening
	// a provider connection; cancellation must release the request-class lease.
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	require.True(t, ok)
	streamStarted := false
	release, err := handler.concurrencyHelper.AcquireUserPolicySlotWithWait(c, subject.UserID, concurrencyPolicyFromAuthSubject(subject), subject.ConcurrencyPolicyGeneration, true, &streamStarted)
	require.NoError(t, err)
	require.NotNil(t, release)
	release = wrapReleaseOnDone(c.Request.Context(), release)
	cancel()
	require.Eventually(t, func() bool { return cache.policyUserReleaseCalls == 1 }, time.Second, time.Millisecond)
	release()
	require.Equal(t, 1, cache.policyUserReleaseCalls)
}
