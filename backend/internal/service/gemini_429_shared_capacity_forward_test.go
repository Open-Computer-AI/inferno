//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type gemini429SequenceUpstream struct {
	responses []*http.Response
	calls     int
}

func (s *gemini429SequenceUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	idx := s.calls
	s.calls++
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	return s.responses[idx], nil
}

func (s *gemini429SequenceUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, concurrency)
}

func TestGeminiForwardNative_VertexSharedCapacityRetriesSameAccountWithoutCooldown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sharedCapacity := `{"error":{"code":429,"message":"Resource exhausted. Please try again later.","status":"RESOURCE_EXHAUSTED"}}`
	success := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`
	upstream := &gemini429SequenceUpstream{responses: []*http.Response{
		{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(sharedCapacity))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(success))},
	}}
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{
		accountRepo:      repo,
		tokenProvider:    NewGeminiTokenProvider(repo, &fakeGeminiTokenCache{token: "vertex-token"}, nil),
		httpUpstream:     upstream,
		rateLimitService: NewRateLimitService(repo, nil, &config.Config{}, nil, nil),
		cfg:              &config.Config{},
	}
	c, rec := newGeminiNativeTestContext(t)
	result, err := svc.ForwardNative(context.Background(), c, newVertexTokenTestAccount(t), "gemini-3.8-flash", "generateContent", false,
		[]byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 2, upstream.calls)
	require.Zero(t, repo.rateLimitCalls, "shared capacity must not write account rate-limit state")
}

func TestHandleGeminiUpstreamError_SharedCapacityBodyStillCoolsNonVertexAccounts(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"Resource exhausted. Please try again later.","status":"RESOURCE_EXHAUSTED"}}`)
	tests := []struct {
		name    string
		account *Account
	}{
		{name: "ai_studio_api_key", account: &Account{ID: 605, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "k"}}},
		{name: "code_assist_oauth", account: &Account{ID: 606, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"oauth_type": "code_assist", "project_id": "project-1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimit429AccountRepoStub{}
			svc := &GeminiMessagesCompatService{accountRepo: repo}
			svc.handleGeminiUpstreamError(context.Background(), tt.account, http.StatusTooManyRequests, http.Header{}, body)
			require.Equal(t, 1, repo.rateLimitCalls)
			require.Equal(t, tt.account.ID, repo.lastRateLimitID)
			require.True(t, repo.lastRateLimitReset.After(time.Now()))
		})
	}
}

func TestHandleGeminiUpstreamError_VertexMessageRetryHintUsesParsedDelay(t *testing.T) {
	repo := &rateLimit429AccountRepoStub{}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	account := &Account{ID: 607, Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{"project_id": "my-vertex-project"}}
	body := []byte(`{"error":{"code":429,"message":"Resource exhausted. Please retry in 5s","status":"RESOURCE_EXHAUSTED"}}`)
	before := time.Now()
	svc.handleGeminiUpstreamError(context.Background(), account, http.StatusTooManyRequests, http.Header{}, body)
	require.Equal(t, 1, repo.rateLimitCalls)
	require.Equal(t, int64(607), repo.lastRateLimitID)
	require.WithinDuration(t, before.Add(5*time.Second), repo.lastRateLimitReset, 2*time.Second)
}
