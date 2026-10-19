//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

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

func TestOrdinaryAccountAdmission_UsesPublishedRequestBudgetWithLiveOccupancy(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	concurrency := service.NewConcurrencyService(cache)
	ctx := context.Background()
	live, requests := 4, 2
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 701, service.ConcurrencyPolicy{
		Live:    &live,
		Request: &requests,
	}, 1))

	// This is the actual mixed Redis occupancy that exposed the production
	// defect: two Live leases are present, but the ordinary request budget is
	// still empty and must admit exactly two requests.
	now := float64(time.Now().Unix())
	for i := 0; i < 2; i++ {
		require.NoError(t, rdb.ZAdd(ctx, liveAccountSlotKey(701), redis.Z{
			Score:  now,
			Member: "live-" + string(rune('a'+i)),
		}).Err())
	}

	first, err := concurrency.AcquireAccountSlot(ctx, 701, requests)
	require.NoError(t, err)
	require.True(t, first.Acquired, "Live occupancy must not consume split request capacity")
	second, err := concurrency.AcquireAccountSlot(ctx, 701, requests)
	require.NoError(t, err)
	require.True(t, second.Acquired)
	third, err := concurrency.AcquireAccountSlot(ctx, 701, requests)
	require.NoError(t, err)
	require.False(t, third.Acquired, "the third ordinary request must hit the request cap")

	first.ReleaseFunc()
	second.ReleaseFunc()
}

func TestAccountOnlyAdmissionRejectsLiveClass(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	live, requests := 4, 2
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 711, service.ConcurrencyPolicy{
		Live: &live, Request: &requests,
	}, 1))

	invalid := service.AccountConcurrencyAdmissionRequest{
		AccountID: 711, Class: service.ConcurrencyClassLive, ReservationID: "account-only-live",
	}
	require.Error(t, invalid.Validate(), "the account-only boundary is reserved for ordinary requests")
	_, err := cache.AcquireAccountConcurrency(ctx, invalid)
	require.Error(t, err)
	require.Error(t, cache.ReleaseAccountConcurrency(ctx, invalid))

	// Exercise the Lua boundary directly too, so a future Go caller cannot
	// bypass the request-only class restriction.
	raw, err := acquireAccountConcurrencyScript.Run(ctx, rdb, []string{
		accountConcurrencyPolicyKey(711), accountSlotKey(711), liveAccountSlotKey(711),
	}, string(service.ConcurrencyClassLive), 900, "lua-account-only-live").Result()
	require.NoError(t, err)
	status, err := redisScriptInt64At(raw, 0)
	require.NoError(t, err)
	require.EqualValues(t, -10, status)
}

func TestOrdinaryAccountAdmission_ReverseOrderAndSharedMode(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	concurrency := service.NewConcurrencyService(cache)

	t.Run("split_request_occupancy_does_not_consume_live_budget", func(t *testing.T) {
		live, requests := 4, 2
		policy := service.ConcurrencyPolicy{Live: &live, Request: &requests}
		require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 702, policy, 1))

		requestLeases := make([]*service.AcquireResult, 0, requests)
		for i := 0; i < requests; i++ {
			lease, err := concurrency.AcquireAccountPolicySlot(ctx, 702, service.ConcurrencyClassRequest, 999)
			require.NoError(t, err)
			require.True(t, lease.Acquired)
			requestLeases = append(requestLeases, lease)
		}
		for i := 0; i < live; i++ {
			require.NoError(t, rdb.ZAdd(ctx, liveAccountSlotKey(702), redis.Z{
				Score: float64(time.Now().Unix()), Member: "reverse-live-" + string(rune('a'+i)),
			}).Err())
		}

		requestLease, err := concurrency.AcquireAccountPolicySlot(ctx, 702, service.ConcurrencyClassRequest, 999)
		require.NoError(t, err)
		require.False(t, requestLease.Acquired)
		require.Equal(t, service.ConcurrencyClassRequest, requestLease.CapacityError.Class)
		require.Equal(t, "account", requestLease.CapacityError.Scope)
		for _, lease := range requestLeases {
			lease.ReleaseFunc()
		}
		require.NoError(t, rdb.Del(ctx, liveAccountSlotKey(702)).Err())
	})

	t.Run("shared_mode_keeps_combined_cap", func(t *testing.T) {
		require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 703, service.ConcurrencyPolicy{Shared: 4}, 1))
		for i := 0; i < 2; i++ {
			require.NoError(t, rdb.ZAdd(ctx, liveAccountSlotKey(703), redis.Z{
				Score: float64(time.Now().Unix()), Member: "shared-live-" + string(rune('a'+i)),
			}).Err())
		}
		requestLeases := make([]*service.AcquireResult, 0, 2)
		for i := 0; i < 2; i++ {
			lease, err := concurrency.AcquireAccountPolicySlot(ctx, 703, service.ConcurrencyClassRequest, 0)
			require.NoError(t, err)
			require.True(t, lease.Acquired)
			requestLeases = append(requestLeases, lease)
		}
		denied, err := concurrency.AcquireAccountPolicySlot(ctx, 703, service.ConcurrencyClassRequest, 0)
		require.NoError(t, err)
		require.False(t, denied.Acquired, "shared mode must count request plus Live occupancy")

		for _, lease := range requestLeases {
			lease.ReleaseFunc()
		}
		require.NoError(t, rdb.Del(ctx, liveAccountSlotKey(703)).Err())
	})
}

func TestOrdinaryAccountAdmission_CurrentAuthorityOverridesStaleUnlimitedAndSharedSnapshots(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	concurrency := service.NewConcurrencyService(cache)

	// The caller still carries the old shared/unlimited snapshot and passes zero
	// as its legacy max. The script must use the acknowledged split authority.
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 704, service.ConcurrencyPolicy{Shared: -1}, 1))
	first, err := concurrency.AcquireAccountPolicySlot(ctx, 704, service.ConcurrencyClassRequest, 0)
	require.NoError(t, err)
	require.True(t, first.Acquired)
	live, requests := 4, 2
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 704, service.ConcurrencyPolicy{Live: &live, Request: &requests}, 2))
	second, err := concurrency.AcquireAccountPolicySlot(ctx, 704, service.ConcurrencyClassRequest, 0)
	require.NoError(t, err)
	require.True(t, second.Acquired)
	third, err := concurrency.AcquireAccountPolicySlot(ctx, 704, service.ConcurrencyClassRequest, 0)
	require.NoError(t, err)
	require.False(t, third.Acquired, "stale unlimited caller must not bypass the published request cap")
	first.ReleaseFunc()
	second.ReleaseFunc()

	// The reverse transition also uses the current mode and cap, rather than a
	// stale split request limit supplied by the scheduler snapshot.
	live, requests = 4, 2
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 705, service.ConcurrencyPolicy{Live: &live, Request: &requests}, 1))
	requestLeases := make([]*service.AcquireResult, 0, requests)
	for i := 0; i < requests; i++ {
		lease, err := concurrency.AcquireAccountPolicySlot(ctx, 705, service.ConcurrencyClassRequest, 2)
		require.NoError(t, err)
		require.True(t, lease.Acquired)
		requestLeases = append(requestLeases, lease)
	}
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 705, service.ConcurrencyPolicy{Shared: 3}, 2))
	thirdShared, err := concurrency.AcquireAccountPolicySlot(ctx, 705, service.ConcurrencyClassRequest, 0)
	require.NoError(t, err)
	require.True(t, thirdShared.Acquired, "current shared cap should replace the stale split request cap")
	fourthShared, err := concurrency.AcquireAccountPolicySlot(ctx, 705, service.ConcurrencyClassRequest, 0)
	require.NoError(t, err)
	require.False(t, fourthShared.Acquired)
	for _, lease := range requestLeases {
		lease.ReleaseFunc()
	}
	thirdShared.ReleaseFunc()
}

func TestOrdinaryAccountAdmission_FailsClosedForMissingOrMalformedAuthority(t *testing.T) {
	ctx := context.Background()
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	concurrency := service.NewConcurrencyService(cache)

	_, err := concurrency.AcquireAccountPolicySlot(ctx, 706, service.ConcurrencyClassRequest, 0)
	require.ErrorIs(t, err, service.ErrConcurrencyPolicyAdmissionUnavailable)
	require.Equal(t, int64(0), mustZCard(t, rdb, accountSlotKey(706)))

	live, requests := 4, 2
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 706, service.ConcurrencyPolicy{Live: &live, Request: &requests}, 1))
	mini.HSet(accountConcurrencyPolicyKey(706), "mode", "split", "shared", "", "live", "4", "request", "not-an-integer")
	_, err = concurrency.AcquireAccountPolicySlot(ctx, 706, service.ConcurrencyClassRequest, 0)
	require.ErrorIs(t, err, service.ErrConcurrencyPolicyAdmissionUnavailable)
	require.Equal(t, int64(0), mustZCard(t, rdb, accountSlotKey(706)))
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

func TestSplitConcurrency_StaleGenerationIsDistinctFromUnavailableAuthority(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	policy := splitConcurrencyPolicy(4, 2)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 1, policy, 2))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 2, policy, 2))

	_, err := cache.AcquireConcurrency(ctx, service.ConcurrencyAdmissionRequest{
		AccountID:         1,
		AccountPolicy:     policy,
		AccountGeneration: 1,
		UserID:            2,
		UserPolicy:        policy,
		UserGeneration:    2,
		APIKeyID:          3,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "stale-generation",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "stale", "generation mismatch must be recoverable as a stale snapshot")
}

func TestSplitConcurrency_ReleaseRejectsMalformedOwnershipRequest(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	policy := splitConcurrencyPolicy(4, 2)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(context.Background(), 1, policy, 1))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(context.Background(), 2, policy, 1))

	err := cache.ReleaseConcurrency(context.Background(), service.ConcurrencyAdmissionRequest{
		AccountID:         1,
		UserID:            2,
		APIKeyID:          0,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "malformed-release",
		AccountPolicy:     policy,
		UserPolicy:        policy,
		AccountGeneration: 1,
		UserGeneration:    1,
	})
	require.Error(t, err, "release must not silently accept an invalid API-key ownership tuple")
}

func TestSplitConcurrency_LoadSnapshotsUseClassSpecificOccupancy(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	now := float64(time.Now().Unix())
	for _, accountID := range []int64{701, 702, 703} {
		for index := 0; index < 2; index++ {
			require.NoError(t, rdb.ZAdd(context.Background(), accountSlotKey(accountID), redis.Z{Score: now, Member: "request-" + string(rune('a'+index))}).Err())
		}
		for index := 0; index < 4; index++ {
			require.NoError(t, rdb.ZAdd(context.Background(), liveAccountSlotKey(accountID), redis.Z{Score: now, Member: "live-" + string(rune('a'+index))}).Err())
		}
	}

	loadMap, err := cache.GetAccountsLoadBatch(context.Background(), []service.AccountWithConcurrency{
		{ID: 701, MaxConcurrency: 2, AdmissionClass: service.ConcurrencyClassRequest, SplitPolicy: true},
		{ID: 702, MaxConcurrency: 4, AdmissionClass: service.ConcurrencyClassLive, SplitPolicy: true},
		{ID: 703, MaxConcurrency: 6},
	})
	require.NoError(t, err)
	require.Equal(t, 2, loadMap[701].CurrentConcurrency)
	require.Equal(t, 100, loadMap[701].LoadRate)
	require.Equal(t, 4, loadMap[702].CurrentConcurrency)
	require.Equal(t, 100, loadMap[702].LoadRate)
	require.Equal(t, 6, loadMap[703].CurrentConcurrency)
	require.Equal(t, 100, loadMap[703].LoadRate)
}

func TestSplitConcurrency_RefreshPreservesOwnedLeaseAcrossPolicyGenerationEdit(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	initial := splitConcurrencyPolicy(4, 2)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 301, initial, 1))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 302, initial, 1))
	request := service.ConcurrencyAdmissionRequest{
		AccountID:         301,
		AccountPolicy:     initial,
		AccountGeneration: 1,
		UserID:            302,
		UserPolicy:        initial,
		UserGeneration:    1,
		APIKeyID:          303,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "refresh-across-policy-edit",
	}
	acquired, err := cache.AcquireConcurrency(ctx, request)
	require.NoError(t, err)
	require.True(t, acquired.Acquired)

	updated := splitConcurrencyPolicy(5, 3)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 301, updated, 2))
	refreshed, err := cache.RefreshConcurrency(ctx, request)
	require.NoError(t, err, "an active owner must remain refreshable after a policy generation edit")
	require.True(t, refreshed)
	require.Equal(t, int64(1), mustZCard(t, rdb, liveAccountSlotKey(301)))
	require.Equal(t, int64(1), mustZCard(t, rdb, liveUserSlotKey(302)))
}

func TestSplitConcurrency_RejectsReservationIDCrossClassCollision(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	policy := splitConcurrencyPolicy(4, 2)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 401, policy, 1))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 402, policy, 1))

	request := func(class service.ConcurrencyClass) service.ConcurrencyAdmissionRequest {
		return service.ConcurrencyAdmissionRequest{
			AccountID:         401,
			AccountPolicy:     policy,
			AccountGeneration: 1,
			UserID:            402,
			UserPolicy:        policy,
			UserGeneration:    1,
			APIKeyID:          403,
			Class:             class,
			ReservationID:     "cross-class-collision",
		}
	}

	live, err := cache.AcquireConcurrency(ctx, request(service.ConcurrencyClassLive))
	require.NoError(t, err)
	require.True(t, live.Acquired)
	_, err = cache.AcquireConcurrency(ctx, request(service.ConcurrencyClassRequest))
	require.Error(t, err, "a request reservation must not reuse a Live reservation ID")
	require.Equal(t, int64(0), mustZCard(t, rdb, accountSlotKey(401)))
	require.Equal(t, int64(1), mustZCard(t, rdb, liveAccountSlotKey(401)))
	require.NoError(t, cache.ReleaseConcurrency(ctx, request(service.ConcurrencyClassLive)))

	regular, err := cache.AcquireConcurrency(ctx, request(service.ConcurrencyClassRequest))
	require.NoError(t, err)
	require.True(t, regular.Acquired)
	_, err = cache.AcquireConcurrency(ctx, request(service.ConcurrencyClassLive))
	require.Error(t, err, "a Live reservation must not reuse a request reservation ID")
	require.Equal(t, int64(1), mustZCard(t, rdb, accountSlotKey(401)))
	require.Equal(t, int64(0), mustZCard(t, rdb, liveAccountSlotKey(401)))
}

func TestSplitConcurrency_FractionalRedisAuthorityFailsClosed(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	policy := splitConcurrencyPolicy(4, 2)
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 501, policy, 1))
	require.NoError(t, cache.PublishUserConcurrencyPolicy(ctx, 502, policy, 1))

	request := service.ConcurrencyAdmissionRequest{
		AccountID:         501,
		AccountPolicy:     policy,
		AccountGeneration: 1,
		UserID:            502,
		UserPolicy:        policy,
		UserGeneration:    1,
		APIKeyID:          503,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "fractional-authority",
	}
	for field, value := range map[string]string{
		"shared":  "2.5",
		"live":    "4.5",
		"request": "2.5",
	} {
		mini.HSet(accountConcurrencyPolicyKey(501), "mode", "split", "shared", "", "live", "4", "request", "2", field, value)
		_, err := cache.AcquireConcurrency(ctx, request)
		require.Error(t, err, "fractional %s authority must fail closed", field)
		mini.HSet(accountConcurrencyPolicyKey(501), "mode", "split", "shared", "", "live", "4", "request", "2")
	}

	mini.HSet(accountConcurrencyPolicyKey(501), "generation", "1.5", "mode", "split", "shared", "", "live", "4", "request", "2")
	_, err := cache.AcquireConcurrency(ctx, request)
	require.Error(t, err, "fractional generation must fail closed")
	err = cache.PublishAccountConcurrencyPolicy(ctx, 501, policy, 2)
	require.Error(t, err, "publication must not overwrite a fractional generation")
}

func mustZCard(t *testing.T, rdb *redis.Client, key string) int64 {
	t.Helper()
	count, err := rdb.ZCard(context.Background(), key).Result()
	require.NoError(t, err)
	return count
}
