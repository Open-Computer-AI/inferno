package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrLiveReservationLost        = errors.New("live reservation lost")
	ErrLiveReservationReleased    = errors.New("live reservation released")
	ErrLiveReservationTransferred = errors.New("live reservation already transferred")
)

const liveReservationOperationTimeout = 3 * time.Second

// LiveReservation owns one policy-aware Live admission from acquisition until
// the created call record takes ownership. It never acquires an ordinary slot
// and it never reacquires during transfer.
type LiveReservation struct {
	cache   ConcurrencyAdmissionCache
	request ConcurrencyAdmissionRequest

	mu          sync.Mutex
	released    bool
	transferred bool
	lost        bool

	refreshCancel   context.CancelCauseFunc
	refreshDone     chan struct{}
	ownershipCtx    context.Context
	ownershipCancel context.CancelCauseFunc
	releaseOnce     sync.Once
	releaseErr      error
}

// AcquireLiveReservation reserves the account and user Live slots atomically.
// A denied result returns no handle; callers must not release a non-existent
// reservation or fall back to ordinary account/user slots.
func (s *ConcurrencyService) AcquireLiveReservation(
	ctx context.Context,
	request ConcurrencyAdmissionRequest,
) (ConcurrencyAdmissionResult, *LiveReservation, error) {
	if request.Class != ConcurrencyClassLive {
		return ConcurrencyAdmissionResult{}, nil, errors.New("live reservation requires the Live admission class")
	}
	if err := request.Validate(); err != nil {
		return ConcurrencyAdmissionResult{}, nil, err
	}
	cache, err := s.policyAdmissionCache()
	if err != nil {
		return ConcurrencyAdmissionResult{}, nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := cache.AcquireConcurrency(ctx, request)
	if err != nil || !result.Acquired {
		return result, nil, err
	}
	return result, &LiveReservation{
		cache:   cache,
		request: cloneConcurrencyAdmissionRequest(request),
	}, nil
}

// Refresh renews the existing ownership. The Redis implementation validates
// the current policy document but intentionally does not require this old
// admission snapshot's generation to remain current.
func (r *LiveReservation) Refresh(ctx context.Context) (bool, error) {
	if r == nil {
		return false, ErrLiveReservationReleased
	}
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return false, ErrLiveReservationReleased
	}
	request := cloneConcurrencyAdmissionRequest(r.request)
	cache := r.cache
	r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	owned, err := cache.RefreshConcurrency(ctx, request)
	if err != nil || !owned {
		r.mu.Lock()
		r.lost = true
		cancel := r.ownershipCancel
		r.mu.Unlock()
		if cancel != nil {
			cancel(ErrLiveReservationLost)
		}
		if err == nil {
			err = ErrLiveReservationLost
		}
	}
	return owned, err
}

// Lost reports whether a refresh has confirmed that the distributed owner is
// gone. Creation must fail rather than transfer a record after that point.
func (r *LiveReservation) Lost() bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	lost := r.lost
	r.mu.Unlock()
	return lost
}

// Context returns a context canceled when refresh loses distributed ownership.
// Creation uses it for the upstream request so a lost lease cannot wait for a
// slow network operation and then create an unowned Live session.
func (r *LiveReservation) Context() context.Context {
	if r == nil {
		return context.Background()
	}
	r.mu.Lock()
	ctx := r.ownershipCtx
	r.mu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// StartRefresh renews ownership while an upstream Live call is being created.
// The returned function is idempotent and waits for the refresh goroutine.
func (r *LiveReservation) StartRefresh(ctx context.Context) func() {
	if r == nil {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if r.released || r.refreshCancel != nil {
		r.mu.Unlock()
		return func() {}
	}
	refreshCtx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	r.refreshCancel = cancel
	r.refreshDone = done
	r.ownershipCtx = refreshCtx
	r.ownershipCancel = cancel
	r.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(liveReservationRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				operationCtx, operationCancel := context.WithTimeout(context.Background(), liveReservationOperationTimeout)
				owned, err := r.Refresh(operationCtx)
				operationCancel()
				if err != nil || !owned {
					return
				}
			}
		}
	}()

	return func() {
		cancel(ErrLiveReservationReleased)
		<-done
	}
}

// TransferToCall moves the existing reservation into the durable Live record.
// It is deliberately a state transition, not another admission call.
func (r *LiveReservation) TransferToCall(record *LiveCallRecord) error {
	if r == nil {
		return ErrLiveReservationReleased
	}
	if record == nil {
		return errors.New("live call record is required")
	}
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return ErrLiveReservationReleased
	}
	if r.lost {
		r.mu.Unlock()
		return ErrLiveReservationLost
	}
	if r.transferred {
		r.mu.Unlock()
		return ErrLiveReservationTransferred
	}
	request := cloneConcurrencyAdmissionRequest(r.request)
	if record.AccountID != 0 && record.AccountID != request.AccountID {
		r.mu.Unlock()
		return errors.New("live record account does not match reservation")
	}
	if record.UserID != 0 && record.UserID != request.UserID {
		r.mu.Unlock()
		return errors.New("live record user does not match reservation")
	}
	if record.APIKeyID != 0 && record.APIKeyID != request.APIKeyID {
		r.mu.Unlock()
		return errors.New("live record API key does not match reservation")
	}
	if record.LeaseID != "" && record.LeaseID != request.ReservationID {
		r.mu.Unlock()
		return errors.New("live record lease does not match reservation")
	}
	refreshCancel := r.refreshCancel
	refreshDone := r.refreshDone
	r.mu.Unlock()
	if refreshCancel != nil {
		refreshCancel(ErrLiveReservationTransferred)
		if refreshDone != nil {
			<-refreshDone
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released {
		return ErrLiveReservationReleased
	}
	if r.lost {
		return ErrLiveReservationLost
	}
	if r.transferred {
		return ErrLiveReservationTransferred
	}
	record.AccountID = request.AccountID
	record.UserID = request.UserID
	record.APIKeyID = request.APIKeyID
	record.LeaseID = request.ReservationID
	record.AccountConcurrency = request.AccountPolicy.Shared
	record.AccountLiveConcurrency = cloneOptionalInt(request.AccountPolicy.Live)
	record.AccountRequestConcurrency = cloneOptionalInt(request.AccountPolicy.Request)
	record.AccountConcurrencyPolicyGeneration = request.AccountGeneration
	record.UserConcurrency = request.UserPolicy.Shared
	record.UserLiveConcurrency = cloneOptionalInt(request.UserPolicy.Live)
	record.UserRequestConcurrency = cloneOptionalInt(request.UserPolicy.Request)
	record.UserConcurrencyPolicyGeneration = request.UserGeneration
	record.ConcurrencyPolicySnapshotPresent = true
	record.reservation = r
	r.transferred = true
	return nil
}

// Release relinquishes the exact owned reservation at most once. It is safe
// to call from both creation-error and finalization paths.
func (r *LiveReservation) Release(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.releaseOnce.Do(func() {
		r.mu.Lock()
		r.released = true
		cancel := r.refreshCancel
		ownershipCancel := r.ownershipCancel
		done := r.refreshDone
		request := cloneConcurrencyAdmissionRequest(r.request)
		cache := r.cache
		r.mu.Unlock()
		if cancel != nil {
			cancel(ErrLiveReservationReleased)
		} else if ownershipCancel != nil {
			ownershipCancel(ErrLiveReservationReleased)
		}
		if done != nil {
			<-done
		}
		// Release is cleanup, so an already-canceled creation/request context
		// must not prevent Redis ownership from being removed.
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), liveReservationOperationTimeout)
		defer releaseCancel()
		r.releaseErr = cache.ReleaseConcurrency(releaseCtx, request)
	})
	return r.releaseErr
}

func cloneConcurrencyAdmissionRequest(request ConcurrencyAdmissionRequest) ConcurrencyAdmissionRequest {
	request.AccountPolicy = cloneConcurrencyPolicy(request.AccountPolicy)
	request.UserPolicy = cloneConcurrencyPolicy(request.UserPolicy)
	return request
}

func cloneConcurrencyPolicy(policy ConcurrencyPolicy) ConcurrencyPolicy {
	return ConcurrencyPolicy{
		Shared:  policy.Shared,
		Live:    cloneOptionalInt(policy.Live),
		Request: cloneOptionalInt(policy.Request),
	}
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
