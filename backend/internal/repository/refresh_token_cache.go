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

// refreshTokenMarkRotatedScript atomically flips a refresh-token record's
// "rotated" flag exactly once. This exists because two concurrent
// presentations of the SAME refresh token (an attacker replaying a stolen
// token at the same instant the legitimate client refreshes on schedule)
// must not both be able to mint a token pair from it — a Go-side
// GetRefreshToken() followed by a separate StoreRefreshToken() is two Redis
// round trips with a window in between where both callers can observe
// Rotated=false. This script's GET-decide-SET happen inside one Redis
// command, so only one of two simultaneous callers can ever be told it won.
//
// KEEPTTL (Redis >= 6.0) preserves the record's exact remaining lifetime —
// no separate PTTL-read-then-SET-PX window to race on.
//
// KEYS[1] = refresh_token:{hash}
// ARGV[1] = the tombstoned JSON value to store if this call wins
//
// Returns:
//   - {-1, ”}  if the key does not exist
//   - {1, raw}  if it was already rotated (raw = the value as found, unchanged)
//   - {0, raw}  if this call won (raw = the value as it stood immediately
//     before the SET below — i.e. pre-rotation)
var refreshTokenMarkRotatedScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {-1, ''}
end
local decoded = cjson.decode(raw)
if decoded.rotated then
  return {1, raw}
end
redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
return {0, raw}
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

func (c *refreshTokenCache) MarkRotated(ctx context.Context, tokenHash string, tombstoned *service.RefreshTokenData) (*service.RefreshTokenData, bool, error) {
	key := refreshTokenKey(tokenHash)
	tombVal, err := json.Marshal(tombstoned)
	if err != nil {
		return nil, false, fmt.Errorf("marshal tombstoned refresh token data: %w", err)
	}

	res, err := refreshTokenMarkRotatedScript.Run(ctx, c.rdb, []string{key}, tombVal).Result()
	if err != nil {
		return nil, false, fmt.Errorf("mark refresh token rotated: %w", err)
	}

	arr, ok := res.([]any)
	if !ok || len(arr) != 2 {
		return nil, false, fmt.Errorf("mark refresh token rotated: unexpected script result %#v", res)
	}
	flag, ok := arr[0].(int64)
	if !ok {
		return nil, false, fmt.Errorf("mark refresh token rotated: unexpected flag type %#v", arr[0])
	}
	if flag == -1 {
		return nil, false, service.ErrRefreshTokenNotFound
	}
	rawStr, ok := arr[1].(string)
	if !ok {
		return nil, false, fmt.Errorf("mark refresh token rotated: unexpected payload type %#v", arr[1])
	}

	var data service.RefreshTokenData
	if err := json.Unmarshal([]byte(rawStr), &data); err != nil {
		return nil, false, fmt.Errorf("unmarshal refresh token data: %w", err)
	}
	return &data, flag == 1, nil
}

func (c *refreshTokenCache) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	key := refreshTokenKey(tokenHash)
	return c.rdb.Del(ctx, key).Err()
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
	keys := make([]string, 0, len(tokenHashes)+1)
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash))
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
	keys := make([]string, 0, len(tokenHashes)+1)
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash))
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
