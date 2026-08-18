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
	// refreshReplayKeyPrefix holds the SEALED token pair a rotation minted,
	// for the length of the reuse grace. The value is opaque ciphertext —
	// the service encrypts it under a key derived from the refresh token
	// being rotated, so this cache never holds a usable token, only a blob
	// that the next presenter of that token can open. Kept in its own key
	// rather than as a field of the refresh_token: record so it can carry
	// its own short TTL (service.RefreshReplayRetention) instead of the
	// tombstone's remaining 30 days.
	refreshReplayKeyPrefix = "refresh_replay:"
	// familyRevokedKeyPrefix marks a token family as revoked. Written by
	// DeleteTokenFamily BEFORE it enumerates the family, and checked by
	// refreshTokenPersistScript inside the same atomic operation that
	// writes — which is what stops a rotation already in flight from
	// re-creating a family that has just been killed. See
	// service.RefreshFamilyRevokedRetention.
	familyRevokedKeyPrefix = "family_revoked:"
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

// refreshReplayKey generates the Redis key holding the sealed grace-window
// replay pair for a rotated refresh token.
func refreshReplayKey(tokenHash string) string {
	return refreshReplayKeyPrefix + tokenHash
}

// familyRevokedKey generates the Redis key that marks a token family as
// permanently revoked.
func familyRevokedKey(familyID string) string {
	return familyRevokedKeyPrefix + familyID
}

// refreshTokenPersistScript writes a refresh-token record together with both
// of its revocation-index memberships, or writes nothing at all.
//
// The three writes used to be three separate round trips in
// OAuthTokenService.persistRefreshToken, which had two failure modes that
// are the same failure seen from two sides — a partial failure orphaning a
// live 30-day credential outside every revocation index, and a concurrent
// DeleteTokenFamily missing a token this call is about to add. The full
// reasoning is on service.RefreshTokenCache.PersistRefreshToken and
// service.RefreshFamilyRevokedRetention; the short version is that both are
// closed only by doing the check and all three writes in ONE operation.
//
// KEYS[1] = refresh_token:{hash}
// KEYS[2] = user_refresh_tokens:{user_id}
// KEYS[3] = token_family:{family_id}
// KEYS[4] = family_revoked:{family_id}
// ARGV[1] = the record JSON
// ARGV[2] = the token hash (the set member)
// ARGV[3] = the record/set TTL, seconds
//
// Returns 1 on success and 0 if the family is revoked (nothing written).
//
// The EXPIRE on each set re-arms the whole set's lifetime on every add,
// exactly as the AddToUserTokenSet / AddToFamilyTokenSet pipelines it
// replaces did — deliberately unchanged, so this is atomicity only and not
// a silent change of retention.
var refreshTokenPersistScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[4]) == 1 then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[3])
redis.call('SADD', KEYS[2], ARGV[2])
redis.call('EXPIRE', KEYS[2], ARGV[3])
redis.call('SADD', KEYS[3], ARGV[2])
redis.call('EXPIRE', KEYS[3], ARGV[3])
return 1
`)

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
// its own short EX instead: nothing will ever open it again once the grace
// has passed, so it should not inherit the tombstone's 30-day life.
//
// KEYS[1] = refresh_token:{hash}          the record / tombstone
// KEYS[2] = refresh_replay:{hash}         this token's replay slot
// KEYS[3] = refresh_replay:{predecessor}  the slot this rotation supersedes
// ARGV[1] = the tombstoned JSON value to store if this call wins
// ARGV[2] = the sealed replay pair to store if this call wins — opaque
// ciphertext, never decoded here, binary-safe
// ARGV[3] = the caller's clock, unix seconds
// ARGV[4] = the grace window, seconds
// ARGV[5] = the replay slot's TTL, seconds
// ARGV[6] = how far a replay may appear to precede its rotation, seconds
// ARGV[7] = the supersession marker
// ARGV[8] = "1" if the caller supplied a replay pair, "0" if not
// ARGV[9] = "1" if KEYS[3] names a real predecessor, "0" if this is a root
//
// Returns a {flag, raw, replay} table, where raw and replay are the empty
// string when they do not apply:
//   - flag -2: the caller brought no pair and the record is NOT rotated, so
//     claiming would tombstone the token with nothing to hand back. Refused.
//   - flag -1: the key does not exist
//   - flag 0:  this call won (raw = the value as it stood immediately before
//     the SET below — i.e. pre-rotation)
//   - flag 1:  reuse — either the window has passed, or the chain has moved
//     on past this token and its slot holds the marker
//   - flag 2:  inside the grace, and replay = the sealed pair that rotation
//     minted, still stored
//   - flag 3:  inside the grace, chain has NOT moved on, but the pair is
//     gone (eviction — not natural expiry, since the slot's TTL is strictly
//     longer than the window). Not reuse; the caller must 500.
//
// WHY THE SUPERSESSION WRITE IS HERE and not a separate call: "the
// immediately previous token" is a property of the whole chain, and the
// instant that stops being true of N0 is the instant N1 is claimed. Closing
// N0's window in the same EVAL that claims N1 means there is no interval in
// which both N0 and N1 are forgivable — which is what stops a thief holding
// a stale N0 from being handed N1 and walking it forward to the live token
// whenever a family rotates twice inside a minute.
//
// Note the already-rotated branch still writes NOTHING, so a replay can
// neither restart nor extend its own window.
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
  local skew = tonumber(ARGV[6])
  if rotatedAt > 0 and (now - rotatedAt) <= grace and (rotatedAt - now) <= skew then
    local replay = redis.call('GET', KEYS[2])
    if replay == ARGV[7] then
      return {1, raw, ''}
    end
    if replay then
      return {2, raw, replay}
    end
    return {3, raw, ''}
  end
  return {1, raw, ''}
end
if ARGV[8] ~= '1' then
  return {-2, raw, ''}
end
redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')
redis.call('SET', KEYS[2], ARGV[2], 'EX', ARGV[5])
if ARGV[9] == '1' then
  redis.call('SET', KEYS[3], ARGV[7], 'EX', ARGV[5])
end
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

func (c *refreshTokenCache) PersistRefreshToken(ctx context.Context, tokenHash string, data *service.RefreshTokenData, ttl time.Duration) error {
	val, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal refresh token data: %w", err)
	}
	keys := []string{
		refreshTokenKey(tokenHash),
		userRefreshTokensKey(data.UserID),
		tokenFamilyKey(data.FamilyID),
		familyRevokedKey(data.FamilyID),
	}
	res, err := refreshTokenPersistScript.Run(ctx, c.rdb, keys, val, tokenHash, int64(ttl/time.Second)).Result()
	if err != nil {
		return fmt.Errorf("persist refresh token: %w", err)
	}
	written, ok := res.(int64)
	if !ok {
		return fmt.Errorf("persist refresh token: unexpected script result %#v", res)
	}
	if written == 0 {
		return service.ErrTokenFamilyRevoked
	}
	return nil
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

func (c *refreshTokenCache) MarkRotated(ctx context.Context, tokenHash string, tombstoned *service.RefreshTokenData, sealedReplay []byte, now time.Time) (*service.RefreshRotationResult, error) {
	// An empty sealedReplay is allowed only because the caller may already
	// know this record is rotated (a replay never needs a pair, and minting
	// one for it would put the signing key on the theft-detection path). The
	// script refuses to CLAIM without one, so the dangerous combination —
	// tombstoning a live token with nothing to hand back — cannot happen
	// here or in any future caller.
	hasPair := "0"
	if len(sealedReplay) > 0 {
		hasPair = "1"
	}
	hasPredecessor := "0"
	if tombstoned.PredecessorHash != "" {
		hasPredecessor = "1"
	}
	tombVal, err := json.Marshal(tombstoned)
	if err != nil {
		return nil, fmt.Errorf("marshal tombstoned refresh token data: %w", err)
	}

	// KEYS[3] is the predecessor's replay slot. When there is no predecessor
	// it degenerates to the bare prefix, a key nothing ever writes, and the
	// script is told not to touch it (ARGV[9]) — declared rather than
	// omitted so every key the script can reach is named up front.
	keys := []string{
		refreshTokenKey(tokenHash),
		refreshReplayKey(tokenHash),
		refreshReplayKey(tombstoned.PredecessorHash),
	}
	res, err := refreshTokenMarkRotatedScript.Run(ctx, c.rdb, keys,
		tombVal,
		sealedReplay,
		now.Unix(),
		int64(service.RefreshReuseGracePeriod/time.Second),
		int64(service.RefreshReplayRetention/time.Second),
		int64(service.RefreshGraceClockSkewAllowance/time.Second),
		service.RefreshReplaySupersededMarker,
		hasPair,
		hasPredecessor,
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
	if flag == -2 {
		return nil, fmt.Errorf("mark refresh token rotated: refused to claim an un-rotated record without a replay pair")
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
		// Handed back exactly as stored. Redis strings are binary-safe and
		// this layer has no key with which to interpret the blob anyway.
		out.Outcome = service.RefreshRotationGraceReplay
		out.SealedReplay = []byte(replayStr)
	case 3:
		out.Outcome = service.RefreshRotationGraceUnavailable
	default:
		return nil, fmt.Errorf("mark refresh token rotated: unexpected script flag %d", flag)
	}
	return out, nil
}

// ownersOf groups token hashes by the user id recorded on each one, for the
// user-set SREM the delete paths below perform.
//
// Best effort by construction: a hash whose record has already expired or
// been deleted names no user, so its membership cannot be removed and is
// left to age out with the set's own TTL. That is the pre-existing
// behaviour for every hash, and this narrows it rather than replacing it.
// Read BEFORE the records are deleted, since the record is the only place
// the user id is written.
func (c *refreshTokenCache) ownersOf(ctx context.Context, tokenHashes []string) map[int64][]string {
	if len(tokenHashes) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tokenHashes))
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash))
	}
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil || len(vals) != len(tokenHashes) {
		return nil
	}
	owners := make(map[int64][]string)
	for i, v := range vals {
		raw, ok := v.(string)
		if !ok {
			continue
		}
		var data service.RefreshTokenData
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			continue
		}
		owners[data.UserID] = append(owners[data.UserID], tokenHashes[i])
	}
	return owners
}

// DeleteRefreshToken removes a record and, with it, any grace-window replay
// pair still parked under that hash, and its membership of the owning
// user's token set.
//
// The pair is sealed, so deleting it is no longer about secrecy — it is
// about not leaving a blob behind that outlives the session it belongs to.
// The set membership is the same argument: without the SREM the set
// accumulated hashes naming records that no longer exist for the rest of
// its TTL, which made "how many live sessions does this user have" an
// unreliable answer and kept memory alive past what it named.
func (c *refreshTokenCache) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	owners := c.ownersOf(ctx, []string{tokenHash})
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, refreshTokenKey(tokenHash), refreshReplayKey(tokenHash))
	for userID, hashes := range owners {
		pipe.SRem(ctx, userRefreshTokensKey(userID), stringsToAny(hashes)...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// stringsToAny adapts a hash slice to go-redis's variadic `...any` member
// arguments.
func stringsToAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
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
	// revocation leaves nothing behind (see DeleteRefreshToken).
	keys := make([]string, 0, 2*len(tokenHashes))
	for _, hash := range tokenHashes {
		keys = append(keys, refreshTokenKey(hash), refreshReplayKey(hash))
	}

	// SREM of exactly the hashes enumerated above, NOT DEL of the set key.
	//
	// A DEL would also discard a membership added between the SMEMBERS and
	// here — a login (or a rotation) racing this revocation — and that
	// token's record is NOT in the delete list, so it would survive as a
	// live credential that no later DeleteUserRefreshTokens could name.
	// SREM-ing only what was enumerated leaves such a token both live and
	// indexed, which is the correct reading of "it arrived after the
	// revocation ran": still revocable next time. Redis drops a set that
	// becomes empty, so the ordinary case still leaves no key behind.
	//
	// The family-scoped analogue of this race needs a stronger primitive —
	// see service.RefreshFamilyRevokedRetention. A user-scoped revocation
	// marker is deliberately NOT the answer here: a user id is reused by
	// every future login, so a marker would refuse legitimate logins for
	// its whole retention, where a family id is single-use and can safely
	// be remembered forever.
	pipe := c.rdb.Pipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	pipe.SRem(ctx, userRefreshTokensKey(userID), stringsToAny(tokenHashes)...)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *refreshTokenCache) DeleteTokenFamily(ctx context.Context, familyID string) error {
	// Mark the family revoked BEFORE enumerating it, and fail the whole
	// revocation if the mark cannot be written.
	//
	// Ordering is the entire point: PersistRefreshToken checks this key
	// inside the same atomic operation that writes, so a rotation whose
	// persist completes before the mark is visible is necessarily visible
	// to the SMEMBERS below, and one that starts after it is refused. Any
	// other order — or a mark written best-effort and ignored on failure —
	// leaves the window where a rotation in flight re-creates the family it
	// was just revoked from, holding only the surviving head. See
	// service.RefreshFamilyRevokedRetention.
	//
	// Returning the error rather than logging it is deliberate: every
	// caller of this method is revoking a credential (reuse detected, code
	// replayed, user banned, password changed), and a revocation that
	// cannot guarantee its own ordering must be reported as failed, not
	// silently downgraded.
	if err := c.rdb.Set(ctx, familyRevokedKey(familyID), "1", service.RefreshFamilyRevokedRetention).Err(); err != nil {
		return fmt.Errorf("mark token family revoked: %w", err)
	}

	// Get all token hashes in this family
	tokenHashes, err := c.GetFamilyTokenHashes(ctx, familyID)
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get family token hashes: %w", err)
	}

	if len(tokenHashes) == 0 {
		return nil
	}

	// Read the owning user BEFORE the records are deleted — the record is
	// the only place the user id is written, and the memberships have to be
	// removed from the user's set too (see DeleteRefreshToken).
	owners := c.ownersOf(ctx, tokenHashes)

	// Build keys to delete
	// Two keys per hash: the record and any grace-window replay pair, so a
	// reuse revocation leaves nothing behind (see DeleteRefreshToken).
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
	for userID, hashes := range owners {
		pipe.SRem(ctx, userRefreshTokensKey(userID), stringsToAny(hashes)...)
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
