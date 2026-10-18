package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type currentPolicyOutboxRepo struct {
	events    []service.ConcurrencyPolicyOutboxEvent
	listCalls int
}

func (r *currentPolicyOutboxRepo) Claim(context.Context, string, int, time.Duration) ([]service.ConcurrencyPolicyOutboxEvent, error) {
	return nil, nil
}

func (r *currentPolicyOutboxRepo) MarkPublished(context.Context, int64, string) error { return nil }

func (r *currentPolicyOutboxRepo) RetryClaimed(context.Context, int64, string, time.Time, string) error {
	return nil
}

func (r *currentPolicyOutboxRepo) ListCurrent(_ context.Context, limit int, after *service.ConcurrencyPolicyCursor) ([]service.ConcurrencyPolicyOutboxEvent, error) {
	r.listCalls++
	start := 0
	if after != nil {
		for start < len(r.events) && (r.events[start].EntityType < after.EntityType ||
			(r.events[start].EntityType == after.EntityType && r.events[start].EntityID <= after.EntityID)) {
			start++
		}
	}
	if start >= len(r.events) {
		return nil, nil
	}
	end := start + limit
	if end > len(r.events) {
		end = len(r.events)
	}
	return r.events[start:end], nil
}

func TestConcurrencyPolicyPublicationIsMonotonicAndFailClosed(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb}
	ctx := context.Background()

	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 17, service.ConcurrencyPolicy{Shared: 4}, 0))
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 17, service.ConcurrencyPolicy{Shared: 4}, 0), "same generation/value is idempotent")

	live, requests := 4, 2
	split := service.ConcurrencyPolicy{Shared: 4, Live: &live, Request: &requests}
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 17, split, 1))
	require.ErrorIs(t,
		cache.PublishAccountConcurrencyPolicy(ctx, 17, service.ConcurrencyPolicy{Shared: 99}, 0),
		service.ErrConcurrencyPolicyGenerationStale,
	)
	conflictingLive := 5
	require.ErrorIs(t,
		cache.PublishAccountConcurrencyPolicy(ctx, 17, service.ConcurrencyPolicy{Shared: 4, Live: &conflictingLive, Request: &requests}, 1),
		service.ErrConcurrencyPolicyGenerationConflict,
	)

	key := accountConcurrencyPolicyKey(17)
	require.Equal(t, "1", mini.HGet(key, "generation"))
	require.Equal(t, "split", mini.HGet(key, "mode"))
	require.Equal(t, "4", mini.HGet(key, "live"))
	require.Equal(t, "2", mini.HGet(key, "request"))

	mini.HSet(key, "generation", "not-a-number")
	err := cache.PublishAccountConcurrencyPolicy(ctx, 17, split, 2)
	require.Error(t, err)
	require.False(t, errors.Is(err, service.ErrConcurrencyPolicyGenerationStale), "malformed authority must not be treated as an ordinary stale snapshot")
}

func TestConcurrencyPolicyPublicationRejectsMalformedHigherGeneration(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb}
	ctx := context.Background()

	key := accountConcurrencyPolicyKey(18)
	mini.HSet(key, "generation", "99")
	err := cache.PublishAccountConcurrencyPolicy(ctx, 18, service.ConcurrencyPolicy{Shared: 4}, 1)
	if err == nil {
		t.Fatal("publication unexpectedly accepted a malformed higher-generation authority")
	}
	if errors.Is(err, service.ErrConcurrencyPolicyGenerationStale) {
		t.Fatalf("malformed higher-generation authority was treated as stale: %v", err)
	}

	mini.Del(key)
	mini.HSet(key,
		"generation", "99",
		"mode", "split",
		"shared", "4",
		"live", "not-a-limit",
		"request", "2",
	)
	err = cache.PublishAccountConcurrencyPolicy(ctx, 18, service.ConcurrencyPolicy{Shared: 4}, 1)
	if err == nil {
		t.Fatal("publication unexpectedly accepted an invalid higher-generation authority")
	}
	if errors.Is(err, service.ErrConcurrencyPolicyGenerationStale) {
		t.Fatalf("invalid higher-generation authority was treated as stale: %v", err)
	}
}

func TestConcurrencyPolicyReconcilerRestoresPublishedPolicyAfterRedisKeyLoss(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb}
	ctx := context.Background()
	key := accountConcurrencyPolicyKey(19)
	policy := service.ConcurrencyPolicy{Shared: 6}
	repo := &currentPolicyOutboxRepo{events: []service.ConcurrencyPolicyOutboxEvent{{
		EntityType: "account",
		EntityID:   19,
		Generation: 7,
		Policy:     policy,
	}}}
	reconciler := service.NewConcurrencyPolicyReconciler(repo, cache)

	// The event has already been acknowledged, so only the authoritative-row
	// reconciliation can repair this simulated Redis key loss.
	require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, 19, policy, 7))
	mini.Del(key)
	require.NoError(t, reconciler.ReconcileCurrentOnce(ctx))
	require.Equal(t, "7", mini.HGet(key, "generation"))
	require.Equal(t, "6", mini.HGet(key, "shared"))
}

func TestConcurrencyPolicyReconcilerPaginatesCurrentRowsPastFirstBatch(t *testing.T) {
	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &concurrencyCache{rdb: rdb}
	ctx := context.Background()
	events := make([]service.ConcurrencyPolicyOutboxEvent, 101)
	keys := make([]string, len(events))
	for i := range events {
		id := int64(1000 + i)
		events[i] = service.ConcurrencyPolicyOutboxEvent{
			EntityType: "account",
			EntityID:   id,
			Generation: 3,
			Policy:     service.ConcurrencyPolicy{Shared: 1},
		}
		keys[i] = accountConcurrencyPolicyKey(id)
		require.NoError(t, cache.PublishAccountConcurrencyPolicy(ctx, id, events[i].Policy, events[i].Generation))
	}
	require.NoError(t, rdb.Del(ctx, keys...).Err())
	repo := &currentPolicyOutboxRepo{events: events}
	reconciler := service.NewConcurrencyPolicyReconciler(repo, cache)

	require.NoError(t, reconciler.ReconcileCurrentOnce(ctx))
	require.Equal(t, 2, repo.listCalls, "current reconciliation must use a keyset cursor after the first 100 rows")
	require.Equal(t, "3", mini.HGet(keys[0], "generation"))
	require.Equal(t, "3", mini.HGet(keys[len(keys)-1], "generation"))
}
