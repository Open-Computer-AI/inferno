//go:build unit

package handler

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestConcurrencyHelper_OrdinaryAccountAdmissionUsesSplitRequestBudget(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := repository.NewConcurrencyCache(rdb, 15, 900)
	publisher := cache.(service.ConcurrencyPolicyPublisher)
	liveAdmission := cache.(service.ConcurrencyAdmissionCache)
	concurrency := service.NewConcurrencyService(cache)
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)
	ctx := context.Background()
	live, requests := 4, 2
	policy := service.ConcurrencyPolicy{Live: &live, Request: &requests}
	require.NoError(t, publisher.PublishAccountConcurrencyPolicy(ctx, 801, policy, 1))
	require.NoError(t, publisher.PublishUserConcurrencyPolicy(ctx, 1801, policy, 1))

	// Hold real Live members first. The helper's legacy-shaped max=0 argument
	// must not turn the current split request policy into an unlimited path.
	liveIDs := []string{"helper-live-a", "helper-live-b"}
	liveRequests := make([]service.ConcurrencyAdmissionRequest, 0, len(liveIDs))
	for i, reservationID := range liveIDs {
		request := service.ConcurrencyAdmissionRequest{
			AccountID: 801, AccountPolicy: policy, AccountGeneration: 1,
			UserID: 1801, UserPolicy: policy, UserGeneration: 1,
			APIKeyID: int64(8801 + i), Class: service.ConcurrencyClassLive, ReservationID: reservationID,
		}
		result, err := liveAdmission.AcquireConcurrency(ctx, request)
		require.NoError(t, err)
		require.True(t, result.Acquired)
		liveRequests = append(liveRequests, request)
	}

	releases := make([]func(), 0, requests)
	for i := 0; i < requests; i++ {
		release, acquired, err := helper.TryAcquireAccountSlot(ctx, 801, 0)
		require.NoError(t, err)
		require.True(t, acquired)
		releases = append(releases, release)
	}
	_, acquired, err := helper.TryAcquireAccountSlot(ctx, 801, 0)
	require.NoError(t, err)
	require.False(t, acquired)

	for _, release := range releases {
		release()
	}
	for _, request := range liveRequests {
		require.NoError(t, liveAdmission.ReleaseConcurrency(ctx, request))
	}
	current, err := cache.GetAccountConcurrency(ctx, 801)
	require.NoError(t, err)
	require.Zero(t, current, "helper release and Live cleanup must leave no account members")
}

func TestConcurrencyHelper_AccountWaitRetryAndCancellationDoNotLeak(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := repository.NewConcurrencyCache(rdb, 15, 900)
	publisher := cache.(service.ConcurrencyPolicyPublisher)
	concurrency := service.NewConcurrencyService(cache)
	helper := NewConcurrencyHelper(concurrency, SSEPingFormatNone, 5*time.Millisecond)
	ctx := context.Background()
	live, requests := 4, 1
	policy := service.ConcurrencyPolicy{Live: &live, Request: &requests}
	require.NoError(t, publisher.PublishAccountConcurrencyPolicy(ctx, 802, policy, 1))

	hold, err := concurrency.AcquireAccountPolicySlot(ctx, 802, service.ConcurrencyClassRequest, 1)
	require.NoError(t, err)
	require.True(t, hold.Acquired)
	go func() {
		time.Sleep(25 * time.Millisecond)
		hold.ReleaseFunc()
	}()
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false
	release, err := helper.AcquireAccountSlotWithWaitTimeout(c, 802, 1, 500*time.Millisecond, false, &streamStarted)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	current, err := cache.GetAccountConcurrency(ctx, 802)
	require.NoError(t, err)
	require.Zero(t, current, "a successful wait retry must release its account member")

	require.NoError(t, publisher.PublishAccountConcurrencyPolicy(ctx, 803, policy, 1))
	hold, err = concurrency.AcquireAccountPolicySlot(ctx, 803, service.ConcurrencyClassRequest, 1)
	require.NoError(t, err)
	require.True(t, hold.Acquired)
	reqCtx, cancel := context.WithCancel(context.Background())
	c, _ = newHelperTestContext(http.MethodPost, "/v1/messages")
	c.Request = c.Request.WithContext(reqCtx)
	done := make(chan error, 1)
	go func() {
		streamStarted := false
		_, waitErr := helper.AcquireAccountSlotWithWaitTimeout(c, 803, 1, time.Second, false, &streamStarted)
		done <- waitErr
	}()
	time.Sleep(25 * time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	current, err = cache.GetAccountConcurrency(ctx, 803)
	require.NoError(t, err)
	require.Equal(t, 1, current, "cancellation must not create a leaked retry member")
	hold.ReleaseFunc()
	current, err = cache.GetAccountConcurrency(ctx, 803)
	require.NoError(t, err)
	require.Zero(t, current)
}

type capacityUserPolicyCache struct {
	helperConcurrencyCacheStub
	capacity service.ConcurrencyCapacityError
}

func (c *capacityUserPolicyCache) AcquireUserConcurrency(context.Context, service.UserConcurrencyAdmissionRequest) (service.ConcurrencyAdmissionResult, error) {
	return service.ConcurrencyAdmissionResult{DeniedScope: c.capacity.Scope}, nil
}

func TestConcurrencyHelper_UserQueueFullPreservesCapacityScope(t *testing.T) {
	cache := &capacityUserPolicyCache{capacity: service.ConcurrencyCapacityError{
		Class:  service.ConcurrencyClassRequest,
		Scope:  "user",
		Reason: "user_capacity",
	}}
	cache.waitAllowed = false
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false
	_, err := helper.AcquireUserPolicySlotWithWait(c, 804, service.ConcurrencyPolicy{Shared: 1}, 0, false, &streamStarted)
	var queueErr *WaitQueueFullError
	require.ErrorAs(t, err, &queueErr)
	var capacityErr *service.ConcurrencyCapacityError
	require.ErrorAs(t, err, &capacityErr)
	require.Equal(t, service.ConcurrencyClassRequest, capacityErr.Class)
	require.Equal(t, "user", capacityErr.Scope)
}

type staleUserPolicyCache struct {
	helperConcurrencyCacheStub
	mu              sync.Mutex
	acquireCalls    int
	readerCalls     int
	freshPolicy     service.ConcurrencyPolicy
	freshGeneration int64
	alwaysStale     bool
	denyAfterReload bool
}

func (c *staleUserPolicyCache) AcquireUserConcurrency(context.Context, service.UserConcurrencyAdmissionRequest) (service.ConcurrencyAdmissionResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquireCalls++
	if c.alwaysStale || c.acquireCalls == 1 {
		return service.ConcurrencyAdmissionResult{}, service.ErrConcurrencyPolicyAdmissionStale
	}
	if c.denyAfterReload && c.acquireCalls == 2 {
		return service.ConcurrencyAdmissionResult{DeniedScope: "user"}, nil
	}
	return service.ConcurrencyAdmissionResult{Acquired: true}, nil
}

func (c *staleUserPolicyCache) ReleaseUserConcurrency(context.Context, service.UserConcurrencyAdmissionRequest) error {
	return nil
}

func (c *staleUserPolicyCache) GetPublishedUserConcurrencyPolicy(context.Context, int64) (service.ConcurrencyPolicy, int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readerCalls++
	return c.freshPolicy, c.freshGeneration, nil
}

func TestConcurrencyHelper_UserPolicyReloadsSharedSplitTransitionsAndBoundsRetry(t *testing.T) {
	live, requests := 4, 2
	split := service.ConcurrencyPolicy{Live: &live, Request: &requests}
	shared := service.ConcurrencyPolicy{Shared: 3}

	for _, tc := range []struct {
		name        string
		oldPolicy   service.ConcurrencyPolicy
		freshPolicy service.ConcurrencyPolicy
	}{
		{name: "shared_to_split", oldPolicy: shared, freshPolicy: split},
		{name: "split_to_shared", oldPolicy: split, freshPolicy: shared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &staleUserPolicyCache{freshPolicy: tc.freshPolicy, freshGeneration: 2}
			helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
			release, acquired, err := helper.TryAcquireUserPolicySlot(context.Background(), 805, tc.oldPolicy, 1)
			require.NoError(t, err)
			require.True(t, acquired)
			require.NotNil(t, release)
			release()
			require.Equal(t, 2, cache.acquireCalls)
			require.Equal(t, 1, cache.readerCalls)
		})
	}

	cache := &staleUserPolicyCache{freshPolicy: split, freshGeneration: 2, alwaysStale: true}
	helper := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	_, acquired, err := helper.TryAcquireUserPolicySlot(context.Background(), 806, shared, 1)
	require.ErrorIs(t, err, service.ErrConcurrencyPolicyAdmissionStale)
	require.False(t, acquired)
	require.Equal(t, 2, cache.acquireCalls, "stale repair must be bounded to one reload and one retry")
	require.Equal(t, 1, cache.readerCalls)

	cache = &staleUserPolicyCache{freshPolicy: split, freshGeneration: 2, denyAfterReload: true}
	cache.waitAllowed = true
	helper = NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, 5*time.Millisecond)
	c, _ := newHelperTestContext(http.MethodPost, "/v1/messages")
	streamStarted := false
	release, err := helper.AcquireUserPolicySlotWithWait(c, 806, shared, 1, false, &streamStarted)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	require.Equal(t, 3, cache.acquireCalls, "waiting must reuse the freshly reloaded policy")
	require.Equal(t, 1, cache.readerCalls, "waiting must not reload the same stale snapshot on every tick")
}
