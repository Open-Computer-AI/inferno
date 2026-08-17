package service

import (
	"context"
	"errors"
	"time"
)

// ErrRefreshTokenNotFound is returned when a refresh token is not found in cache.
// This is used to abstract away the underlying cache implementation (e.g., redis.Nil).
var ErrRefreshTokenNotFound = errors.New("refresh token not found")

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

	// MarkRotated atomically marks a refresh token record as rotated,
	// GETting its current value and SETting the tombstoned replacement in
	// ONE operation. Implementations MUST make the get-and-set atomic (e.g.
	// a Redis Lua script) — NOT a separate GetRefreshToken() followed by
	// StoreRefreshToken(), which race under concurrent redemption of the
	// same token: two simultaneous presentations of one refresh token could
	// otherwise both observe Rotated=false and both proceed to mint a token
	// pair from it. See OAuthTokenService.ExchangeRefreshToken, which uses
	// this as the single serialization point deciding who gets to rotate.
	//
	// tombstoned is the value to store IF this call wins the race (i.e. the
	// record was not already rotated) — callers derive it from a prior read
	// with Rotated set true. The record's existing TTL is preserved
	// unchanged.
	//
	// Returns:
	//   - (data, false, nil): this call won. data reflects the record as it
	//     stood immediately before the mark (Rotated=false). tombstoned has
	//     been stored.
	//   - (data, true, nil): this call lost — the record was already
	//     rotated, by either a genuine replay of an old token or the losing
	//     side of a concurrent double-presentation. Nothing was written.
	//     data reflects the (already-rotated) record as found — still
	//     useful for its FamilyID.
	//   - (nil, false, ErrRefreshTokenNotFound): no record exists for this hash.
	MarkRotated(ctx context.Context, tokenHash string, tombstoned *RefreshTokenData) (data *RefreshTokenData, alreadyRotated bool, err error)
}
