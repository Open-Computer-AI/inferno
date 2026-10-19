//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type liveRecoveryUserRepo struct {
	UserRepository
	user *User
}

func TestCreateLiveCallFailsClosedWithoutConcurrencyService(t *testing.T) {
	request := &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live"}`),
	}
	for _, svc := range []*OpenAIGatewayService{nil, {}} {
		_, err := svc.CreateLiveCall(context.Background(), request, LiveCallIdentity{}, ConcurrencyPolicy{}, 0)
		require.ErrorIs(t, err, ErrConcurrencyPolicyAdmissionUnavailable)
	}
}

func (r *liveRecoveryUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

type liveRecoveryCache struct {
	schedulerTestConcurrencyCache

	mu               sync.Mutex
	acquireCalls     int
	releaseCalls     int
	accountPublishes []int64
	userPublishes    []int64
}

func (c *liveRecoveryCache) AcquireConcurrency(_ context.Context, request ConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquireCalls++
	if c.acquireCalls == 1 {
		return ConcurrencyAdmissionResult{}, ErrConcurrencyPolicyAdmissionStale
	}
	return ConcurrencyAdmissionResult{Acquired: true}, nil
}

func (c *liveRecoveryCache) RefreshConcurrency(context.Context, ConcurrencyAdmissionRequest) (bool, error) {
	return true, nil
}

func (c *liveRecoveryCache) ReleaseConcurrency(context.Context, ConcurrencyAdmissionRequest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseCalls++
	return nil
}

func (c *liveRecoveryCache) PublishAccountConcurrencyPolicy(_ context.Context, accountID int64, _ ConcurrencyPolicy, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountPublishes = append(c.accountPublishes, accountID)
	return nil
}

func (c *liveRecoveryCache) PublishUserConcurrencyPolicy(_ context.Context, userID int64, _ ConcurrencyPolicy, _ int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userPublishes = append(c.userPublishes, userID)
	return nil
}

type liveRecoveryStore struct {
	liveTestStore
}

func (s *liveRecoveryStore) ClaimLiveController(context.Context, string, string, string) (bool, error) {
	return false, nil
}

type cancelAfterLivePostUpstream struct {
	cancel  context.CancelFunc
	mu      sync.Mutex
	methods []string
}

func (u *cancelAfterLivePostUpstream) Do(request *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.methods = append(u.methods, request.Method)
	u.mu.Unlock()
	if request.Method == http.MethodPost && u.cancel != nil {
		u.cancel()
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Location": {"/backend-api/codex/call_cancelled"}},
		Body:       io.NopCloser(strings.NewReader("v=0\r\n")),
	}, nil
}

func (u *cancelAfterLivePostUpstream) DoWithTLS(request *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(request, proxyURL, accountID, accountConcurrency)
}

func TestCreateLiveCallReloadsDBAndRetriesAfterStaleAuthority(t *testing.T) {
	live, requests := 4, 2
	account := Account{
		ID:                          88001,
		Platform:                    PlatformOpenAI,
		Type:                        AccountTypeOAuth,
		Status:                      StatusActive,
		Schedulable:                 true,
		Concurrency:                 9,
		LiveConcurrency:             &live,
		RequestConcurrency:          &requests,
		ConcurrencyPolicyGeneration: 2,
		Credentials: map[string]any{
			"access_token":       "live-recovery-token",
			"chatgpt_account_id": "live-recovery-account",
		},
	}
	user := &User{
		ID:                          88002,
		Concurrency:                 9,
		LiveConcurrency:             &live,
		RequestConcurrency:          &requests,
		ConcurrencyPolicyGeneration: 3,
	}
	cache := &liveRecoveryCache{}
	store := &liveRecoveryStore{}
	store.liveTestStore.GatewayCache = &schedulerTestGatewayCache{}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "live-recovery-test-secret"}}
	svc := &OpenAIGatewayService{
		accountRepo:           schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		userRepo:              &liveRecoveryUserRepo{user: user},
		cache:                 store,
		cfg:                   cfg,
		concurrencyService:    NewConcurrencyService(cache),
		httpUpstream:          &liveHTTPUpstreamStub{},
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.recovery"}`},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}

	created, err := svc.CreateLiveCall(context.Background(), &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-recovery"}`),
	}, LiveCallIdentity{APIKeyID: 88003, UserID: user.ID}, ConcurrencyPolicy{
		Live: &live, Request: &requests,
	}, 1)
	require.NoError(t, err)
	require.NotNil(t, created)
	require.Equal(t, 2, cache.acquireCalls, "stale authority must trigger one bounded retry")
	require.Equal(t, []int64{account.ID}, cache.accountPublishes)
	require.Equal(t, []int64{user.ID}, cache.userPublishes)
	require.NotNil(t, store.record)
	require.Equal(t, int64(2), store.record.AccountConcurrencyPolicyGeneration)
	require.Equal(t, int64(3), store.record.UserConcurrencyPolicyGeneration)
}

func TestCreateLiveCallCleansUpUpstreamWhenRequestCancelsBeforeSave(t *testing.T) {
	live, requests := 4, 2
	account := Account{
		ID:                 88101,
		Platform:           PlatformOpenAI,
		Type:               AccountTypeOAuth,
		Status:             StatusActive,
		Schedulable:        true,
		Concurrency:        9,
		LiveConcurrency:    &live,
		RequestConcurrency: &requests,
		Credentials: map[string]any{
			"access_token":       "live-cancel-token",
			"chatgpt_account_id": "live-cancel-account",
		},
	}
	cache := &liveRecoveryCache{acquireCalls: 1}
	store := &liveRecoveryStore{}
	store.liveTestStore.GatewayCache = &schedulerTestGatewayCache{}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "live-cancel-test-secret"}}
	ctx, cancel := context.WithCancel(context.Background())
	upstream := &cancelAfterLivePostUpstream{cancel: cancel}
	svc := &OpenAIGatewayService{
		accountRepo:           schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:                 store,
		cfg:                   cfg,
		concurrencyService:    NewConcurrencyService(cache),
		httpUpstream:          upstream,
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.cancel"}`},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}

	created, err := svc.CreateLiveCall(ctx, &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-cancel"}`),
	}, LiveCallIdentity{APIKeyID: 88103, UserID: 88102}, ConcurrencyPolicy{
		Live: &live, Request: &requests,
	}, 1)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, created)
	require.Nil(t, store.record, "canceled creation must not save a durable orphan mapping")
	cache.mu.Lock()
	require.Equal(t, 1, cache.releaseCalls, "canceled creation must release the single Live reservation")
	cache.mu.Unlock()
	upstream.mu.Lock()
	require.Equal(t, []string{http.MethodPost, http.MethodDelete}, upstream.methods)
	upstream.mu.Unlock()
}
