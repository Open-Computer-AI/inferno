//go:build unit

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestUserPolicyAdmission_SplitAndSharedOccupancy(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	liveLimit, requestLimit := 4, 2
	split := service.ConcurrencyPolicy{Live: &liveLimit, Request: &requestLimit}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 601, split, 1))

	request := func(id string, generation int64) service.UserConcurrencyAdmissionRequest {
		return service.UserConcurrencyAdmissionRequest{
			UserID:         601,
			UserPolicy:     split,
			UserGeneration: generation,
			Class:          service.ConcurrencyClassRequest,
			ReservationID:  id,
		}
	}
	first, err := cache.AcquireUserConcurrency(ctx, request("user-request-1", 1))
	require.NoError(t, err)
	require.True(t, first.Acquired)
	second, err := cache.AcquireUserConcurrency(ctx, request("user-request-2", 1))
	require.NoError(t, err)
	require.True(t, second.Acquired)
	third, err := cache.AcquireUserConcurrency(ctx, request("user-request-3", 1))
	require.NoError(t, err)
	require.False(t, third.Acquired)

	require.NoError(t, cache.ReleaseUserConcurrency(ctx, request("user-request-1", 1)))
	now := time.Now().Unix()
	require.NoError(t, rdb.ZAdd(ctx, liveUserSlotKey(601),
		redis.Z{Score: float64(now), Member: "existing-live-1"},
		redis.Z{Score: float64(now), Member: "existing-live-2"},
	).Err())
	independent, err := cache.AcquireUserConcurrency(ctx, request("user-request-after-live", 1))
	require.NoError(t, err)
	require.True(t, independent.Acquired, "split request budget must not count Live members")
	require.NoError(t, cache.ReleaseUserConcurrency(ctx, request("user-request-after-live", 1)))

	shared := service.ConcurrencyPolicy{Shared: 2}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 601, shared, 2))
	denied, err := cache.AcquireUserConcurrency(ctx, service.UserConcurrencyAdmissionRequest{
		UserID:         601,
		UserPolicy:     shared,
		UserGeneration: 2,
		Class:          service.ConcurrencyClassRequest,
		ReservationID:  "user-request-shared",
	})
	require.NoError(t, err)
	require.False(t, denied.Acquired, "legacy shared user budget must include Live occupancy")
}

func TestUserPolicyAdmission_RefreshPreservesLeaseAcrossGenerationEdit(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	liveLimit, requestLimit := 4, 2
	initial := service.ConcurrencyPolicy{Live: &liveLimit, Request: &requestLimit}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 602, initial, 1))
	request := service.UserConcurrencyAdmissionRequest{
		UserID:         602,
		UserPolicy:     initial,
		UserGeneration: 1,
		Class:          service.ConcurrencyClassRequest,
		ReservationID:  "user-refresh-after-edit",
	}
	acquired, err := cache.AcquireUserConcurrency(ctx, request)
	require.NoError(t, err)
	require.True(t, acquired.Acquired)
	updated := service.ConcurrencyPolicy{Live: &liveLimit, Request: intPtrForUserPolicyTest(3)}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 602, updated, 2))
	refreshed, err := cache.RefreshUserConcurrency(ctx, request)
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, cache.ReleaseUserConcurrency(ctx, request))
}

func TestUserPolicyAdmission_StaleGenerationUsesDistinctSentinel(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	live, requests := 4, 2
	policy := service.ConcurrencyPolicy{Live: &live, Request: &requests}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 603, policy, 2))

	_, err := cache.AcquireUserConcurrency(ctx, service.UserConcurrencyAdmissionRequest{
		UserID:         603,
		UserPolicy:     policy,
		UserGeneration: 1,
		Class:          service.ConcurrencyClassRequest,
		ReservationID:  "stale-user-sentinel",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrConcurrencyPolicyAdmissionStale), "valid newer authority must be distinguishable from malformed authority")
	require.False(t, errors.Is(err, service.ErrConcurrencyPolicyAdmissionUnavailable))
}

func intPtrForUserPolicyTest(value int) *int {
	return &value
}
