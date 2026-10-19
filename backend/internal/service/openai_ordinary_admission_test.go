//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type schedulerPolicyAwareConcurrencyCache struct {
	schedulerTestConcurrencyCache
	policyCalls int
	legacyCalls int
}

func (c *schedulerPolicyAwareConcurrencyCache) AcquireAccountConcurrency(context.Context, AccountConcurrencyAdmissionRequest) (ConcurrencyAdmissionResult, error) {
	c.policyCalls++
	return ConcurrencyAdmissionResult{Acquired: true}, nil
}

func (c *schedulerPolicyAwareConcurrencyCache) ReleaseAccountConcurrency(context.Context, AccountConcurrencyAdmissionRequest) error {
	return nil
}

func (c *schedulerPolicyAwareConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	c.legacyCalls++
	return false, nil
}

func TestOpenAIGatewayService_OrdinaryAccountSlotUsesPolicyAwareAccountCache(t *testing.T) {
	cache := &schedulerPolicyAwareConcurrencyCache{}
	svc := &OpenAIGatewayService{concurrencyService: NewConcurrencyService(cache)}

	result, err := svc.tryAcquireAccountSlot(context.Background(), 901, 0)
	require.NoError(t, err)
	require.True(t, result.Acquired)
	require.Equal(t, 1, cache.policyCalls)
	require.Zero(t, cache.legacyCalls, "ordinary OpenAI scheduling must not bypass the account-only policy surface")
	result.ReleaseFunc()
}

func TestGatewayAccountAdmission_FailsClosedWithoutConcurrencyService(t *testing.T) {
	ctx := context.Background()

	generic, err := (&GatewayService{}).tryAcquireAccountSlot(ctx, 902, 1)
	require.ErrorIs(t, err, ErrConcurrencyPolicyAdmissionUnavailable)
	require.Nil(t, generic)

	openAI, err := (&OpenAIGatewayService{}).tryAcquireAccountSlot(ctx, 903, 1)
	require.ErrorIs(t, err, ErrConcurrencyPolicyAdmissionUnavailable)
	require.Nil(t, openAI)

	// Live selection deliberately skips the ordinary slot; the full Live
	// account/user/key admission runs later in the Live creation path.
	live, err := (&OpenAIGatewayService{}).tryAcquireAccountSlotForAdmission(ctx, 904, 1, true)
	require.NoError(t, err)
	require.True(t, live.Acquired)
}
