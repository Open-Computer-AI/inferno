//go:build unit

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newRefreshTokenTestCache(t *testing.T) (*refreshTokenCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &refreshTokenCache{rdb: rdb}, mr
}

// TestRefreshTokenCache_MarkRotatedWinnerLoser exercises the Lua script
// (refreshTokenMarkRotatedScript) directly against a real EVAL round trip,
// including go-redis's decoding of its {flag, raw} table reply — the part
// unit tests against a hand-rolled Go fake (internal/service/oauth_token_service_test.go)
// cannot verify, since that fake reimplements the atomicity in Go rather
// than driving the actual Lua script.
func TestRefreshTokenCache_MarkRotatedWinnerLoser(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()

	hash := "token-hash-abc"
	data := &service.RefreshTokenData{
		UserID:    7,
		FamilyID:  "family-1",
		ClientID:  "agent:x",
		Scope:     "inference",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))

	tomb := *data
	tomb.Rotated = true

	got, alreadyRotated, err := c.MarkRotated(ctx, hash, &tomb)
	require.NoError(t, err)
	require.False(t, alreadyRotated, "first call must win")
	require.Equal(t, "family-1", got.FamilyID)
	require.False(t, got.Rotated, "the returned pre-rotation snapshot must show Rotated=false")

	got2, alreadyRotated2, err := c.MarkRotated(ctx, hash, &tomb)
	require.NoError(t, err)
	require.True(t, alreadyRotated2, "second call on the same hash must report it already rotated")
	require.Equal(t, "family-1", got2.FamilyID)
}

// TestRefreshTokenCache_MarkRotatedPreservesTTL asserts the KEEPTTL clause
// actually preserves the existing TTL rather than resetting or dropping it —
// a plain SET without KEEPTTL would make a marked-rotated record immortal
// (no TTL) or, with a naive PTTL-then-SET-PX, race against a concurrent
// expiry.
func TestRefreshTokenCache_MarkRotatedPreservesTTL(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()

	hash := "token-hash-ttl"
	data := &service.RefreshTokenData{FamilyID: "family-ttl", ClientID: "agent:x"}
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, 90*time.Minute))

	tomb := *data
	tomb.Rotated = true
	_, alreadyRotated, err := c.MarkRotated(ctx, hash, &tomb)
	require.NoError(t, err)
	require.False(t, alreadyRotated)

	ttl := mr.TTL(refreshTokenKey(hash))
	require.Greater(t, ttl, time.Duration(0))
	require.LessOrEqual(t, ttl, 90*time.Minute)
}

// TestRefreshTokenCache_MarkRotatedNotFound covers the sentinel-error path:
// a hash with no record must map to service.ErrRefreshTokenNotFound, the
// same sentinel GetRefreshToken uses, so callers can share one errors.Is check.
func TestRefreshTokenCache_MarkRotatedNotFound(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()

	_, _, err := c.MarkRotated(ctx, "does-not-exist", &service.RefreshTokenData{})
	require.ErrorIs(t, err, service.ErrRefreshTokenNotFound)
}

// TestRefreshTokenCache_MarkRotatedConcurrentOnlyOneWins is the atomicity
// guarantee itself, driven directly through Redis (miniredis) rather than
// through OAuthTokenService: N goroutines call MarkRotated on the exact same
// hash at once; EVAL's atomicity must serialize them so exactly one sees
// alreadyRotated=false.
func TestRefreshTokenCache_MarkRotatedConcurrentOnlyOneWins(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()

	hash := "token-hash-concurrent"
	data := &service.RefreshTokenData{FamilyID: "family-concurrent", ClientID: "agent:x"}
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	tomb := *data
	tomb.Rotated = true

	const n = 50
	var wg sync.WaitGroup
	start := make(chan struct{})
	var mu sync.Mutex
	wins, losses := 0, 0
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, alreadyRotated, err := c.MarkRotated(ctx, hash, &tomb)
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			if alreadyRotated {
				losses++
			} else {
				wins++
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, 1, wins, "exactly one concurrent MarkRotated call must win")
	require.Equal(t, n-1, losses)
}
