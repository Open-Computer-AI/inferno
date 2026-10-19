//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type admissionCacheForTest struct {
	stubConcurrencyCacheForTest
	result       ConcurrencyAdmissionResult
	acquireErr   error
	refreshValue bool
	refreshErr   error
	released     bool
}

func (c *admissionCacheForTest) AcquireConcurrency(context.Context, ConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error) {
	return c.result, c.acquireErr
}

func (c *admissionCacheForTest) RefreshConcurrency(context.Context, ConcurrencyAdmissionRequest) (bool, error) {
	return c.refreshValue, c.refreshErr
}

func (c *admissionCacheForTest) ReleaseConcurrency(context.Context, ConcurrencyAdmissionRequest) error {
	c.released = true
	return nil
}

var _ ConcurrencyCache = (*admissionCacheForTest)(nil)
var _ ConcurrencyAdmissionCache = (*admissionCacheForTest)(nil)

func TestConcurrencyServicePolicyAdmissionDelegatesToOptionalCache(t *testing.T) {
	cache := &admissionCacheForTest{result: ConcurrencyAdmissionResult{Acquired: true}}
	service := NewConcurrencyService(cache)
	request := ConcurrencyAdmissionRequest{
		AccountID:         1,
		AccountPolicy:     ConcurrencyPolicy{Shared: 2},
		AccountGeneration: 1,
		UserID:            2,
		UserPolicy:        ConcurrencyPolicy{Shared: 2},
		UserGeneration:    1,
		APIKeyID:          3,
		Class:             ConcurrencyClassRequest,
		ReservationID:     "request-1",
	}

	result, err := service.AcquireConcurrency(context.Background(), request)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.NoError(t, service.ReleaseConcurrency(context.Background(), request))
	require.True(t, cache.released)
}

func TestConcurrencyServicePolicyAdmissionFailsClosedWithoutOptionalCache(t *testing.T) {
	service := NewConcurrencyService(&stubConcurrencyCacheForTest{})
	request := ConcurrencyAdmissionRequest{
		AccountID:         1,
		AccountPolicy:     ConcurrencyPolicy{Shared: 2},
		AccountGeneration: 1,
		UserID:            2,
		UserPolicy:        ConcurrencyPolicy{Shared: 2},
		UserGeneration:    1,
		APIKeyID:          3,
		Class:             ConcurrencyClassRequest,
		ReservationID:     "request-1",
	}

	_, err := service.AcquireConcurrency(context.Background(), request)
	require.ErrorIs(t, err, ErrConcurrencyPolicyAdmissionUnavailable)
}
