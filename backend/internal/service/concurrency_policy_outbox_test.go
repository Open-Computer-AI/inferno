package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type concurrencyPolicyOutboxRepoStub struct {
	event       ConcurrencyPolicyOutboxEvent
	published   bool
	claimed     bool
	retryErrors []string
}

func (r *concurrencyPolicyOutboxRepoStub) Claim(context.Context, string, int, time.Duration) ([]ConcurrencyPolicyOutboxEvent, error) {
	if r.published || r.claimed {
		return nil, nil
	}
	r.claimed = true
	return []ConcurrencyPolicyOutboxEvent{r.event}, nil
}

func (r *concurrencyPolicyOutboxRepoStub) MarkPublished(context.Context, int64, string) error {
	r.published = true
	r.claimed = false
	return nil
}

func (r *concurrencyPolicyOutboxRepoStub) RetryClaimed(_ context.Context, _ int64, _ string, _ time.Time, lastError string) error {
	r.retryErrors = append(r.retryErrors, lastError)
	r.claimed = false
	return nil
}

func (r *concurrencyPolicyOutboxRepoStub) ListCurrent(context.Context, int, *ConcurrencyPolicyCursor) ([]ConcurrencyPolicyOutboxEvent, error) {
	return nil, nil
}

type concurrencyPolicyPublisherStub struct {
	accountCalls int
	userCalls    int
	err          error
}

func (p *concurrencyPolicyPublisherStub) PublishAccountConcurrencyPolicy(context.Context, int64, ConcurrencyPolicy, int64) error {
	p.accountCalls++
	return p.err
}

func (p *concurrencyPolicyPublisherStub) PublishUserConcurrencyPolicy(context.Context, int64, ConcurrencyPolicy, int64) error {
	p.userCalls++
	return p.err
}

func TestConcurrencyPolicyReconcilerRetriesRuntimePublication(t *testing.T) {
	repo := &concurrencyPolicyOutboxRepoStub{
		event: ConcurrencyPolicyOutboxEvent{
			ID:         7,
			EntityType: "user",
			EntityID:   11,
			Generation: 4,
			Policy:     ConcurrencyPolicy{Shared: 9},
		},
	}
	publisher := &concurrencyPolicyPublisherStub{err: errors.New("redis unavailable")}
	reconciler := NewConcurrencyPolicyReconciler(repo, publisher)

	err := reconciler.ReconcileOnce(context.Background())
	require.ErrorContains(t, err, "redis unavailable")
	require.Len(t, repo.retryErrors, 1)
	require.False(t, repo.published)

	publisher.err = nil
	require.NoError(t, reconciler.ReconcileOnce(context.Background()))
	require.True(t, repo.published)
	require.Equal(t, 2, publisher.userCalls)
}

func TestConcurrencyPolicyReconcilerRetiresAlreadyStaleGeneration(t *testing.T) {
	repo := &concurrencyPolicyOutboxRepoStub{
		event: ConcurrencyPolicyOutboxEvent{
			ID:         8,
			EntityType: "account",
			EntityID:   12,
			Generation: 2,
			Policy:     ConcurrencyPolicy{Shared: 4},
		},
	}
	publisher := &concurrencyPolicyPublisherStub{err: ErrConcurrencyPolicyGenerationStale}
	reconciler := NewConcurrencyPolicyReconciler(repo, publisher)

	require.NoError(t, reconciler.ReconcileOnce(context.Background()))
	require.True(t, repo.published, "a stale durable event is safe to retire after a newer CAS winner")
	require.Empty(t, repo.retryErrors)
	require.Equal(t, 1, publisher.accountCalls)
}

func TestConcurrencyPolicyReconcilerRetriesMalformedSnapshot(t *testing.T) {
	repo := &concurrencyPolicyOutboxRepoStub{
		event: ConcurrencyPolicyOutboxEvent{
			ID:         9,
			EntityType: "user",
			EntityID:   13,
			Generation: 3,
			Policy:     ConcurrencyPolicy{Live: pointer(0), Request: pointer(2)},
		},
	}
	reconciler := NewConcurrencyPolicyReconciler(repo, &concurrencyPolicyPublisherStub{})

	err := reconciler.ReconcileOnce(context.Background())
	require.Error(t, err)
	require.Contains(t, repo.retryErrors[0], "positive")
	require.False(t, repo.published)
}

func pointer(value int) *int { return &value }
