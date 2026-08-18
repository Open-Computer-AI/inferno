package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	refreshTokenKeyPrefix   = "refresh_token:"
	userRefreshTokensPrefix = "user_refresh_tokens:"
	tokenFamilyPrefix       = "token_family:"
	// refreshReplayKeyPrefix holds the token pair a rotation minted, for the
	// length of the reuse grace. Kept in its OWN key rather than as a field
	// of the refresh_token: record because it contains RAW tokens: a
	// separate key gets its own short TTL (service.RefreshReplayRetention),
	// so Redis never holds a usable refresh token for more than about two
	// minutes, instead of for the tombstone's remaining 30 days.
	refreshReplayKeyPrefix = "refresh_replay:"
)

// refreshTokenKey generates the Redis key for a refresh token.
func refreshTokenKey(tokenHash string) string {
	return refreshTokenKeyPrefix + tokenHash
}

// userRefreshTokensKey generates the Redis key for user's token set.
func userRefreshTokensKey(userID int64) string {
	return fmt.Sprintf("%s%d", userRefreshTokensPrefix, userID)
}

// tokenFamilyKey generates the Redis key for token family set.
func tokenFamilyKey(familyID string) string {
	return tokenFamilyPrefix + familyID
}

// refreshReplayKey generates the Redis key holding the grace-window replay
// pair for a rotated refresh token.
func refreshReplayKey(tokenHash string) string {
	return refreshReplayKeyPrefix + tokenHash
}

// refreshTokenMarkRotatedScript atomically flips a refresh-token record's
// "rotated" flag exactly once, and classifies every caller that does not win
// as either a forgivable replay inside the reuse grace or genuine reuse.
//
// This exists because two concurrent presentations of the SAME refresh token
// (an attacker replaying a stolen token at the same instant the legitimate
// client refreshes on schedule) must not both be able to mint a token pair
// from it — a Go-side GetRefreshToken() followed by a separate
// StoreRefreshToken() is two Redis round trips with a window in between
// where both callers can observe Rotated=false. This script's
// GET-decide-SET happen inside one Redis command, so only one of two
// simultaneous callers can ever be told it won.
//
// The grace decision lives HERE, in the same EVAL, for the same reason: the
// alternative — returning rotated_at_unix and comparing it in Go — is a
// read-then-decide split, and splitting the read from the decision is
// precisely the shape of race this script was written to close. The reply
// therefore carries the verdict, not the raw material for one, and the
// caller needs no second round trip to act on it.
//
// KEEPTTL (Redis >= 6.0) preserves the record's exact remaining lifetime —
// no separate PTTL-read-then-SET-PX window to race on. The replay key gets
// its own short EX instead: it holds raw tokens, so it must NOT inherit the
// tombstone's 30-day life.
//
// KEYS[1] = refresh_token:{hash}
// KEYS[2] = refresh_replay:{hash}
// ARGV[1] = the tombstoned JSON value to store if this call wins
// ARGV[2] = the JSON replay pair to store if this call wins
// ARGV[3] = the caller's clock, unix seconds
// ARGV[4] = the grace window, seconds
// ARGV[5] = the replay key's TTL, seconds
//
// Returns a {flag, raw, replay} table, where raw and replay are the empty
// string when they do not apply:
//   - flag -1: the key does not exist
//   - flag 0:  this call won (raw = the value as it stood immediately before
//     the SET below — i.e. pre-rotation)
//   - flag 1:  already rotated, OUTSIDE the grace: reuse
//   - flag 2:  already rotated, INSIDE the grace, and replay = the pair that
//     rotation minted, still stored
//
// Note the grace is never extended: nothing on the already-rotated branch
// writes, so neither the tombstone's rotated_at_unix nor the replay key's
// TTL moves when a replay arrives.
var refreshTokenMarkRotatedScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {-1, '', ''}
end
local decoded = cjson.decode(raw)
if decoded.rotated then
  local rotatedAt = decoded.rotated_at_unix
  if type(rotatedAt) ~= 'number' then
    rotatedAt = 0
  end
  local now = tonumber(ARGV[3])
  local grace = tonumber(ARGV[4])
  if rotatedAt > 0 and now >= rotatedAt and (now - rotatedAt) <= grace then
    local replay = redis.call('GET', KEYS[2])
    if replay then
      return {2, raw, replay}
    end
  end
  return {1, raw, ''}
end
redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
redis.call('SET', KEYS[2], ARGV[2], 'EX', ARGV[5])
return {0, raw, ''}
`)

type refreshTokenCache struct {
	rdb *redis.Client
}

// NewRefreshTokenCache creates a new RefreshTokenCache implementation.
func NewRefreshTokenCache(rdb *redis.Client) service.RefreshTokenCache {
	return &refreshTokenCache{rdb: rdb}
}

func (c *refreshTokenCache) StoreRefreshToken(ctx context.Context, tokenHash string, data *service.RefreshTokenData, ttl time.Duration) error {
	key := refreshTokenKey(tokenHash)
	val, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal refresh token data: %w", err)
	}
	return c.rdb.Set(ctx, key, val, ttl).Err()
}

func (c *refreshTokenCache) GetRefreshToken(ctx context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	key := refreshTokenKey(tokenHash)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrRefreshTokenNotFound
		}
		return nil, err
	}
	var data service.RefreshTokenData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("unmarshal refresh token data: %w", err)
	}
	return &data, nil
}

func (c *refreshTokenCache) MarkRotated(ctx context.Context, tokenHash string, tombstoned *service.RefreshTokenData, replay *service.RefreshReplayPair, now time.Time) (*service.RefreshRotationResult, error) {
	if replay == nil {
		// Not defensive padding: without a stored pair every concurrent
		// presentation inside the grace would be classified as reuse and
		// would revoke a healthy family, which is exactly the failure this
		// whole mechanism exists to remove.
		return nil, fmt.Errorf("mark refresh token rotated: replay pair is required")
	}
	tombVal, err := json.Marshal(tombstoned)
	if err != nil {
		return nil, fmt.Errorf("marshal tombstoned refresh token data: %w", err)
	}
	replayVal, err := json.Marshal(replay)
	if err != nil {
		return nil, fmt.Errorf("marshal refresh token replay pair: %w", err)
	}

	keys := []string{refreshTokenKey(tokenHash), refreshReplayKey(tokenHash)}
	res, err := refreshTokenMarkRotatedScript.Run(ctx, c.rdb, keys,
		tombVal,
		replayVal,
		now.Unix(),
		int64(service.RefreshReuseGracePeriod/time.Second),
		int64(service.RefreshReplayRetention/time.Second),
	).Result()
	if err != nil {
		return nil, fmt.Errorf("mark refresh token rotated: %w", err)
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 3 {
		return nil, fmt.Errorf("mark refresh token rotated: unexpected script result %#v", res)
	}
	flag, ok := arr[0].(int64)
	if !ok {
		return nil, fmt.Errorf("mark refresh token rotated: unexpected flag type %#v", arr[0])
	}
	if flag == -1 {
		return nil, service.ErrRefreshTokenNotFound
	}
	rawStr, ok := arr[1].(string)
	if !ok {
		return nil, fmt.Errorf("mark refresh token rotated: unexpected payload type %#v", arr[1])
	}

	var data service.RefreshTokenData
	if err := json.Unmarshal([]byte(rawStr), &data); err != nil {
		return nil, fmt.Errorf("unmarshal refresh token data: %w", err)
	}
	out := &service.RefreshRotationResult{Data: &data}

	switch flag {
	case 0:
		out.Outcome = service.RefreshRotationClaimed
	case 1:
		out.Outcome = service.RefreshRotationReuse
	case 2:
		replayStr, ok := arr[2].(string)
		if !ok {
			return nil, fmt.Errorf("mark refresh token rotated: unexpected replay type %#v", arr[2])
		}
		var pair service.RefreshReplayPair
		if err := json.Unmarshal([]byte(replayStr), &pair); err != nil {
			return nil, fmt.Errorf("unmarshal refresh token replay pair: %w", err)
		}
		out.Outcome = service.RefreshRotationGraceReplay
		out.Replay = &pair
	default:
		return nil, fmt.Errorf("mark refresh token rotated: unexpected script flag %d", flag)
	}
	return out, nil
}

// DeleteRefreshToken removes a record and, with it, any grace-window replay
// pair still parked under that hash. Dropping the replay key alongside the
// record is what keeps an explicit revocation from leaving a RAW refresh
// token readable in Redis for the rest of the retention window.
func (c *refreshTokenCache) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	return c.rdb.Del(ctx, refreshTokenKey(tokenHash), refreshReplayKey(tokenHash)).Err()
}

func (c *refreshTokenCache) DeleteUserRefreshTokens(ctx context.Context, userID int64) error {
	// Get all token hashes for this user
	tokenHashes, err := c.GetUserTokenHashes(ctx, userID)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get user token hashes: %w", err)
	}

	if len(tokenHashes) == 0 {
		return nil
	}

	// Build keys to delete
	// Two keys per hash: the record and any grace-window replay pair, so a
	// revocation never leaves a RAW refresh token behind (see
	// DeleteRefreshToken).
	keys := make([]string, 0, 2*len(tokenHashes)+1)
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash), refreshReplayKey(hash))
	}
	keys = append(keys, userRefreshTokensKey(userID))

	// Delete all keys in a pipeline
	pipe := c.rdb.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) DeleteTokenFamily(ctx context.Context, familyID string) error {
	// Get all token hashes in this family
	tokenHashes, err := c.GetFamilyTokenHashes(ctx, familyID)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get family token hashes: %w", err)
	}

	if len(tokenHashes) == 0 {
		return nil
	}

	// Build keys to delete
	// Two keys per hash: the record and any grace-window replay pair, so a
	// reuse revocation never leaves a RAW refresh token behind (see
	// DeleteRefreshToken).
	keys := make([]string, 0, 2*len(tokenHashes)+1)
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash), refreshReplayKey(hash))
	}
	keys = append(keys, tokenFamilyKey(familyID))

	// Delete all keys in a pipeline
	pipe := c.rdb.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) AddToUserTokenSet(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error {
	key := userRefreshTokensKey(userID)
	pipe := c.rdb.Pipeline()
	pipe.SAdd(ctx, key, tokenHash)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) AddToFamilyTokenSet(ctx context.Context, familyID string, tokenHash string, ttl time.Duration) error {
	key := tokenFamilyKey(familyID)
	pipe := c.rdb.Pipeline()
	pipe.SAdd(ctx, key, tokenHash)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) GetUserTokenHashes(ctx context.Context, userID int64) ([]string, error) {
	key := userRefreshTokensKey(userID)
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *refreshTokenCache) GetFamilyTokenHashes(ctx context.Context, familyID string) ([]string, error) {
	key := tokenFamilyKey(familyID)
	return c.rdb.SMembers(ctx, key).Result()
}

func (c *refreshTokenCache) IsTokenInFamily(ctx context.Context, familyID string, tokenHash string) (bool, error) {
	key := tokenFamilyKey(familyID)
	return c.rdb.SIsMember(ctx, key, tokenHash).Result()
}
