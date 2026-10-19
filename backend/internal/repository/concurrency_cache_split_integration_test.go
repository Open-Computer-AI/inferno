//go:build integration

package repository

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type splitConcurrencyRedisSuite struct {
	IntegrationRedisSuite
	cache *concurrencyCache
}

func TestSplitConcurrencyRedisSuite(t *testing.T) {
	suite.Run(t, new(splitConcurrencyRedisSuite))
}

func (s *splitConcurrencyRedisSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.cache = NewConcurrencyCache(s.rdb, 15, 900).(*concurrencyCache)
}

type splitAdmissionAttempt struct {
	request service.ConcurrencyAdmissionRequest
	result  service.ConcurrencyAdmissionResult
	err     error
}

func splitAdmissionPolicy() service.ConcurrencyPolicy {
	live, requests := 4, 2
	return service.ConcurrencyPolicy{Live: &live, Request: &requests}
}

func (s *splitConcurrencyRedisSuite) publishSplitPolicy(accountID, userID int64) service.ConcurrencyPolicy {
	s.T().Helper()
	policy := splitAdmissionPolicy()
	require.NoError(s.T(), s.cache.PublishAccountConcurrencyPolicy(s.ctx, accountID, policy, 1))
	require.NoError(s.T(), s.cache.PublishUserConcurrencyPolicy(s.ctx, userID, policy, 1))
	return policy
}

func (s *splitConcurrencyRedisSuite) acquireMany(
	accountID, userID int64,
	policy service.ConcurrencyPolicy,
	class service.ConcurrencyClass,
	prefix string,
	count int,
) []splitAdmissionAttempt {
	s.T().Helper()
	attempts := make(chan splitAdmissionAttempt, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := service.ConcurrencyAdmissionRequest{
				AccountID:         accountID,
				AccountPolicy:     policy,
				AccountGeneration: 1,
				UserID:            userID,
				UserPolicy:        policy,
				UserGeneration:    1,
				APIKeyID:          accountID*1000 + int64(i) + 1,
				Class:             class,
				ReservationID:     prefix + "-" + strconv.Itoa(i),
			}
			result, err := s.cache.AcquireConcurrency(s.ctx, request)
			attempts <- splitAdmissionAttempt{request: request, result: result, err: err}
		}()
	}
	wg.Wait()
	close(attempts)

	result := make([]splitAdmissionAttempt, 0, count)
	for attempt := range attempts {
		result = append(result, attempt)
	}
	return result
}

func (s *splitConcurrencyRedisSuite) releaseAttempts(attempts ...[]splitAdmissionAttempt) {
	s.T().Helper()
	for _, group := range attempts {
		for _, attempt := range group {
			if !attempt.result.Acquired {
				continue
			}
			require.NoError(s.T(), s.cache.ReleaseConcurrency(s.ctx, attempt.request))
			require.NoError(s.T(), s.cache.ReleaseConcurrency(s.ctx, attempt.request), "release must be idempotent")
		}
	}
}

func (s *splitConcurrencyRedisSuite) assertParallelCaps(
	accountID, userID int64,
	first service.ConcurrencyClass,
) {
	s.T().Helper()
	policy := s.publishSplitPolicy(accountID, userID)
	firstAttempts := s.acquireMany(accountID, userID, policy, first, "first", 32)
	second := service.ConcurrencyClassRequest
	if first == service.ConcurrencyClassRequest {
		second = service.ConcurrencyClassLive
	}
	secondAttempts := s.acquireMany(accountID, userID, policy, second, "second", 32)

	firstAcquired, secondAcquired := 0, 0
	for _, attempt := range firstAttempts {
		require.NoError(s.T(), attempt.err)
		if attempt.result.Acquired {
			firstAcquired++
		}
	}
	for _, attempt := range secondAttempts {
		require.NoError(s.T(), attempt.err)
		if attempt.result.Acquired {
			secondAcquired++
		}
	}
	if first == service.ConcurrencyClassRequest {
		require.Equal(s.T(), 2, firstAcquired, "request cap must be independent of Live occupancy")
		require.Equal(s.T(), 4, secondAcquired, "Live cap must be independent of request occupancy")
	} else {
		require.Equal(s.T(), 4, firstAcquired, "Live cap must be independent of request occupancy")
		require.Equal(s.T(), 2, secondAcquired, "request cap must be independent of Live occupancy")
	}

	require.Equal(s.T(), int64(2), s.rdb.ZCard(s.ctx, accountSlotKey(accountID)).Val())
	require.Equal(s.T(), int64(4), s.rdb.ZCard(s.ctx, liveAccountSlotKey(accountID)).Val())
	require.Equal(s.T(), int64(2), s.rdb.ZCard(s.ctx, userSlotKey(userID)).Val())
	require.Equal(s.T(), int64(4), s.rdb.ZCard(s.ctx, liveUserSlotKey(userID)).Val())
	s.releaseAttempts(firstAttempts, secondAttempts)
	require.Eventually(s.T(), func() bool {
		return s.rdb.ZCard(s.ctx, accountSlotKey(accountID)).Val() == 0 &&
			s.rdb.ZCard(s.ctx, liveAccountSlotKey(accountID)).Val() == 0 &&
			s.rdb.ZCard(s.ctx, userSlotKey(userID)).Val() == 0 &&
			s.rdb.ZCard(s.ctx, liveUserSlotKey(userID)).Val() == 0
	}, time.Second, 10*time.Millisecond)
}

func (s *splitConcurrencyRedisSuite) TestSplitConcurrencyRedis_ParallelCaps_RequestFirst() {
	s.assertParallelCaps(7101, 7102, service.ConcurrencyClassRequest)
}

func (s *splitConcurrencyRedisSuite) TestSplitConcurrencyRedis_ParallelCaps_LiveFirst() {
	s.assertParallelCaps(7201, 7202, service.ConcurrencyClassLive)
}

func (s *splitConcurrencyRedisSuite) TestSplitConcurrencyRedis_LiveLifecycleRenewalTTLAndCancellation() {
	accountID, userID := int64(7301), int64(7302)
	policy := s.publishSplitPolicy(accountID, userID)
	request := service.ConcurrencyAdmissionRequest{
		AccountID:         accountID,
		AccountPolicy:     policy,
		AccountGeneration: 1,
		UserID:            userID,
		UserPolicy:        policy,
		UserGeneration:    1,
		APIKeyID:          7303,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "lifecycle-live",
	}

	acquired, err := s.cache.AcquireConcurrency(s.ctx, request)
	require.NoError(s.T(), err)
	require.True(s.T(), acquired.Acquired)
	liveTTL, err := s.rdb.TTL(s.ctx, liveAccountSlotKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), liveTTL, time.Duration(0))
	require.LessOrEqual(s.T(), liveTTL, time.Minute)

	for _, key := range []string{
		liveAccountSlotKey(accountID),
		liveUserSlotKey(userID),
		liveAPIKeySlotKey(request.APIKeyID),
	} {
		require.NoError(s.T(), s.rdb.Expire(s.ctx, key, time.Second).Err())
	}
	refreshed, err := s.cache.RefreshConcurrency(s.ctx, request)
	require.NoError(s.T(), err)
	require.True(s.T(), refreshed)
	renewedTTL, err := s.rdb.TTL(s.ctx, liveAccountSlotKey(accountID)).Result()
	require.NoError(s.T(), err)
	require.Greater(s.T(), renewedTTL, time.Second)

	canceledCtx, cancel := context.WithCancel(s.ctx)
	cancel()
	_, err = s.cache.AcquireConcurrency(canceledCtx, service.ConcurrencyAdmissionRequest{
		AccountID:         accountID,
		AccountPolicy:     policy,
		AccountGeneration: 1,
		UserID:            userID,
		UserPolicy:        policy,
		UserGeneration:    1,
		APIKeyID:          7304,
		Class:             service.ConcurrencyClassLive,
		ReservationID:     "canceled-live",
	})
	require.Error(s.T(), err, "a canceled admission must not create a reservation")

	require.NoError(s.T(), s.cache.ReleaseConcurrency(s.ctx, request))
	require.NoError(s.T(), s.cache.ReleaseConcurrency(s.ctx, request), "release must be idempotent")
	require.Equal(s.T(), int64(0), s.rdb.ZCard(s.ctx, liveAccountSlotKey(accountID)).Val())
	require.Equal(s.T(), int64(0), s.rdb.ZCard(s.ctx, liveUserSlotKey(userID)).Val())
	require.Equal(s.T(), int64(0), s.rdb.ZCard(s.ctx, liveAPIKeySlotKey(request.APIKeyID)).Val())
}
