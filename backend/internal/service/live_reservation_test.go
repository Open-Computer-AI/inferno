//go:build unit

package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type liveReservationCacheForTest struct {
	stubConcurrencyCacheForTest
	acquireResult   ConcurrencyAdmissionResult
	acquireErr      error
	refreshResult   bool
	refreshErr      error
	acquireCalls    atomic.Int64
	refreshCalls    atomic.Int64
	releaseCalls    atomic.Int64
	refreshObserved chan struct{}
}

func (c *liveReservationCacheForTest) AcquireConcurrency(context.Context, ConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error) {
	c.acquireCalls.Add(1)
	return c.acquireResult, c.acquireErr
}

func (c *liveReservationCacheForTest) RefreshConcurrency(context.Context, ConcurrencyAdmissionRequest) (bool, error) {
	c.refreshCalls.Add(1)
	if c.refreshObserved != nil {
		select {
		case c.refreshObserved <- struct{}{}:
		default:
		}
	}
	return c.refreshResult, c.refreshErr
}

func (c *liveReservationCacheForTest) ReleaseConcurrency(context.Context, ConcurrencyAdmissionRequest) error {
	c.releaseCalls.Add(1)
	return nil
}

var _ ConcurrencyCache = (*liveReservationCacheForTest)(nil)
var _ ConcurrencyAdmissionCache = (*liveReservationCacheForTest)(nil)

func TestLiveReservation_AcquiresRefreshesAndTransfersWithoutReacquire(t *testing.T) {
	live, requests := 4, 2
	request := ConcurrencyAdmissionRequest{
		AccountID:         11,
		AccountPolicy:     ConcurrencyPolicy{Live: &live, Request: &requests},
		AccountGeneration: 3,
		UserID:            22,
		UserPolicy:        ConcurrencyPolicy{Live: &live, Request: &requests},
		UserGeneration:    4,
		APIKeyID:          33,
		Class:             ConcurrencyClassLive,
		ReservationID:     "live-reservation-1",
	}
	cache := &liveReservationCacheForTest{
		acquireResult: ConcurrencyAdmissionResult{Acquired: true},
		refreshResult: true,
	}
	svc := NewConcurrencyService(cache)

	result, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.NotNil(t, reservation)
	require.Equal(t, int64(1), cache.acquireCalls.Load())

	refreshed, err := reservation.Refresh(context.Background())
	require.NoError(t, err)
	require.True(t, refreshed)

	record := &LiveCallRecord{CallID: "call-1"}
	require.NoError(t, reservation.TransferToCall(record))
	require.Equal(t, int64(11), record.AccountID)
	require.Equal(t, int64(22), record.UserID)
	require.Equal(t, "live-reservation-1", record.LeaseID)
	require.Equal(t, int64(3), record.AccountConcurrencyPolicyGeneration)
	require.Equal(t, int64(4), record.UserConcurrencyPolicyGeneration)
	require.Same(t, reservation, record.reservation)
	require.Equal(t, int64(1), cache.acquireCalls.Load(), "transfer must not reacquire a Live slot")

	require.NoError(t, reservation.Release(context.Background()))
	require.NoError(t, reservation.Release(context.Background()))
	require.Equal(t, int64(1), cache.releaseCalls.Load(), "release must be idempotent")
}

func TestLiveReservation_RenewsDuringCreationWindow(t *testing.T) {
	oldInterval := liveReservationRefreshInterval
	liveReservationRefreshInterval = time.Millisecond
	t.Cleanup(func() { liveReservationRefreshInterval = oldInterval })

	cache := &liveReservationCacheForTest{
		acquireResult:   ConcurrencyAdmissionResult{Acquired: true},
		refreshResult:   true,
		refreshObserved: make(chan struct{}, 1),
	}
	svc := NewConcurrencyService(cache)
	live, requests := 4, 2
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-renewal-1",
	}
	_, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	stop := reservation.StartRefresh(context.Background())
	t.Cleanup(stop)

	select {
	case <-cache.refreshObserved:
	case <-time.After(time.Second):
		t.Fatal("Live reservation was not renewed during the creation window")
	}
	require.GreaterOrEqual(t, cache.refreshCalls.Load(), int64(1))
}

func TestLiveReservation_TransferRejectsSecondOwner(t *testing.T) {
	live, requests := 4, 2
	cache := &liveReservationCacheForTest{acquireResult: ConcurrencyAdmissionResult{Acquired: true}}
	svc := NewConcurrencyService(cache)
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-transfer-1",
	}
	_, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	require.NoError(t, reservation.TransferToCall(&LiveCallRecord{CallID: "first"}))
	require.Error(t, reservation.TransferToCall(&LiveCallRecord{CallID: "second"}))
}

func TestLiveReservation_TransferRejectsDifferentAPIKey(t *testing.T) {
	live, requests := 4, 2
	cache := &liveReservationCacheForTest{acquireResult: ConcurrencyAdmissionResult{Acquired: true}}
	svc := NewConcurrencyService(cache)
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-transfer-api-key",
	}
	_, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	require.Error(t, reservation.TransferToCall(&LiveCallRecord{CallID: "wrong-key", APIKeyID: 34}))
	require.NoError(t, reservation.TransferToCall(&LiveCallRecord{CallID: "right-key", APIKeyID: 33}))
}

func TestLiveReservation_RefreshLossCancelsCreationContext(t *testing.T) {
	oldInterval := liveReservationRefreshInterval
	liveReservationRefreshInterval = time.Millisecond
	t.Cleanup(func() { liveReservationRefreshInterval = oldInterval })

	cache := &liveReservationCacheForTest{
		acquireResult: ConcurrencyAdmissionResult{Acquired: true},
		refreshResult: false,
	}
	svc := NewConcurrencyService(cache)
	live, requests := 4, 2
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-refresh-loss",
	}
	_, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	stop := reservation.StartRefresh(context.Background())
	t.Cleanup(stop)

	select {
	case <-reservation.Context().Done():
		require.ErrorIs(t, context.Cause(reservation.Context()), ErrLiveReservationLost)
	case <-time.After(time.Second):
		t.Fatal("refresh loss did not cancel the creation context")
	}
}

func TestLiveReservation_TransferStopsCreationRenewerButKeepsOwnership(t *testing.T) {
	oldInterval := liveReservationRefreshInterval
	liveReservationRefreshInterval = time.Millisecond
	t.Cleanup(func() { liveReservationRefreshInterval = oldInterval })

	cache := &liveReservationCacheForTest{
		acquireResult:   ConcurrencyAdmissionResult{Acquired: true},
		refreshResult:   true,
		refreshObserved: make(chan struct{}, 1),
	}
	svc := NewConcurrencyService(cache)
	live, requests := 4, 2
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-transfer-renewer",
	}
	_, reservation, err := svc.AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	stop := reservation.StartRefresh(context.Background())
	t.Cleanup(stop)
	select {
	case <-cache.refreshObserved:
	case <-time.After(time.Second):
		t.Fatal("creation renewer did not run")
	}

	record := &LiveCallRecord{CallID: "transferred", APIKeyID: 33}
	require.NoError(t, reservation.TransferToCall(record))
	select {
	case <-reservation.refreshDone:
	default:
		t.Fatal("creation renewer remained active after ownership transfer")
	}
	refreshCalls := cache.refreshCalls.Load()
	time.Sleep(5 * time.Millisecond)
	require.Equal(t, refreshCalls, cache.refreshCalls.Load(), "transfer must leave renewal to the record observer")
	refreshed, err := reservation.Refresh(context.Background())
	require.NoError(t, err)
	require.True(t, refreshed, "record-owned reservation must remain refreshable after transfer")
}

type blockingLiveReservationRefreshCache struct {
	liveReservationCacheForTest
	refreshStarted chan struct{}
	releaseRefresh chan struct{}
}

func (c *blockingLiveReservationRefreshCache) RefreshConcurrency(ctx context.Context, request ConcurrencyAdmissionRequest) (bool, error) {
	c.refreshCalls.Add(1)
	select {
	case <-c.refreshStarted:
	default:
		close(c.refreshStarted)
	}
	select {
	case <-c.releaseRefresh:
		return false, errors.New("refresh lost ownership")
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func TestLiveReservation_TransferRechecksRefreshLossBeforeMutatingRecord(t *testing.T) {
	oldInterval := liveReservationRefreshInterval
	liveReservationRefreshInterval = time.Millisecond
	t.Cleanup(func() { liveReservationRefreshInterval = oldInterval })

	live, requests := 4, 2
	cache := &blockingLiveReservationRefreshCache{
		liveReservationCacheForTest: liveReservationCacheForTest{
			acquireResult: ConcurrencyAdmissionResult{Acquired: true},
		},
		refreshStarted: make(chan struct{}),
		releaseRefresh: make(chan struct{}),
	}
	request := ConcurrencyAdmissionRequest{
		AccountID: 11, AccountPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, AccountGeneration: 1,
		UserID: 22, UserPolicy: ConcurrencyPolicy{Live: &live, Request: &requests}, UserGeneration: 1,
		APIKeyID: 33, Class: ConcurrencyClassLive, ReservationID: "live-transfer-refresh-loss",
	}
	_, reservation, err := NewConcurrencyService(cache).AcquireLiveReservation(context.Background(), request)
	require.NoError(t, err)
	reservation.StartRefresh(context.Background())

	select {
	case <-cache.refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("refresh did not enter the deterministic blocking point")
	}

	record := &LiveCallRecord{
		CallID:    "preexisting-record",
		AccountID: request.AccountID,
		UserID:    request.UserID,
		APIKeyID:  request.APIKeyID,
		LeaseID:   request.ReservationID,
		Model:     "preserved-marker",
	}
	transferDone := make(chan error, 1)
	go func() { transferDone <- reservation.TransferToCall(record) }()

	select {
	case <-transferDone:
		t.Fatal("transfer returned before the in-flight refresh completed")
	case <-time.After(10 * time.Millisecond):
	}
	close(cache.releaseRefresh)

	require.ErrorIs(t, <-transferDone, ErrLiveReservationLost)
	require.Equal(t, "preexisting-record", record.CallID)
	require.Equal(t, request.AccountID, record.AccountID)
	require.Equal(t, request.UserID, record.UserID)
	require.Equal(t, request.APIKeyID, record.APIKeyID)
	require.Equal(t, request.ReservationID, record.LeaseID)
	require.Equal(t, "preserved-marker", record.Model)
	require.Nil(t, record.reservation)
	require.False(t, reservation.transferred)
	require.NoError(t, reservation.Release(context.Background()))
	require.Equal(t, int64(1), cache.releaseCalls.Load(), "failed transfer must still be releasable exactly once")
}
