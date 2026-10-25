package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyCacheIncrementCreateCountUsesFixedWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := NewAPIKeyCache(client)
	ctx := context.Background()
	key := apiKeyCreateCountKey(7)

	count, err := cache.IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, time.Hour, server.TTL(key))

	// 后续创建不得延长窗口，否则持续创建会让计数永不过期。
	server.FastForward(40 * time.Minute)
	count, err = cache.IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Equal(t, 20*time.Minute, server.TTL(key))

	// 其他用户独立计数。
	count, err = cache.IncrementCreateCount(ctx, 8, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	// 窗口到期后重新计数。
	server.FastForward(21 * time.Minute)
	count, err = cache.IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestAPIKeyCacheDecrementCreateCountReleasesReservation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := &apiKeyCache{rdb: client}
	ctx := context.Background()
	key := apiKeyCreateCountKey(7)

	require.NoError(t, client.Set(ctx, key, 2, time.Hour).Err())
	require.NoError(t, cache.DecrementCreateCount(ctx, 7))
	count, err := client.Get(ctx, key).Int64()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	require.NoError(t, cache.DecrementCreateCount(ctx, 7))
	_, err = client.Get(ctx, key).Result()
	require.ErrorIs(t, err, redis.Nil)
}
