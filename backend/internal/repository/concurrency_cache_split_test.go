//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func splitConcurrencyPolicy(live, requests int) service.ConcurrencyPolicy {
	return service.ConcurrencyPolicy{
		Live:    &live,
		Request: &requests,
	}
}

func TestSplitConcurrency_IndependentBudgetsAcrossPolicyModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		accountSplit  bool
		userSplit     bool
		wantLiveScope string
		wantReqScope  string
	}{
		{name: "shared_shared", wantLiveScope: "account", wantReqScope: "account"},
		{name: "split_shared", accountSplit: true, wantLiveScope: "account", wantReqScope: "account"},
		{name: "shared_split", userSplit: true, wantLiveScope: "account", wantReqScope: "account"},
		{name: "split_split", accountSplit: true, userSplit: true, wantLiveScope: "account", wantReqScope: "account"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mini := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
			t.Cleanup(func() { _ = rdb.Close() })
			cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
			ctx := context.Background()

			accountPolicy := service.ConcurrencyPolicy{Shared: 6}
			if tt.accountSplit {
				accountPolicy = splitConcurrencyPolicy(4, 2)
			}
			userPolicy := service.ConcurrencyPolicy{Shared: 6}
			if tt.userSplit {
				userPolicy = splitConcurrencyPolicy(4, 2)
			}
			require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 10, accountPolicy, 1))
			require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 20, userPolicy, 1))

			admit := func(class service.ConcurrencyClass, id string) service.ConcurrencyAdmissionRequest {
				return service.ConcurrencyAdmissionRequest{
					AccountID:         10,
					AccountPolicy:     accountPolicy,
					AccountGeneration: 1,
					UserID:            20,
					UserPolicy:        userPolicy,
					UserGeneration:    1,
					APIKeyID:          30,
					Class:             class,
					ReservationID:     id,
				}
			}

			for i := 0; i < 2; i++ {
				result, err := cache.AcquireConcurrency(ctx, admit(service.ConcurrencyClassRequest, "request-"+string(rune('a'+i))))
				require.NoError(t, err)
				require.True(t, result.Acquired, "request %d should fit its independent budget", i)
			}
			for i := 0; i < 4; i++ {
				result, err := cache.AcquireConcurrency(ctx, admit(service.ConcurrencyClassLive, "live-"+string(rune('a'+i))))
				require.NoError(t, err)
				require.True(t, result.Acquired, "live %d should fit its independent budget", i)
			}

			result, err := cache.AcquireConcurrency(ctx, admit(service.ConcurrencyClassLive, "live-over-cap"))
			require.NoError(t, err)
			require.False(t, result.Acquired)
			require.Equal(t, tt.wantLiveScope, result.DeniedScope)

			result, err = cache.AcquireConcurrency(ctx, admit(service.ConcurrencyClassRequest, "request-over-cap"))
			require.NoError(t, err)
			require.False(t, result.Acquired)
			require.Equal(t, tt.wantReqScope, result.DeniedScope)
		})
	}
}

func TestSplitConcurrency_UserBudgetSpansAccountsAndAPIKeys(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	accountPolicy := splitConcurrencyPolicy(4, 2)
	userPolicy := splitConcurrencyPolicy(2, 2)
	for accountID := int64(101); accountID <= 103; accountID++ {
		require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, accountID, accountPolicy, 1))
	}
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 99, userPolicy, 1))

	for i, accountID := range []int64{101, 102} {
		result, err := cache.AcquireConcurrency(ctx, service.ConcurrencyAdmissionRequest{
			AccountID:         accountID,
			AccountPolicy:     accountPolicy,
			AccountGeneration: 1,
			UserID:            99,
			UserPolicy:        userPolicy,
			UserGeneration:    1,
			APIKeyID:          int64(501 + i),
			Class:             service.ConcurrencyClassLive,
			ReservationID:     "cross-account-live-" + string(rune('a'+i)),
		})
		require.NoError(t, err)
		require.True(t, result.Acquired)
	}

	result, err := cache.AcquireConcurrency(ctx, service.ConcurrencyAdmissionRequest{
		AccountID:         103,
		AccountPolicy:     accountPolicy,
		AccountGeneration: 1,
		UserID:            99,
		UserPolicy:        userPolicy,
		UserGeneration:    1,
		APIKeyID:          503,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "cross-account-live-over-cap",
	})
	require.NoError(t, err)
	require.False(t, result.Acquired)
	require.Equal(t, "user", result.DeniedScope)
	require.Equal(t, int64(1), mustZCard(t, rdb, liveAPIKeySlotKey(501)))
	require.Equal(t, int64(1), mustZCard(t, rdb, liveAPIKeySlotKey(502)))
	require.Equal(t, int64(0), mustZCard(t, rdb, liveAPIKeySlotKey(503)))
}

func TestSplitConcurrency_FailsClosedForAbsentOrMismatchedPolicyAuthority(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	policy := splitConcurrencyPolicy(4, 2)

	request := service.ConcurrencyAdmissionRequest{
		AccountID:         1,
		AccountPolicy:     policy,
		AccountGeneration: 1,
		UserID:            2,
		UserPolicy:        policy,
		UserGeneration:    1,
		APIKeyID:          3,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "fail-closed",
	}
	_, err := cache.AcquireConcurrency(ctx, request)
	require.Error(t, err, "absent policy hashes must not become unlimited")

	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, request.AccountID, policy, 2))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, request.UserID, policy, 1))
	request.AccountGeneration = 1
	_, err = cache.AcquireConcurrency(ctx, request)
	require.Error(t, err, "generation mismatch must fail closed")

	mini.HSet(accountConcurrencyPolicyKey(request.AccountID), "mode", "split", "generation", "2", "shared", "", "live", "", "request", "2")
	request.AccountGeneration = 2
	_, err = cache.AcquireConcurrency(ctx, request)
	require.Error(t, err, "malformed policy authority must fail closed")
}

func mustZCard(t *testing.T, rdb *redis.Client, key string) int64 {
	t.Helper()
	count, err := rdb.ZCard(context.Background(), key).Result()
	require.NoError(t, err)
	return count
}
