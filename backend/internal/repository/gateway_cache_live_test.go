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

func TestGatewayCacheLiveCallIdentityAndController(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	otherInstance, ok := NewGatewayCache(client).(service.LiveCallStore)
	require.True(t, ok)
	record := &service.LiveCallRecord{
		CallID:                "call_secret",
		CallHash:              HashLiveCallID("call_secret"),
		AccountID:             11,
		APIKeyID:              22,
		UserID:                33,
		GroupID:               44,
		LeaseID:               "lease",
		Model:                 "gpt-live-test",
		AttestationCiphertext: "encrypted-attestation",
		UpstreamSessionID:     "session-from-create",
		UpstreamThreadID:      "thread-from-create",
		CreatedAt:             time.Now(),
		ExpiresAt:             time.Now().Add(time.Hour),
		Controller:            service.LiveControllerPending,
	}
	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))

	loaded, err := otherInstance.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.CallID, loaded.CallID)
	require.Equal(t, record.AccountID, loaded.AccountID)
	require.Equal(t, record.AttestationCiphertext, loaded.AttestationCiphertext)
	require.Equal(t, record.UpstreamSessionID, loaded.UpstreamSessionID)
	require.Equal(t, record.UpstreamThreadID, loaded.UpstreamThreadID)

	claimed, err := cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerObserver, "observer-1")
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = cache.ClaimLiveController(context.Background(), record.CallHash, service.LiveControllerProxy, "proxy-1")
	require.NoError(t, err)
	require.True(t, claimed)
	controller, err := cache.GetLiveController(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, service.LiveControllerProxy, controller)

	released, err := cache.ReleaseLiveController(context.Background(), record.CallHash, "proxy-1")
	require.NoError(t, err)
	require.True(t, released)
	closed, err := cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.True(t, closed)
	closed, err = cache.MarkLiveCallClosed(context.Background(), record.CallHash, time.Hour)
	require.NoError(t, err)
	require.False(t, closed)
}

func TestGatewayCacheLiveCallRoundTripsConcurrencyPolicySnapshot(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewGatewayCache(client).(service.LiveCallStore)
	liveUser, requestUser := 3, 1
	record := &service.LiveCallRecord{
		CallID:                             "call-split-policy",
		CallHash:                           HashLiveCallID("call-split-policy"),
		AccountID:                          11,
		APIKeyID:                           22,
		UserID:                             33,
		LeaseID:                            "lease-split-policy",
		Model:                              "gpt-live-test",
		CreatedAt:                          time.Now(),
		ExpiresAt:                          time.Now().Add(time.Hour),
		Controller:                         service.LiveControllerPending,
		AccountConcurrency:                 8,
		AccountConcurrencyPolicyGeneration: 7,
		UserConcurrency:                    6,
		UserLiveConcurrency:                &liveUser,
		UserRequestConcurrency:             &requestUser,
		UserConcurrencyPolicyGeneration:    9,
		ConcurrencyPolicySnapshotPresent:   true,
	}

	require.NoError(t, cache.SaveLiveCall(context.Background(), record, time.Hour))
	loaded, err := cache.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.AccountConcurrency, loaded.AccountConcurrency)
	require.Equal(t, record.AccountLiveConcurrency, loaded.AccountLiveConcurrency)
	require.Equal(t, record.AccountRequestConcurrency, loaded.AccountRequestConcurrency)
	require.Equal(t, record.AccountConcurrencyPolicyGeneration, loaded.AccountConcurrencyPolicyGeneration)
	require.Equal(t, record.UserConcurrency, loaded.UserConcurrency)
	require.Equal(t, record.UserLiveConcurrency, loaded.UserLiveConcurrency)
	require.Equal(t, record.UserRequestConcurrency, loaded.UserRequestConcurrency)
	require.Equal(t, record.UserConcurrencyPolicyGeneration, loaded.UserConcurrencyPolicyGeneration)
	require.True(t, loaded.ConcurrencyPolicySnapshotPresent)
}

func TestGatewayCacheLiveCallRejectsInvalidConcurrencyPolicySnapshotSemantics(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewGatewayCache(client).(service.LiveCallStore)
	baseFields := map[string]any{
		"call_id":                               "call-invalid-policy",
		"account_id":                            11,
		"api_key_id":                            22,
		"user_id":                               33,
		"lease_id":                              "invalid-policy-lease",
		"controller":                            service.LiveControllerPending,
		"concurrency_policy_snapshot_present":   "1",
		"account_concurrency":                   "8",
		"account_live_concurrency":              "",
		"account_request_concurrency":           "",
		"account_concurrency_policy_generation": "7",
		"user_concurrency":                      "6",
		"user_live_concurrency":                 "3",
		"user_request_concurrency":              "1",
		"user_concurrency_policy_generation":    "9",
	}
	cases := []struct {
		name  string
		field string
		value any
	}{
		{name: "partial account split", field: "account_live_concurrency", value: "4"},
		{name: "zero split limit", field: "user_live_concurrency", value: "0"},
		{name: "partial user split", field: "user_live_concurrency", value: ""},
		{name: "negative account generation", field: "account_concurrency_policy_generation", value: "-1"},
		{name: "negative user generation", field: "user_concurrency_policy_generation", value: "-1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fields := make(map[string]any, len(baseFields))
			for field, value := range baseFields {
				fields[field] = value
			}
			fields[testCase.field] = testCase.value
			callHash := HashLiveCallID("call-invalid-policy-" + testCase.name)
			require.NoError(t, client.HSet(context.Background(), liveCallKey(callHash), fields).Err())

			loaded, err := cache.GetLiveCall(context.Background(), callHash)
			require.Error(t, err)
			require.Nil(t, loaded)
		})
	}
}

func TestGatewayCacheLiveCallLegacyRecordHasNoConcurrencyPolicySnapshot(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewGatewayCache(client).(service.LiveCallStore)
	callHash := HashLiveCallID("call-legacy-policy")
	require.NoError(t, client.HSet(context.Background(), liveCallKey(callHash), map[string]any{
		"call_id":    "call-legacy-policy",
		"account_id": 11,
		"api_key_id": 22,
		"user_id":    33,
		"lease_id":   "legacy-lease",
		"controller": service.LiveControllerPending,
	}).Err())

	loaded, err := cache.GetLiveCall(context.Background(), callHash)
	require.NoError(t, err)
	require.False(t, loaded.ConcurrencyPolicySnapshotPresent)
	require.Nil(t, loaded.AccountLiveConcurrency)
	require.Nil(t, loaded.UserRequestConcurrency)
}

func TestGatewayCacheLiveCallRejectsMalformedExplicitConcurrencyPolicySnapshot(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	cache := NewGatewayCache(client).(service.LiveCallStore)
	callHash := HashLiveCallID("call-malformed-policy")
	require.NoError(t, client.HSet(context.Background(), liveCallKey(callHash), map[string]any{
		"call_id":                             "call-malformed-policy",
		"account_id":                          11,
		"api_key_id":                          22,
		"user_id":                             33,
		"lease_id":                            "malformed-lease",
		"controller":                          service.LiveControllerPending,
		"concurrency_policy_snapshot_present": "1",
		"account_concurrency":                 "not-an-integer",
	}).Err())

	loaded, err := cache.GetLiveCall(context.Background(), callHash)
	require.Error(t, err)
	require.Nil(t, loaded)
}
