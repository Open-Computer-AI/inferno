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

func TestSplitConcurrency_ClassSnapshotsExposeSeparateOccupancyAndExactAggregate(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	ctx := context.Background()
	now := time.Now().Unix()

	require.NoError(t, rdb.ZAdd(ctx, accountSlotKey(10), redis.Z{Score: float64(now), Member: "request-1"}, redis.Z{Score: float64(now + 1), Member: "request-2"}).Err())
	require.NoError(t, rdb.ZAdd(ctx, liveAccountSlotKey(10), redis.Z{Score: float64(now), Member: "live-1"}).Err())
	require.NoError(t, rdb.ZAdd(ctx, userSlotKey(20), redis.Z{Score: float64(now), Member: "request-3"}).Err())
	require.NoError(t, rdb.ZAdd(ctx, liveUserSlotKey(20), redis.Z{Score: float64(now), Member: "live-2"}, redis.Z{Score: float64(now + 1), Member: "live-3"}).Err())
	require.NoError(t, rdb.ZAdd(ctx, apiKeySlotKey(30), redis.Z{Score: float64(now), Member: "request-4"}).Err())
	require.NoError(t, rdb.ZAdd(ctx, liveAPIKeySlotKey(30), redis.Z{Score: float64(now), Member: "live-4"}, redis.Z{Score: float64(now + 1), Member: "live-5"}).Err())

	accounts, err := cache.GetAccountConcurrencySnapshotBatch(ctx, []int64{10})
	require.NoError(t, err)
	require.Equal(t, service.ConcurrencySnapshot{Request: 2, Live: 1, Total: 3}, accounts[10])

	users, err := cache.GetUserConcurrencySnapshotBatch(ctx, []int64{20})
	require.NoError(t, err)
	require.Equal(t, service.ConcurrencySnapshot{Request: 1, Live: 2, Total: 3}, users[20])

	apiKeys, err := cache.GetAPIKeyConcurrencySnapshotBatch(ctx, []int64{30})
	require.NoError(t, err)
	require.Equal(t, service.ConcurrencySnapshot{Request: 1, Live: 2, Total: 3}, apiKeys[30])
}

func TestSplitConcurrency_ClassSnapshotReadFailureIsNotZero(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cache := &concurrencyCache{rdb: rdb, slotTTLSeconds: 900}
	mini.Close()

	snapshots, err := cache.GetAccountConcurrencySnapshotBatch(context.Background(), []int64{10})
	require.Error(t, err)
	require.Nil(t, snapshots)
}
