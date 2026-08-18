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

// sealedReplayFixture stands in for the sealed blob the service hands down.
// Deliberately NOT valid JSON or valid UTF-8: this layer must treat it as
// opaque bytes and hand them back unchanged, and the round trip through
// EVAL + Redis + go-redis has to be binary-safe to do that. A test using a
// printable string would not notice if it were not.
var sealedReplayFixture = []byte{0x00, 0xff, 0x7b, 0x22, 0x01, 0x80, 0xfe, 0x0a, 0x00, 0x2a}

// markRotatedFixture stores a record and returns the tombstone a rotation at
// `now` would write, so each test drives the script the same way
// OAuthTokenService.ExchangeRefreshToken does.
func markRotatedFixture(now time.Time) (*service.RefreshTokenData, *service.RefreshTokenData, []byte) {
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
	return data, &tomb, sealedReplayFixture
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
	require.Empty(t, res.SealedReplay, "the winner is not a replay")

	// Far outside the grace: the loser is genuine reuse.
	res2, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, res2.Outcome, "a replay long after the rotation must report reuse")
	require.Equal(t, "family-1", res2.Data.FamilyID)
	require.Empty(t, res2.SealedReplay)
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
	// Byte-identical, including the NUL and the non-UTF-8 bytes: this layer
	// stores and returns the sealed blob without interpreting it.
	require.Equal(t, sealedReplayFixture, inside.SealedReplay, "the grace must return the sealed pair the rotation stored, unchanged")

	// 90 seconds as a literal, not service.RefreshReuseGracePeriod+1: a test
	// whose clock is expressed in terms of the constant it is checking slides
	// along with that constant and stops discriminating a widened window.
	outside, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(90*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, outside.Outcome, "90s after the rotation is outside the grace, i.e. reuse")
	require.Empty(t, outside.SealedReplay)
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
	// Literal, not service.RefreshReplayRetention: an expectation written in
	// terms of the constant it is checking slides with that constant and
	// asserts nothing about its value. The value itself is pinned once, in
	// TestRefreshGraceConstants.
	require.LessOrEqual(t, ttlBefore, 2*time.Minute)

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

	// 3 minutes as a literal, against a 2-minute retention. Written as
	// service.RefreshReplayRetention+1s this proved only "the key expires at
	// whatever the retention happens to be" — the review demonstrated that a
	// 5x widening of the retention was invisible to the entire suite.
	mr.FastForward(3 * time.Minute)
	require.False(t, mr.Exists(refreshReplayKey(hash)), "the sealed replay pair must not outlive its retention")

	// The tombstone itself survives, so a much later replay is still
	// traceable to its family.
	stored, err := c.GetRefreshToken(ctx, hash)
	require.NoError(t, err)
	require.True(t, stored.Rotated)
}

// TestRefreshTokenCache_SupersessionClosesPredecessorWindow drives F1's fix
// through the real Lua: claiming a rotation must, in the SAME EVAL,
// overwrite its predecessor's replay slot with the supersession marker, and
// a later replay of that predecessor must then read as reuse even though its
// own 60 seconds have not elapsed.
func TestRefreshTokenCache_SupersessionClosesPredecessorWindow(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	ancestor := "token-hash-n0"
	current := "token-hash-n1"

	// N0 is rotated first: it has no predecessor, so nothing is superseded.
	n0Data, n0Tomb, _ := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, ancestor, n0Data, time.Hour))
	res, err := c.MarkRotated(ctx, ancestor, n0Tomb, sealedReplayFixture, now)
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationClaimed, res.Outcome)
	require.Equal(t, sealedReplayFixture, mustGet(t, mr, refreshReplayKey(ancestor)))

	// N1 is stored with N0 recorded as its predecessor, then rotated 10s later.
	n1Data, n1Tomb, _ := markRotatedFixture(now.Add(10 * time.Second))
	n1Data.PredecessorHash = ancestor
	n1Tomb.PredecessorHash = ancestor
	require.NoError(t, c.StoreRefreshToken(ctx, current, n1Data, time.Hour))
	res, err = c.MarkRotated(ctx, current, n1Tomb, sealedReplayFixture, now.Add(10*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationClaimed, res.Outcome)

	// N0's slot now holds the marker, not the ciphertext: the secret is gone
	// AND the slot still distinguishes supersession from eviction.
	require.Equal(t, []byte(service.RefreshReplaySupersededMarker), mustGet(t, mr, refreshReplayKey(ancestor)))

	// t0+30s: inside N0's own window, but N0 is two generations stale.
	superseded, err := c.MarkRotated(ctx, ancestor, n0Tomb, sealedReplayFixture, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, superseded.Outcome,
		"a superseded ancestor must read as reuse, not as a forgivable replay")
	require.Empty(t, superseded.SealedReplay)

	// N1, the token that IS immediately previous, keeps its window.
	stillCurrent, err := c.MarkRotated(ctx, current, n1Tomb, sealedReplayFixture, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationGraceReplay, stillCurrent.Outcome)
	require.Equal(t, sealedReplayFixture, stillCurrent.SealedReplay)
}

// TestRefreshTokenCache_MissingPairInsideWindowIsNotReuse covers the
// eviction arm: the slot's TTL is strictly longer than the window, so a slot
// that is absent inside the window means Redis dropped it, not that it
// expired. That must not be reported as theft.
func TestRefreshTokenCache_MissingPairInsideWindowIsNotReuse(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-evicted"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	_, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)

	mr.Del(refreshReplayKey(hash)) // maxmemory eviction

	evicted, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationGraceUnavailable, evicted.Outcome,
		"an evicted slot inside the window is an infrastructure fault, not reuse")

	// Outside the window it is reuse again, evicted or not.
	late, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(90*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, late.Outcome)
}

// TestRefreshTokenCache_ToleratesNegativeSkew asserts the replica-skew
// allowance: a replay that appears to arrive a couple of seconds before the
// rotation it replays is still forgiven, but a wildly backwards clock is not.
func TestRefreshTokenCache_ToleratesNegativeSkew(t *testing.T) {
	c, _ := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-skew"
	data, tomb, replay := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))
	_, err := c.MarkRotated(ctx, hash, tomb, replay, now)
	require.NoError(t, err)

	behind, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(-2*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationGraceReplay, behind.Outcome,
		"2s of replica clock skew must not turn a benign replay into a revocation")

	wild, err := c.MarkRotated(ctx, hash, tomb, replay, now.Add(-10*time.Minute))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, wild.Outcome)
}

// TestRefreshTokenCache_RefusesToClaimWithoutPair is the guard that makes it
// safe for the service to skip minting when it already knows the record is
// rotated: an un-rotated record must never be tombstoned with nothing to
// hand back, since that would burn a live token and make every subsequent
// replay inside the window look like theft.
func TestRefreshTokenCache_RefusesToClaimWithoutPair(t *testing.T) {
	c, mr := newRefreshTokenTestCache(t)
	ctx := context.Background()
	now := time.Now()

	hash := "token-hash-no-pair"
	data, tomb, _ := markRotatedFixture(now)
	require.NoError(t, c.StoreRefreshToken(ctx, hash, data, time.Hour))

	_, err := c.MarkRotated(ctx, hash, tomb, nil, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refused to claim")

	stored, err := c.GetRefreshToken(ctx, hash)
	require.NoError(t, err)
	require.False(t, stored.Rotated, "the refusal must not have tombstoned the record")
	require.False(t, mr.Exists(refreshReplayKey(hash)))

	// But a caller that already knows the record is rotated may pass no pair
	// and still get its verdict -- that is what keeps theft detection off the
	// signing-key path.
	_, err = c.MarkRotated(ctx, hash, tomb, sealedReplayFixture, now)
	require.NoError(t, err)
	verdict, err := c.MarkRotated(ctx, hash, tomb, nil, now.Add(90*time.Second))
	require.NoError(t, err)
	require.Equal(t, service.RefreshRotationReuse, verdict.Outcome)
}

func mustGet(t *testing.T, mr *miniredis.Miniredis, key string) []byte {
	t.Helper()
	v, err := mr.Get(key)
	require.NoError(t, err)
	return []byte(v)
}

// TestRefreshGraceConstants pins the three timing values in one place, so
// that every other test can express its clock in literals without any of
// them silently tracking a changed constant.
//
// The last assertion is the load-bearing one, not decoration: because the
// replay slot outlives the window, a slot that is MISSING inside the window
// cannot be natural expiry — which is what lets the script treat that case
// as eviction (an infrastructure fault, 500) instead of as theft. Invert
// this relationship and every late replay silently becomes a 500 instead of
// revoking a stolen token.
func TestRefreshGraceConstants(t *testing.T) {
	require.Equal(t, 60*time.Second, service.RefreshReuseGracePeriod)
	require.Equal(t, 2*time.Minute, service.RefreshReplayRetention)
	require.Equal(t, 5*time.Second, service.RefreshGraceClockSkewAllowance)
	require.Greater(t, service.RefreshReplayRetention, service.RefreshReuseGracePeriod,
		"the replay slot must outlive the grace window, or a missing slot inside the window becomes ambiguous between expiry and eviction")
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

	_, err := c.MarkRotated(ctx, "does-not-exist", &service.RefreshTokenData{}, sealedReplayFixture, time.Now())
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
