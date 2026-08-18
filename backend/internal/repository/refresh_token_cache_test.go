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

// markRotatedFixture stores a record and returns the tombstone + replay pair
// a rotation at `now` would write, so each test drives the script the same
// way OAuthTokenService.ExchangeRefreshToken does.
func markRotatedFixture(now time.Time) (*service.RefreshTokenData, *service.RefreshTokenData, *service.RefreshReplayPair) {
	data := &service.RefreshTokenData{
		UserID:    7,
		FamilyID:  "family-1",
		ClientID:  "agent:x",
		Scope:     "inference:invoke",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}
	tomb := *data
	tomb.Rotated = true
	tomb.RotatedAtUnix = now.Unix()
	replay := &service.RefreshReplayPair{
		AccessToken:         "access-token-value",
		RefreshToken:        "art_successor",
		Scope:               data.Scope,
		AccessExpiresAtUnix: now.Add(15 * time.Minute).Unix(),
	}
	return data, &tomb, replay
}

// TestRefreshTokenCache_MarkRotatedWinnerLoser exercises the Lua script
// (refreshTokenMarkRotatedScript) directly against a real EVAL round trip,
// including go-redis's decoding of its {flag, raw, replay} table reply — the
// part unit tests against a hand-rolled Go fake (internal/service/oauth_token_service_test.go)
// cannot verify, since that fake reimplements the atomicity in Go rather
// than driving the actual Lua script.
func TestRefreshTokenCache_MarkRotatedWinnerLoser(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-abc"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))

	res, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationClaimed, res.Outcome, "first call must win")
	require.Equal(t, "family-1", res.Data.FamilyID)
	require.False(t, res.Data.Rotated, "the returned pre-rotation snapshot must show Rotated=false")
	require.Nil(t, res.Replay, "the winner is not a replay")

	// Far outside the grace: the loser is genuine reuse.
	res2, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, res2.Outcome, "a replay long after the rotation must report reuse")
	require.Equal(t, "family-1", res2.Data.FamilyID)
	require.Nil(t, res2.Replay)
}

// TestRefreshTokenCache_MarkRotatedGraceReplay drives the grace branch of the
// script itself. The Go fake in the service package can only prove the
// service reacts correctly to an outcome; this proves the outcome is what
// Redis actually computes — including that cjson reads rotated_at_unix back
// out of the tombstone the previous EVAL wrote.
func TestRefreshTokenCache_MarkRotatedGraceReplay(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-grace"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))

	res, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationClaimed, res.Outcome)

	inside, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationGraceReplay, inside.Outcome, "a replay 30s after rotation is inside the grace")
	require.NotNil(t, inside.Replay)
	require.Equal(t, "art_successor", inside.Replay.RefreshToken, "the grace must return the pair the rotation minted")
	require.Equal(t, "access-token-value", inside.Replay.AccessToken)
	require.Equal(t, "inference:invoke", inside.Replay.Scope)

	// 90 seconds as a literal, not service.RefreshReuseGracePeriod+1: a test
	// whose clock is expressed in terms of the constant it is checking slides
	// along with that constant and stops discriminating a widened window.
	outside, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(90*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, outside.Outcome, "90s after the rotation is outside the grace, i.e. reuse")
	require.Nil(t, outside.Replay)
}

// TestRefreshTokenCache_MarkRotatedGraceIsNotExtended asserts the
// already-rotated branch writes NOTHING: neither the tombstone's
// rotated_at_unix nor the replay key's TTL may move when a replay lands, or a
// stolen token could be replayed indefinitely, one window at a time.
func TestRefreshTokenCache_MarkRotatedGraceIsNotExtended(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-no-extend"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	_, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)

	ttlBefore := mr.TTL(refreshReplayKey(hash))
	require.Greater(t, ttlBefore, time.Duration(0))
	require.LessOrEqual(t, ttlBefore, service.RefreshReplayRetention)

	inside, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationGraceReplay, inside.Outcome)

	require.Equal(t, ttlBefore, mr.TTL(refreshReplayKey(hash)), "a replay must not extend the replay key's TTL")

	stored, err := c.GetRefreshToken(ctx, hash)
	require.NoError(t, err)
	require.Equal(t, now.Unix(), stored.RotatedAtUnix, "a replay must not re-stamp the rotation time")
}

// TestRefreshTokenCache_MarkRotatedReplayKeyExpires proves the RAW tokens in
// the replay pair are not parked in Redis for the tombstone's remaining 30
// days: the replay key carries its own short TTL, so once it lapses the
// record is still there (reuse detection intact) but the usable tokens are
// gone.
func TestRefreshTokenCache_MarkRotatedReplayKeyExpires(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-replay-ttl"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, 30*24*time.Hour))
	_, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)

	mr.FastForward(service.RefreshReplayRetention + time.Second)
	require.False(t, mr.Exists(refreshReplayKey(hash)), "the raw replay pair must not outlive its retention")

	// The tombstone itself survives, so a much later replay is still
	// traceable to its family.
	stored, err := c.GetRefreshToken(ctx, hash)
	require.NoError(t, err)
	require.True(t, stored.Rotated)
}

// TestRefreshTokenCache_DeleteDropsReplayPair asserts a revocation takes the
// raw replay pair with it. Leaving it behind would keep a usable refresh
// token readable in Redis after the session it belongs to was explicitly
// killed.
func TestRefreshTokenCache_DeleteDropsReplayPair(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-family-delete"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	require.NoError(t, c.AddToFamilyTokenSet(ctx, data.FamilyID, hash, time.Hour))
	_, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)
	require.True(t, mr.Exists(refreshReplayKey(hash)))

	require.NoError(t, c.DeleteTokenFamily(ctx, data.FamilyID))
	require.False(t, mr.Exists(refreshReplayKey(hash)), "DeleteTokenFamily must drop the replay pair too")
	require.False(t, mr.Exists(refreshTokenKey(hash)))

	// And the single-token delete path.
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	_, err = c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)
	require.NoError(t, c.DeleteRefreshToken(ctx, hash))
	require.False(t, mr.Exists(refreshReplayKey(hash)), "DeleteRefreshToken must drop the replay pair too")
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
	data, tomb, replay := markRotatedFixture(time.Now())
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, 90*time.Minute))

	res, err := c.MarkRotated(ctx, hash, tomb, replay, time.Now())
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationClaimed, res.Outcome)

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

	_, err := c.MarkRotated(ctx, "does-not-exist", &service.RefreshTokenData{}, &service.RefreshReplayPair{}, time.Now())
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
	now := time.Now()
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))

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
			res, err := c.MarkRotated(ctx, hash, tomb, replay, now)
			require.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			if res.Outcome == service.RefreshRotationClaimed {
				wins++
			} else {
				losses++
			}
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, 1, wins, "exactly one concurrent MarkRotated call must win")
	require.Equal(t, n-1, losses)
}
