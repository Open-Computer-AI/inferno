package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// liveMergePinAccountRepo makes the service look like a managed gateway
// (accountRepo != nil) without touching any repository method.
type liveMergePinAccountRepo struct{ AccountRepository }

// TestLiveCreateFailoverKeepsStagePolicy pins stage's Live create failover
// policy after the port/upstream merge (integrate/stage-all-20261025).
// Upstream made shouldFailoverOpenAIUpstreamResponse account-aware so a
// model_not_found 400 rotates accounts on managed gateways. Live call creation
// must keep stage's behaviour: that 400 is returned, not failed over.
func TestLiveCreateFailoverKeepsStagePolicy(t *testing.T) {
	svc := &OpenAIGatewayService{accountRepo: liveMergePinAccountRepo{}}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := []byte(`{"error":{"code":"model_not_found","message":"The model does not exist"}}`)

	// The ordinary OpenAI path now fails over on this response (upstream).
	require.True(t, svc.shouldFailoverOpenAIUpstreamResponse(account, http.StatusBadRequest, "", body))

	// Live creation keeps stage's policy and does not.
	require.False(t, svc.shouldFailoverLiveCreateError(&UpstreamFailoverError{
		StatusCode:   http.StatusBadRequest,
		ResponseBody: body,
	}))
	// Stage's existing Live policy rows are unchanged.
	require.True(t, svc.shouldFailoverLiveCreateError(&UpstreamFailoverError{StatusCode: http.StatusForbidden}))
	require.True(t, svc.shouldFailoverLiveCreateError(&UpstreamFailoverError{StatusCode: http.StatusTooManyRequests}))
}
