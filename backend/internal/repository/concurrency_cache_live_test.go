package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLiveLeaseReplacesRegularSlotsAndCountsTowardLimits(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	accountAcquired, err := regular.AcquireAccountSlot(ctx, 10, 1, "regular-account")
	require.NoError(t, err)
	require.True(t, accountAcquired)
	userAcquired, err := regular.AcquireUserSlot(ctx, 20, 1, "regular-user")
	require.NoError(t, err)
	require.True(t, userAcquired)

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "live-lease", true)
	require.NoError(t, err)
	require.True(t, acquired)
	require.NoError(t, regular.ReleaseAccountSlot(ctx, 10, "regular-account"))
	require.NoError(t, regular.ReleaseUserSlot(ctx, 20, "regular-user"))

	accountCount, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 1, accountCount)
	userCount, err := regular.GetUserConcurrency(ctx, 20)
	require.NoError(t, err)
	require.Equal(t, 1, userCount)
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-blocked")
	require.NoError(t, err)
	require.False(t, accountAcquired)

	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "live-lease")
	require.NoError(t, err)
	require.True(t, refreshed)
	require.NoError(t, live.ReleaseLiveLease(ctx, 10, 20, 30, "live-lease"))
	accountAcquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-allowed")
	require.NoError(t, err)
	require.True(t, accountAcquired)
}

func TestLiveLeaseExpiresWithoutRefresh(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live, ok := regular.(service.LiveConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	acquired, err := live.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, "expired-live", false)
	require.NoError(t, err)
	require.True(t, acquired)

	redisServer.FastForward(61 * time.Second)
	acquired, err = regular.AcquireAccountSlot(ctx, 10, 1, "ordinary-after-expiry")
	require.NoError(t, err)
	require.True(t, acquired)
	refreshed, err := live.RefreshLiveLease(ctx, 10, 20, 30, "expired-live")
	require.NoError(t, err)
	require.False(t, refreshed)
}

func TestLiveLeaseAccountLiveMaxCapsLiveOnlyAndLeavesRegularSlots(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	capped, ok := regular.(service.LiveAccountLiveMaxConcurrencyCache)
	require.True(t, ok)
	ctx := context.Background()

	// Live max 4, total 8: four Live leases fit, the fifth is refused as live-capped.
	for i := 0; i < 4; i++ {
		acquired, liveCapped, err := capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, fmt.Sprintf("live-%d", i), false)
		require.NoError(t, err)
		require.True(t, acquired)
		require.False(t, liveCapped)
	}
	acquired, liveCapped, err := capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, "live-4", false)
	require.NoError(t, err)
	require.False(t, acquired)
	require.True(t, liveCapped)
	count, err := regular.GetAccountConcurrency(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, 4, count, "a refused lease must not be recorded")

	// The four remaining total slots stay open for regular (planner) requests.
	for i := 0; i < 4; i++ {
		ok, err := regular.AcquireAccountSlot(ctx, 10, 8, fmt.Sprintf("regular-%d", i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	ok, err = regular.AcquireAccountSlot(ctx, 10, 8, "regular-over-total")
	require.NoError(t, err)
	require.False(t, ok)

	// An existing lease is still idempotent while the account is at its Live max.
	acquired, liveCapped, err = capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, "live-0", false)
	require.NoError(t, err)
	require.True(t, acquired)
	require.False(t, liveCapped)

	// Releasing a Live lease frees Live room again (3 live + 3 regular of 8).
	require.NoError(t, regular.ReleaseAccountSlot(ctx, 10, "regular-0"))
	live, _ := regular.(service.LiveConcurrencyCache)
	require.NoError(t, live.ReleaseLiveLease(ctx, 10, 20, 30, "live-1"))
	acquired, liveCapped, err = capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, "live-5", false)
	require.NoError(t, err)
	require.True(t, acquired)
	require.False(t, liveCapped)
}

func TestLiveLeaseAccountLiveMaxTotalLimitStillApplies(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	capped := regular.(service.LiveAccountLiveMaxConcurrencyCache)
	ctx := context.Background()

	// Total 8 all taken by regular requests: a Live lease under its Live max is
	// refused by the total check (not live-capped), exactly as before.
	for i := 0; i < 8; i++ {
		ok, err := regular.AcquireAccountSlot(ctx, 10, 8, fmt.Sprintf("regular-%d", i))
		require.NoError(t, err)
		require.True(t, ok)
	}
	acquired, liveCapped, err := capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, "live-0", false)
	require.NoError(t, err)
	require.False(t, acquired)
	require.False(t, liveCapped)

	// With replacement (the scheduler's regular slot is handed over) it fits.
	acquired, liveCapped, err = capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, 4, 20, 0, 30, "live-0", true)
	require.NoError(t, err)
	require.True(t, acquired)
	require.False(t, liveCapped)
}

func TestLiveLeaseWithoutAccountLiveMaxKeepsTotalOnlyBehaviour(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	regular := NewConcurrencyCache(client, 15, 900)
	live := regular.(service.LiveConcurrencyCache)
	capped := regular.(service.LiveAccountLiveMaxConcurrencyCache)
	ctx := context.Background()

	// Legacy method and live max <= 0: Live leases are bounded only by the total.
	for i := 0; i < 4; i++ {
		acquired, err := live.AcquireLiveLease(ctx, 10, 8, 20, 0, 30, fmt.Sprintf("legacy-%d", i), false)
		require.NoError(t, err)
		require.True(t, acquired)
	}
	for i, liveMax := range []int{0, -1, 0, -5} {
		acquired, liveCapped, err := capped.AcquireLiveLeaseWithAccountLiveMax(ctx, 10, 8, liveMax, 20, 0, 30, fmt.Sprintf("off-%d", i), false)
		require.NoError(t, err)
		require.True(t, acquired)
		require.False(t, liveCapped)
	}
	acquired, err := live.AcquireLiveLease(ctx, 10, 8, 20, 0, 30, "legacy-over-total", false)
	require.NoError(t, err)
	require.False(t, acquired)
}
