package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// liveMaxTestStore is the GatewayCache + LiveCallStore the scheduler and
// CreateLiveCall need. Claims are refused so the background observer exits.
type liveMaxTestStore struct {
	*schedulerTestGatewayCache
	mu      sync.Mutex
	records []*LiveCallRecord
}

func (s *liveMaxTestStore) SaveLiveCall(_ context.Context, record *LiveCallRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *record
	s.records = append(s.records, &copy)
	return nil
}

func (s *liveMaxTestStore) GetLiveCall(context.Context, string) (*LiveCallRecord, error) {
	return nil, ErrLiveCallNotFound
}

func (s *liveMaxTestStore) ClaimLiveController(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (s *liveMaxTestStore) ReleaseLiveController(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *liveMaxTestStore) GetLiveController(context.Context, string) (string, error) {
	return "", ErrLiveCallNotFound
}

func (s *liveMaxTestStore) MarkLiveCallClosed(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

// liveMaxTestConcurrencyCache mirrors the Redis Live lease rules in memory:
// a lease is refused with liveCapped when the account holds accountLiveMax
// Live sessions (accountLiveMax > 0).
type liveMaxTestConcurrencyCache struct {
	schedulerTestConcurrencyCache
	mu             sync.Mutex
	live           map[int64]int
	legacyAcquires []int64
	cappedAcquires []int64
	releases       int
}

func (c *liveMaxTestConcurrencyCache) AcquireLiveLease(
	_ context.Context,
	accountID int64,
	_ int,
	_ int64,
	_ int,
	_ int64,
	_ string,
	_ bool,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.legacyAcquires = append(c.legacyAcquires, accountID)
	c.live[accountID]++
	return true, nil
}

func (c *liveMaxTestConcurrencyCache) AcquireLiveLeaseWithAccountLiveMax(
	_ context.Context,
	accountID int64,
	_ int,
	accountLiveMax int,
	_ int64,
	_ int,
	_ int64,
	_ string,
	_ bool,
) (bool, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cappedAcquires = append(c.cappedAcquires, accountID)
	if accountLiveMax > 0 && c.live[accountID] >= accountLiveMax {
		return false, true, nil
	}
	c.live[accountID]++
	return true, false, nil
}

func (c *liveMaxTestConcurrencyCache) RefreshLiveLease(context.Context, int64, int64, int64, string) (bool, error) {
	return true, nil
}

func (c *liveMaxTestConcurrencyCache) ReleaseLiveLease(_ context.Context, accountID, _, _ int64, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releases++
	c.live[accountID]--
	return nil
}

func liveMaxTestAccount(id int64, priority int, liveMax any) Account {
	account := Account{
		ID:          id,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 8,
		Priority:    priority,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	if liveMax != nil {
		account.Extra = map[string]any{"live_max_concurrency": liveMax}
	}
	return account
}

func newLiveMaxTestService(t *testing.T, accounts []Account, cache *liveMaxTestConcurrencyCache) (*OpenAIGatewayService, *liveMaxTestStore) {
	t.Helper()
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "live-max-test-secret"}}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	store := &liveMaxTestStore{schedulerTestGatewayCache: &schedulerTestGatewayCache{}}
	return &OpenAIGatewayService{
		accountRepo:           schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:                 store,
		cfg:                   cfg,
		concurrencyService:    NewConcurrencyService(cache),
		httpUpstream:          &liveHTTPUpstreamStub{},
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}, store
}

func createLiveMaxTestCall(svc *OpenAIGatewayService) (*LiveCallCreated, error) {
	return svc.CreateLiveCall(context.Background(), &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test"}`),
	}, LiveCallIdentity{APIKeyID: 30, UserID: 20}, 0)
}

func TestAccountGetLiveMaxConcurrency(t *testing.T) {
	cases := []struct {
		extra map[string]any
		want  int
	}{
		{nil, 0},
		{map[string]any{}, 0},
		{map[string]any{"live_max_concurrency": 0}, 0},
		{map[string]any{"live_max_concurrency": -3}, 0},
		{map[string]any{"live_max_concurrency": "bad"}, 0},
		{map[string]any{"live_max_concurrency": float64(4)}, 4},
		{map[string]any{"live_max_concurrency": json.Number("6")}, 6},
		{map[string]any{"live_max_concurrency": "5"}, 5},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, (&Account{Extra: tc.extra}).GetLiveMaxConcurrency(), "%v", tc.extra)
	}
	require.Equal(t, 0, (*Account)(nil).GetLiveMaxConcurrency())
}

// Feature off: no live_max_concurrency -> the original AcquireLiveLease path,
// the extended method is never called.
func TestCreateLiveCallWithoutLiveMaxUsesLegacyLease(t *testing.T) {
	cache := &liveMaxTestConcurrencyCache{live: map[int64]int{}}
	accounts := []Account{liveMaxTestAccount(501, 0, nil), liveMaxTestAccount(502, 5, 0)}
	svc, store := newLiveMaxTestService(t, accounts, cache)

	created, err := createLiveMaxTestCall(svc)
	require.NoError(t, err)
	require.Equal(t, int64(501), created.Account.ID)
	require.Equal(t, []int64{501}, cache.legacyAcquires)
	require.Empty(t, cache.cappedAcquires)
	require.Len(t, store.records, 1)
}

// An account at its Live maximum is skipped for another account with Live room.
func TestCreateLiveCallSkipsAccountAtLiveMax(t *testing.T) {
	cache := &liveMaxTestConcurrencyCache{live: map[int64]int{601: 4}}
	accounts := []Account{liveMaxTestAccount(601, 0, 4), liveMaxTestAccount(602, 5, 4)}
	svc, store := newLiveMaxTestService(t, accounts, cache)

	created, err := createLiveMaxTestCall(svc)
	require.NoError(t, err)
	require.Equal(t, int64(602), created.Account.ID)
	require.Equal(t, []int64{601, 602}, cache.cappedAcquires)
	require.Empty(t, cache.legacyAcquires)
	require.Equal(t, 4, cache.live[601])
	require.Equal(t, 1, cache.live[602])
	require.Len(t, store.records, 1)
	require.Equal(t, int64(602), store.records[0].AccountID)
}

// Every account at its Live maximum -> the usual Live-full error (429).
func TestCreateLiveCallAllAccountsAtLiveMaxIsFull(t *testing.T) {
	cache := &liveMaxTestConcurrencyCache{live: map[int64]int{701: 2, 702: 2}}
	accounts := []Account{liveMaxTestAccount(701, 0, 2), liveMaxTestAccount(702, 5, 2)}
	svc, store := newLiveMaxTestService(t, accounts, cache)

	_, err := createLiveMaxTestCall(svc)
	require.ErrorIs(t, err, ErrLiveConcurrencyFull)
	require.ElementsMatch(t, []int64{701, 702}, cache.cappedAcquires)
	require.Empty(t, store.records)
	require.Equal(t, 0, cache.releases)
}

// A capped account next to an uncapped one: the uncapped account takes the
// call through the legacy path.
func TestCreateLiveCallCappedAccountFallsBackToUncappedAccount(t *testing.T) {
	cache := &liveMaxTestConcurrencyCache{live: map[int64]int{801: 1}}
	accounts := []Account{liveMaxTestAccount(801, 0, 1), liveMaxTestAccount(802, 5, nil)}
	svc, _ := newLiveMaxTestService(t, accounts, cache)

	created, err := createLiveMaxTestCall(svc)
	require.NoError(t, err)
	require.Equal(t, int64(802), created.Account.ID)
	require.Equal(t, []int64{801}, cache.cappedAcquires)
	require.Equal(t, []int64{802}, cache.legacyAcquires)
}
