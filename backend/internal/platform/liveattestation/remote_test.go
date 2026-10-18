package liveattestation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testHelperToken = "local-test-only-0123456789abcdef0123456789"
const testHeader = `{"v":1,"s":0,"t":"v1.test-device-proof"}`

type helperStub struct {
	generate func(context.Context) (string, error)
	checkErr error
}

func (p helperStub) Check(context.Context) error { return p.checkErr }
func (p helperStub) Generate(ctx context.Context) (string, error) {
	if p.generate != nil {
		return p.generate(ctx)
	}
	return testHeader, nil
}

func TestRemoteProviderFactoryUsesConfiguredHelper(t *testing.T) {
	handler, err := NewHelperHandler(helperStub{}, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	file := filepath.Join(t.TempDir(), "helper-token")
	if err := os.WriteFile(file, []byte(testHelperToken), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LIVE_ATTESTATION_URL", server.URL)
	t.Setenv("LIVE_ATTESTATION_TOKEN_FILE", file)
	p := NewProvider()
	if err := p.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	header, err := p.Generate(context.Background())
	if err != nil || header != testHeader {
		t.Fatalf("remote generation failed: %v", err)
	}
}

func TestRemoteProviderConfigurationFailsClosed(t *testing.T) {
	t.Setenv("LIVE_ATTESTATION_URL", "http://example.com")
	t.Setenv("LIVE_ATTESTATION_TOKEN_FILE", "")
	if err := NewProvider().Check(context.Background()); err == nil {
		t.Fatal("invalid remote configuration fell back to local provider")
	}
	for _, endpoint := range []string{"http://example.com", "http://user:pass@localhost", "http://localhost?token=x", "http://localhost/#fragment", "http://localhost/path", "ftp://localhost", ""} {
		if _, err := NewRemoteProvider(endpoint, testHelperToken); err == nil {
			t.Errorf("accepted invalid endpoint %q", endpoint)
		}
	}
	if _, err := NewRemoteProvider("http://localhost:19085", "short"); err == nil {
		t.Fatal("accepted short token")
	}
}

func TestRemoteProviderRejectsBadResponsesAndDoesNotLeakThem(t *testing.T) {
	for name, body := range map[string]string{
		"not-json":       "private-upstream-response",
		"empty":          `{"attestation":""}`,
		"wrong-envelope": `{"attestation":"{\"v\":2,\"s\":0,\"t\":\"v1.invalid-proof\"}"}`,
		"wrong-token":    `{"attestation":"{\"v\":1,\"s\":0,\"t\":\"invalid-proof\"}"}`,
		"trailing-json":  `{"attestation":""} {}`,
		"oversized":      strings.Repeat("private-upstream-response", 2000),
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			p, err := NewRemoteProvider(server.URL, testHelperToken)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Generate(context.Background())
			if err == nil {
				t.Fatal("accepted malformed response")
			}
			if strings.Contains(err.Error(), "private-upstream-response") {
				t.Fatal("response leaked through error")
			}
		})
	}
}

func TestRemoteProviderDoesNotFollowRedirects(t *testing.T) {
	var requests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	p, err := NewRemoteProvider(source.URL, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Generate(context.Background()); err == nil {
		t.Fatal("accepted redirect")
	}
	if requests.Load() != 0 {
		t.Fatal("forwarded helper credential to redirect target")
	}
}

func TestRemoteProviderRespectsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	p, err := NewRemoteProvider(server.URL, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = p.Generate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error: %v", err)
	}
}

func TestHelperAuthenticationMethodsBodiesAndNoStore(t *testing.T) {
	var generated atomic.Int32
	handler, err := NewHelperHandler(helperStub{generate: func(context.Context) (string, error) {
		generated.Add(1)
		return testHeader, nil
	}}, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, token, body string
		status                    int
	}{
		{"POST", "/v1/attestation", "", "", 401},
		{"POST", "/v1/attestation", "wrong", "", 401},
		{"GET", "/health", testHelperToken, "", 200},
		{"GET", "/v1/attestation", testHelperToken, "", 405},
		{"POST", "/v1/attestation", testHelperToken, "{}", 400},
		{"POST", "/unknown", testHelperToken, "", 404},
		{"POST", "/v1/attestation", testHelperToken, "", 200},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: got %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	if generated.Load() != 1 {
		t.Fatal("non-generation requests invoked DeviceCheck")
	}
}

func TestHelperRejectsConcurrentGenerationAndRedactsErrors(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	handler, err := NewHelperHandler(helperStub{generate: func(context.Context) (string, error) {
		close(started)
		<-release
		return "", errors.New("private-provider-error")
	}}, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	p, err := NewRemoteProvider(server.URL, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := p.Generate(context.Background()); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("generation did not start")
	}
	_, secondErr := p.Generate(context.Background())
	close(release)
	firstErr := <-done
	if secondErr == nil || !strings.Contains(secondErr.Error(), "429") {
		t.Fatalf("concurrent call error: %v", secondErr)
	}
	if firstErr == nil || strings.Contains(firstErr.Error(), "private-provider-error") {
		t.Fatal("provider error was not redacted")
	}
}

func TestHelperTokenFileMustBePrivate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(testHelperToken), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHelperTokenFile(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHelperTokenFile(file); err == nil {
		t.Fatal("accepted world-readable token file")
	}
}
