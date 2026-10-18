package service

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
	"github.com/stretchr/testify/require"
)

func TestLiveRemoteAttestationCreationAndSideband(t *testing.T) {
	const token = "local-test-only-0123456789abcdef0123456789"
	handler, err := liveattestation.NewHelperHandler(liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.remote-proof"}`}, token)
	require.NoError(t, err)
	helper := httptest.NewServer(handler)
	defer helper.Close()
	file := filepath.Join(t.TempDir(), "helper-token")
	require.NoError(t, os.WriteFile(file, []byte(token), 0600))
	t.Setenv("LIVE_ATTESTATION_URL", helper.URL)
	t.Setenv("LIVE_ATTESTATION_TOKEN_FILE", file)
	verifyRemoteLiveAttestation(t)
}

// This opt-in test uses real Mac DeviceCheck but a captured upstream transport;
// it never contacts OpenAI or reads real account credentials.
func TestLiveRealRemoteAttestationOnLinux(t *testing.T) {
	if os.Getenv("INFERNO_TEST_REAL_HELPER") != "1" {
		t.Skip("requires an explicitly configured local Mac helper")
	}
	require.Equal(t, "linux", runtime.GOOS)
	verifyRemoteLiveAttestation(t)
}

func verifyRemoteLiveAttestation(t *testing.T) {
	t.Helper()
	provider := liveattestation.NewProvider()
	require.NoError(t, provider.Check(context.Background()))
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "local-integration-test-only"}}
	upstream := &liveHTTPUpstreamStub{}
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-access-token", "chatgpt_account_id": "acct_test"}}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, liveAttestation: provider,
		liveAttestationCipher: newLiveAttestationCipher(cfg), accountRepo: &liveTestAccountRepo{account: account}}
	header, encrypted, err := svc.prepareLiveAttestation(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, header, encrypted)
	_, err = svc.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{SDP: "v=offer\r\n", Session: []byte(`{"model":"gpt-live-test"}`)}, header)
	require.NoError(t, err)
	require.True(t, upstream.request.Header.Get(liveAttestationHeader) == header, "creation must forward the generated proof")
	// Sideband must reuse the encrypted creation proof without generating again.
	svc.liveAttestation = liveAttestationStub{}
	headers, err := svc.liveSidebandHeaders(context.Background(), account, &LiveCallRecord{AttestationCiphertext: encrypted})
	require.NoError(t, err)
	require.True(t, headers.Get(liveAttestationHeader) == header, "sideband must reuse the creation proof")
}
