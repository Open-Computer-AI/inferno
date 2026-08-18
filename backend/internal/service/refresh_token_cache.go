package service

import (
	"context"
	"errors"
	"time"
)

// ErrRefreshTokenNotFound is returned when a refresh token is not found in cache.
// This is used to abstract away the underlying cache implementation (e.g., redis.Nil).
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

// ErrTokenFamilyRevoked is returned by PersistRefreshToken when the family
// the token would join has already been revoked (DeleteTokenFamily).
//
// It is a DELIBERATE refusal, not an infrastructure fault, and callers must
// classify it as such: the family was killed on purpose — by reuse
// detection, by an authorization-code replay, by a ban, or by a credential
// change — and a token that lands a millisecond later must not be allowed
// to resurrect it. See RefreshFamilyRevokedRetention for why this cannot be
// answered by simply re-checking set membership after the write.
var ErrTokenFamilyRevoked = errors.New("refresh token family revoked")

// RefreshTokenData 存储在Redis中的Refresh Token数据
type RefreshTokenData struct {
	UserID       int64     `json:"user_id"`
	TokenVersion int64     `json:"token_version"`          // 用于检测密码更改后的Token失效
	FamilyID     string    `json:"family_id"`              // Token家族ID，用于防重放攻击
	BindingHash  string    `json:"binding_hash,omitempty"` // 会话指纹哈希（IP+UA），会话绑定开启时校验
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`

	// The fields below are OAuth-token-service additions (device_code /
	// refresh_token grants, backend/internal/service/oauth_token_service.go).
	// Panel-session refresh tokens never set them; `omitempty` keeps existing
	// Redis-stored session records backward compatible — they deserialize
	// with these at their zero value and behave exactly as before.

	// Scope is the OAuth scope this refresh token is bound to. A refresh
	// must return exactly this scope, never a wider or narrower one.
	Scope string `json:"scope,omitempty"`
	// ClientID is the OAuth client_id this refresh token was issued to.
	// ExchangeRefreshToken rejects a refresh_token grant presented by any
	// other client_id.
	ClientID string `json:"client_id,omitempty"`
	// Rotated marks this record as a tombstone: the raw token that hashes to
	// this entry has already been redeemed once. The record is deliberately
	// kept (re-stored, not deleted) for the rest of its original TTL so that
	// a replay of that same raw token can still be traced to FamilyID and
	// the whole family revoked — see OAuthTokenService.ExchangeRefreshToken.
	Rotated bool `json:"rotated,omitempty"`
	// RotatedAtUnix is the wall-clock second at which Rotated was set. It is
	// the anchor of the reuse grace window (RefreshReuseGracePeriod): a
	// replay is inside the grace iff now - RotatedAtUnix <= the grace.
	//
	// Unix seconds, not a time.Time, on purpose: the comparison is made
	// inside the Redis Lua script (see repository.refreshTokenMarkRotatedScript),
	// and Lua's cjson can compare a JSON number but cannot parse RFC 3339.
	// Zero on a record that was never rotated, and on any tombstone written
	// before this field existed — both are treated as "outside the grace",
	// which is the fail-secure direction.
	RotatedAtUnix int64 `json:"rotated_at_unix,omitempty"`
	// PredecessorHash is the hash of the refresh token this one replaced, or
	// empty for a token issued by a device_code / authorization_code grant
	// (the root of a family).
	//
	// It exists so a rotation can close its predecessor's grace window. The
	// grace forgives the IMMEDIATELY previous token, and "immediately
	// previous" stops being true the moment the chain moves on: rotating
	// N1 -> N2 makes N0 two generations stale, and N0 must from then on be
	// treated as reuse even if its own 60 seconds have not elapsed.
	// Without this, a thief holding N0 could be handed N1 and walk it
	// forward to the live N2 in silence whenever a family rotated twice
	// inside a minute — which is exactly the chatty-client population the
	// grace was written for.
	PredecessorHash string `json:"predecessor_hash,omitempty"`
}

const (
	// RefreshReuseGracePeriod is how long after a refresh token is rotated a
	// replay of that same token is forgiven instead of treated as reuse.
	//
	// This exists because instant revocation kills healthy sessions: a
	// desktop agent that retries a refresh whose response was lost in
	// flight, or two windows of the same session refreshing at once, both
	// present an already-rotated token through no fault of anyone's. Portal
	// documents the same 60s window (plugins/dashboard_auth/nous/__init__.py),
	// and the hermes client is written against that contract.
	//
	// 60 seconds is defined HERE and only here — the Lua script's window and
	// the replay key's TTL are both derived from this constant rather than
	// restating the literal.
	//
	// The grace is deliberately narrow: it forgives ONE token (the one whose
	// own rotation was less than a minute ago), it returns the pair that
	// rotation already minted rather than minting another, and it is never
	// extended or restarted by a replay. Anything broader would blunt reuse
	// detection, which is the only reason rotation exists.
	RefreshReuseGracePeriod = 60 * time.Second

	// RefreshReplayRetention is how long the sealed replay pair is kept.
	//
	// Deliberately a little LONGER than the grace so that the script's
	// explicit RotatedAtUnix comparison, not key expiry, is what decides the
	// window; the TTL is cleanup, not policy. It is no longer load-bearing
	// for secrecy either — the stored pair is encrypted under a key only the
	// presenter of the predecessor token can derive (see sealRefreshReplay)
	// — but a short life on a blob nobody will ever open again is still the
	// right hygiene.
	RefreshReplayRetention = 2 * RefreshReuseGracePeriod

	// RefreshGraceClockSkewAllowance is how far a replay may appear to
	// arrive BEFORE the rotation it is replaying.
	//
	// rotated_at_unix is stamped by whichever instance won the rotation and
	// `now` is supplied by whichever instance handles the replay. On a
	// multi-replica deployment with ordinary NTP skew those are different
	// clocks, so a perfectly benign retry can look like it happened a second
	// before the rotation. Without an allowance that lands on the reuse arm
	// and kills a healthy session — the precise failure this grace exists to
	// remove, reintroduced by a different route.
	//
	// It does not widen the window in the direction that matters: the
	// forward edge is still exactly RefreshReuseGracePeriod.
	RefreshGraceClockSkewAllowance = 5 * time.Second

	// RefreshFamilyRevokedRetention is how long a revoked family is
	// remembered as revoked, so that a rotation still in flight when the
	// revocation ran cannot re-create the family behind it.
	//
	// This exists because DeleteTokenFamily is SMEMBERS -> DEL and a
	// concurrent persist is a separate write. Without a marker, the
	// canonical theft interleaving — thief replays while the real client
	// keeps refreshing — reads:
	//
	//	revoker:  SMEMBERS token_family:F -> {N0, N1}
	//	rotator:  writes N2 and SADDs it to token_family:F
	//	revoker:  DEL refresh_token:N0, refresh_token:N1, token_family:F
	//
	// and leaves N2 — the live head of a family that was just revoked for
	// reuse — alive for the rest of its 30 days, with the family set
	// re-created holding only N2 so no later revocation is even wrong.
	// Re-checking membership after the persist does NOT close it: the DEL
	// can land BEFORE the SADD, in which case the re-check sees the token
	// it just added and concludes all is well.
	//
	// The marker is written BEFORE the SMEMBERS, and PersistRefreshToken
	// checks it inside the same atomic operation that writes. That leaves
	// no interleaving: a persist that completes before the marker is
	// visible is necessarily visible to the SMEMBERS that follows it, and a
	// persist that starts after it is refused.
	//
	// Retention matches the longest refresh-token lifetime in the system
	// (OAuth's 30 days). A family id is 16 bytes of crypto/rand and is
	// never reused, so remembering it costs one small key per revocation
	// and can never refuse a legitimate future login — unlike a user-scoped
	// marker, which would (see DeleteUserRefreshTokens).
	RefreshFamilyRevokedRetention = 30 * 24 * time.Hour
)

// RefreshReplaySupersededMarker is written over a token's replay pair when
// its successor is itself rotated, replacing the ciphertext with a tombstone
// that says "this token is no longer the immediately previous one".
//
// A marker rather than a plain delete because absence and supersession must
// mean different things: an absent pair inside the window means the blob was
// evicted and the request cannot be answered (an infrastructure fault, 500),
// while a marker means the chain moved on and the presentation is reuse
// (revoke the family). Deleting would collapse the two and make eviction
// look like theft.
//
// Never confusable with a sealed pair: those are AES-GCM outputs of at least
// 12 (nonce) + 16 (tag) bytes with a random prefix, compared here for exact
// equality.
const RefreshReplaySupersededMarker = "superseded:v1"

// RefreshReplayPair is the token pair minted by a rotation, stashed so that a
// replay of the rotated token inside RefreshReuseGracePeriod can be answered
// with the SAME pair rather than either minting a second one (which would
// fork the family into two live branches — the race MarkRotated exists to
// close) or revoking the family (which is what the grace is softening).
//
// It NEVER leaves the service package in this form. RefreshTokenCache stores
// it sealed (AES-256-GCM under a key derived from the predecessor refresh
// token, see sealRefreshReplay/openRefreshReplay in oauth_token_service.go),
// so the cache — and anyone reading the cache — holds ciphertext, not
// tokens. The only party that can open it is the one presenting the
// predecessor, which is exactly the party entitled to the pair.
type RefreshReplayPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	// AccessExpiresAtUnix lets a replay report the access token's REMAINING
	// lifetime. Re-reporting the full TTL would overstate it by however long
	// into the grace the replay arrived, and a client that trusted that would
	// hold a dead access token for that long.
	AccessExpiresAtUnix int64 `json:"access_expires_at_unix"`
}

// RefreshRotationOutcome is what MarkRotated's single atomic claim decided.
type RefreshRotationOutcome int

const (
	// RefreshRotationUnknown is the zero value and is never returned by a
	// correct implementation. It exists so that a result struct that was
	// never populated cannot be mistaken for RefreshRotationClaimed —
	// callers must treat it as an internal error, not as permission to mint.
	RefreshRotationUnknown RefreshRotationOutcome = iota
	// RefreshRotationClaimed: this call won the race. The record was not
	// rotated; the tombstone and the sealed replay pair have been stored.
	RefreshRotationClaimed
	// RefreshRotationGraceReplay: the record was already rotated, but less
	// than RefreshReuseGracePeriod ago and the sealed pair that rotation
	// minted is still stored. Nothing was written. The caller must unseal it
	// and return that pair verbatim.
	RefreshRotationGraceReplay
	// RefreshRotationReuse: the record was already rotated, and either the
	// window has passed or the chain has moved on past this token (its
	// replay slot holds RefreshReplaySupersededMarker). Nothing was
	// written. This is the reuse signal: the caller must revoke the family.
	RefreshRotationReuse
	// RefreshRotationGraceUnavailable: the record was already rotated,
	// inside the window, and the chain has NOT moved on — but the sealed
	// pair is gone. Since RefreshReplayRetention is strictly longer than
	// RefreshReuseGracePeriod this cannot be natural expiry; it means
	// eviction (Redis maxmemory with an allkeys-* policy) or corruption.
	//
	// It is deliberately NOT reuse. Everything known about this
	// presentation says benign — a token rotated seconds ago, still the
	// immediately previous one — so revoking the family on the strength of
	// a missing cache entry would kill a healthy session over an
	// infrastructure event. The caller must surface it as a 500.
	RefreshRotationGraceUnavailable
)

// RefreshRotationResult is MarkRotated's single return value.
type RefreshRotationResult struct {
	// Data is the record as it stood immediately BEFORE this call, whatever
	// the outcome — still carrying FamilyID, which is what a reuse
	// revocation needs.
	Data *RefreshTokenData
	// Outcome is the decision the atomic claim reached.
	Outcome RefreshRotationOutcome
	// SealedReplay is non-empty iff Outcome is RefreshRotationGraceReplay.
	// It is opaque to the cache: only a caller holding the predecessor
	// refresh token can open it.
	SealedReplay []byte
}

// RefreshTokenCache 管理Refresh Token的Redis缓存
// 用于JWT Token刷新机制，支持Token轮转和防重放攻击
//
// Key 格式:
//   - refresh_token:{token_hash}     -> RefreshTokenData (JSON)
//   - user_refresh_tokens:{user_id}  -> Set<token_hash>
//   - token_family:{family_id}       -> Set<token_hash>
type RefreshTokenCache interface {
	// StoreRefreshToken 存储Refresh Token
	// tokenHash: Token的SHA256哈希值（不存储原始Token）
	// data: Token关联的数据
	// ttl: Token过期时间
	StoreRefreshToken(ctx context.Context, tokenHash string, data *RefreshTokenData, ttl time.Duration) error

	// PersistRefreshToken stores a refresh-token record AND both of its
	// revocation-index memberships (the user set and the family set) as one
	// indivisible operation, refusing outright if the family has already
	// been revoked.
	//
	// Implementations MUST make all of it a single atomic operation (a
	// Redis Lua EVAL) — NOT StoreRefreshToken followed by AddToUserTokenSet
	// followed by AddToFamilyTokenSet. That sequence is what this method
	// replaces, and it had two independent defects that are the same defect
	// seen from two sides:
	//
	//   - A partial failure (write 1 commits, write 2 or 3 errors) leaves a
	//     fully redeemable 30-day record that is in NEITHER revocation
	//     index. Both DeleteTokenFamily and DeleteUserRefreshTokens work by
	//     SMEMBERS -> DEL and there is no scan-based sweep anywhere, so a
	//     hash missing from the sets is simply never named again: reuse
	//     revocation, ban/delete, logout-all-devices and forgot-password
	//     reset all miss it, for its full life. Returning an error did not
	//     mitigate this, because the record had already committed — and
	//     since MarkRotated seals the successor pair BEFORE the persist
	//     runs, the client's ordinary retry of the resulting 500 lands
	//     inside the reuse grace and is handed exactly that orphan.
	//
	//   - A concurrent DeleteTokenFamily can enumerate the family, miss the
	//     token this call is about to add, and then delete the family key —
	//     leaving the added token alive and the family set re-created
	//     holding only it. See RefreshFamilyRevokedRetention.
	//
	// Returns ErrTokenFamilyRevoked, having written NOTHING, when the
	// family is already revoked. That is a deliberate refusal and callers
	// must not treat it as an infrastructure fault.
	//
	// StoreRefreshToken / AddToUserTokenSet / AddToFamilyTokenSet remain for
	// the panel-session path (AuthService.storeRefreshToken), which has its
	// own separate family bookkeeping and is out of this branch's scope.
	PersistRefreshToken(ctx context.Context, tokenHash string, data *RefreshTokenData, ttl time.Duration) error

	// GetRefreshToken 获取Refresh Token数据
	// 返回 (data, nil) 如果Token存在
	// 返回 (nil, ErrRefreshTokenNotFound) 如果Token不存在
	// 返回 (nil, err) 如果发生其他错误
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshTokenData, error)

	// DeleteRefreshToken 删除单个Refresh Token
	// 用于Token轮转时使旧Token失效
	DeleteRefreshToken(ctx context.Context, tokenHash string) error

	// DeleteUserRefreshTokens 删除用户的所有Refresh Token
	// 用于密码更改或用户主动登出所有设备
	DeleteUserRefreshTokens(ctx context.Context, userID int64) error

	// DeleteTokenFamily 删除整个Token家族
	// 用于检测到Token重放攻击时，撤销整个会话链
	//
	// Implementations MUST mark the family revoked BEFORE enumerating it,
	// and that mark MUST be the one PersistRefreshToken refuses under.
	// Enumerate-then-delete on its own cannot see a token a concurrent
	// rotation is in the middle of writing — see
	// RefreshFamilyRevokedRetention for the interleaving.
	DeleteTokenFamily(ctx context.Context, familyID string) error

	// AddToUserTokenSet 将Token添加到用户的Token集合
	// 用于跟踪用户的所有活跃Refresh Token
	AddToUserTokenSet(ctx context.Context, userID int64, tokenHash string, ttl time.Duration) error

	// AddToFamilyTokenSet 将Token添加到家族Token集合
	// 用于跟踪同一登录会话的所有Token
	AddToFamilyTokenSet(ctx context.Context, familyID string, tokenHash string, ttl time.Duration) error

	// GetUserTokenHashes 获取用户的所有Token哈希
	// 用于批量删除用户Token
	GetUserTokenHashes(ctx context.Context, userID int64) ([]string, error)

	// GetFamilyTokenHashes 获取家族的所有Token哈希
	// 用于批量删除家族Token
	GetFamilyTokenHashes(ctx context.Context, familyID string) ([]string, error)

	// IsTokenInFamily 检查Token是否属于指定家族
	// 用于验证Token家族关系
	IsTokenInFamily(ctx context.Context, familyID string, tokenHash string) (bool, error)

	// MarkRotated atomically claims the right to rotate a refresh token, and
	// classifies a losing caller as either a forgivable replay inside the
	// reuse grace or genuine reuse.
	//
	// Implementations MUST make the whole read-decide-write ONE operation
	// (e.g. a single Redis Lua EVAL) — NOT a GetRefreshToken() followed by a
	// StoreRefreshToken(), and NOT a read here plus a decision in Go. Two
	// simultaneous presentations of one refresh token (an attacker replaying
	// a stolen token at the same instant the legitimate client refreshes on
	// schedule) could otherwise both observe Rotated=false and both mint a
	// pair from it, forking the family into two live branches that never
	// again present an already-rotated token to each other — which silently
	// disables reuse detection for good. This method is the single
	// serialization point that prevents that; see
	// OAuthTokenService.ExchangeRefreshToken.
	//
	// tombstoned is the value to store IF this call wins: callers derive it
	// from a prior read with Rotated set true and RotatedAtUnix set to now.
	// The record's existing TTL is preserved unchanged, so the tombstone
	// outlives the grace and a much later replay is still traceable to its
	// FamilyID.
	//
	// sealedReplay is the pair the caller has just minted for THIS rotation,
	// already encrypted by the caller. Implementations MUST treat it as
	// opaque bytes and MUST NOT attempt to interpret it: keeping the
	// plaintext out of the cache layer is what stops the store from ever
	// holding a usable refresh token. It is stored by the same atomic
	// operation that writes the tombstone, deliberately: if it were written
	// afterwards, a concurrent presentation landing in between would find a
	// rotated record with no pair to hand back and would revoke the family —
	// exactly the healthy-session kill the grace exists to prevent. It must
	// not be empty.
	//
	// sealedReplay may be empty ONLY when the caller already knows the
	// record is rotated (so the claim branch is unreachable and no pair
	// could be stored). Implementations MUST refuse to claim an un-rotated
	// record without a pair rather than tombstoning it with nothing to hand
	// back — that would burn the caller's token and leave every replay
	// inside the window looking like theft.
	//
	// tombstoned.PredecessorHash, when set, names the token this rotation
	// supersedes. On a successful claim the implementation MUST close that
	// predecessor's grace by overwriting its replay slot with
	// RefreshReplaySupersededMarker, in the SAME atomic operation.
	//
	// now is the caller's clock reading, and MUST be the same instant
	// stamped into tombstoned.RotatedAtUnix. It is passed in rather than read
	// inside so the grace boundary is decided by one clock the caller
	// controls, which is also what makes it testable without sleeping.
	//
	// Returns (nil, ErrRefreshTokenNotFound) when no record exists for this
	// hash. Any other error is an infrastructure fault and must be surfaced
	// as such by callers — never collapsed into an OAuth invalid_grant,
	// which would make a Redis outage indistinguishable from a client
	// presenting a bad token.
	MarkRotated(ctx context.Context, tokenHash string, tombstoned *RefreshTokenData, sealedReplay []byte, now time.Time) (*RefreshRotationResult, error)
}
