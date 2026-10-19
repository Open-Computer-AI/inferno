package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

// ConcurrencyPolicyOutboxEvent is the complete durable policy snapshot written
// by the database trigger. Keeping the nullable pair here avoids reconstructing
// policy from a later database read after a retry.
type ConcurrencyPolicyOutboxEvent struct {
	ID         int64
	EntityType string
	EntityID   int64
	Generation int64
	Policy     ConcurrencyPolicy
	Attempts   int
}

// ConcurrencyPolicyCursor is the stable keyset boundary for a bounded current
// row sweep. Entity type is part of the key because the SQL source is a union
// of account and user rows.
type ConcurrencyPolicyCursor struct {
	EntityType string
	EntityID   int64
}

// ConcurrencyPolicyOutboxRepository is deliberately narrower than the other
// outboxes: policy publication has one monotonic acknowledgement transition
// and one retry transition.
type ConcurrencyPolicyOutboxRepository interface {
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]ConcurrencyPolicyOutboxEvent, error)
	MarkPublished(ctx context.Context, id int64, workerID string) error
	RetryClaimed(ctx context.Context, id int64, workerID string, availableAt time.Time, lastError string) error
	// ListCurrent returns authoritative account/user policy rows. It repairs a
	// Redis key that was already marked published but later disappeared.
	ListCurrent(ctx context.Context, limit int, after *ConcurrencyPolicyCursor) ([]ConcurrencyPolicyOutboxEvent, error)
}

const (
	concurrencyPolicyOutboxBatch = 100
	concurrencyPolicyOutboxLease = 30 * time.Second
	concurrencyPolicyRetryDelay  = time.Second
	concurrencyPolicyCurrentSync = 30 * time.Second
)

// ConcurrencyPolicyReconciler continuously repairs the Redis projection after
// transient failures. It is started by production wiring, while ReconcileOnce
// is kept public for deterministic integration tests and controlled recovery.
type ConcurrencyPolicyReconciler struct {
	repo            ConcurrencyPolicyOutboxRepository
	publisher       ConcurrencyPolicyPublisher
	workerID        string
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	start           sync.Once
	stop            sync.Once
	currentMu       sync.Mutex
	lastCurrentSync time.Time
}

func NewConcurrencyPolicyReconciler(
	repo ConcurrencyPolicyOutboxRepository,
	publisher ConcurrencyPolicyPublisher,
) *ConcurrencyPolicyReconciler {
	ctx, cancel := context.WithCancel(context.Background())
	return &ConcurrencyPolicyReconciler{
		repo:      repo,
		publisher: publisher,
		workerID:  uuid.NewString(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func ProvideConcurrencyPolicyReconciler(
	repo ConcurrencyPolicyOutboxRepository,
	publisher ConcurrencyPolicyPublisher,
) *ConcurrencyPolicyReconciler {
	worker := NewConcurrencyPolicyReconciler(repo, publisher)
	worker.Start()
	return worker
}

func (w *ConcurrencyPolicyReconciler) Start() {
	if w == nil || w.repo == nil || w.publisher == nil {
		return
	}
	w.start.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.loop()
		}()
	})
}

func (w *ConcurrencyPolicyReconciler) Stop() {
	if w == nil {
		return
	}
	w.stop.Do(func() { w.cancel() })
	w.wg.Wait()
}

func (w *ConcurrencyPolicyReconciler) loop() {
	// Reconcile immediately so startup recovery does not wait for the first
	// ticker interval, then continue retrying without a process restart.
	for {
		if err := w.ReconcileOnce(w.ctx); err != nil {
			logger.LegacyPrintf("service.concurrency_policy", "reconcile failed: %v", err)
		}
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(concurrencyPolicyRetryDelay):
		}
	}
}

// ReconcileOnce claims a bounded batch and publishes each complete policy.
// A stale event is safe to retire because a newer generation already won the
// Redis CAS; every other error remains durable for a later retry.
func (w *ConcurrencyPolicyReconciler) ReconcileOnce(ctx context.Context) error {
	if w == nil || w.repo == nil || w.publisher == nil {
		return errors.New("concurrency policy reconciler is not configured")
	}
	events, err := w.repo.Claim(ctx, w.workerID, concurrencyPolicyOutboxBatch, concurrencyPolicyOutboxLease)
	if err != nil {
		return err
	}
	var firstErr error
	for _, event := range events {
		if err := ValidateConcurrencyPolicy(event.Policy); err != nil {
			firstErr = w.retry(ctx, event, err, firstErr)
			continue
		}

		publishErr := w.publishEvent(ctx, event)
		if publishErr == nil {
			if markErr := w.repo.MarkPublished(ctx, event.ID, w.workerID); markErr != nil {
				firstErr = markErr
			}
			continue
		}
		firstErr = w.retry(ctx, event, publishErr, firstErr)
	}
	currentErr := w.reconcileCurrentIfDue(ctx)
	if firstErr != nil {
		return firstErr
	}
	return currentErr
}

func (w *ConcurrencyPolicyReconciler) publishEvent(ctx context.Context, event ConcurrencyPolicyOutboxEvent) error {
	var publishErr error
	switch event.EntityType {
	case "account":
		publishErr = w.publisher.PublishAccountConcurrencyPolicy(ctx, event.EntityID, event.Policy, event.Generation)
	case "user":
		publishErr = w.publisher.PublishUserConcurrencyPolicy(ctx, event.EntityID, event.Policy, event.Generation)
	default:
		publishErr = fmt.Errorf("unknown concurrency policy entity type %q", event.EntityType)
	}
	if errors.Is(publishErr, ErrConcurrencyPolicyGenerationStale) {
		return nil
	}
	return publishErr
}

// ReconcileCurrentOnce republishes the current durable rows without relying
// on an unpublished outbox event. This is the recovery path for Redis key loss
// after a prior publication was acknowledged.
func (w *ConcurrencyPolicyReconciler) ReconcileCurrentOnce(ctx context.Context) error {
	if w == nil || w.repo == nil || w.publisher == nil {
		return errors.New("concurrency policy reconciler is not configured")
	}
	var firstErr error
	var cursor *ConcurrencyPolicyCursor
	for {
		events, err := w.repo.ListCurrent(ctx, concurrencyPolicyOutboxBatch, cursor)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			if err := ValidateConcurrencyPolicy(event.Policy); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if err := w.publishEvent(ctx, event); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if len(events) < concurrencyPolicyOutboxBatch {
			break
		}
		last := events[len(events)-1]
		cursor = &ConcurrencyPolicyCursor{EntityType: last.EntityType, EntityID: last.EntityID}
	}
	return firstErr
}

func (w *ConcurrencyPolicyReconciler) reconcileCurrentIfDue(ctx context.Context) error {
	now := time.Now()
	w.currentMu.Lock()
	if !w.lastCurrentSync.IsZero() && now.Sub(w.lastCurrentSync) < concurrencyPolicyCurrentSync {
		w.currentMu.Unlock()
		return nil
	}
	w.currentMu.Unlock()

	err := w.ReconcileCurrentOnce(ctx)
	if err == nil {
		w.currentMu.Lock()
		w.lastCurrentSync = now
		w.currentMu.Unlock()
	}
	return err
}

func (w *ConcurrencyPolicyReconciler) retry(
	ctx context.Context,
	event ConcurrencyPolicyOutboxEvent,
	err error,
	firstErr error,
) error {
	availableAt := time.Now().Add(concurrencyPolicyRetryDelay)
	if retryErr := w.repo.RetryClaimed(ctx, event.ID, w.workerID, availableAt, err.Error()); retryErr != nil {
		if firstErr != nil {
			return firstErr
		}
		return retryErr
	}
	if firstErr != nil {
		return firstErr
	}
	return err
}
