//go:build darwin

package liveattestation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Concurrent callers share one process; answers come back in any order and are
// routed by ID, so every caller gets its own, distinct proof.
func TestHelperPipelinesConcurrentCallsToOneProcess(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	const callers = 3 // the reverse3 fake answers only once three requests wait
	headers := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			headers[i], errs[i] = helper.generate(ctx, node+"|reverse3", node, "reverse3", "com.openai.chat", []byte(`{}`))
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if seen[headers[i]] {
			t.Fatalf("two callers got the same proof %s", headers[i])
		}
		seen[headers[i]] = true
		if fakePid(t, headers[i]) != fakePid(t, headers[0]) {
			t.Fatal("concurrent callers were served by different processes")
		}
	}
}

func TestHelperServesABurstFromOneProcess(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	const burst = 10
	var wg sync.WaitGroup
	var failures atomic.Int32
	pids := sync.Map{}
	tokens := sync.Map{}
	started := time.Now()
	for range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
			defer cancel()
			header, err := helper.generate(ctx, node+"|slow", node, "slow", "com.openai.chat", []byte(`{}`))
			if err != nil {
				failures.Add(1)
				return
			}
			pids.Store(fakePid(t, header), true)
			if _, dup := tokens.LoadOrStore(header, true); dup {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("%d of %d burst requests failed or repeated a proof", failures.Load(), burst)
	}
	count := 0
	pids.Range(func(any, any) bool { count++; return true })
	if count != 1 {
		t.Fatalf("burst used %d processes, want 1", count)
	}
	t.Logf("burst of %d through one fake process (50 ms each, serial) took %s", burst, time.Since(started).Round(time.Millisecond))
}

// A crash fails the waiting callers at once (not at their deadline), and the
// next call starts a new process.
func TestHelperCrashFailsFastAndRestarts(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err := helper.generate(ctx, node+"|crash", node, "crash", "com.openai.chat", []byte(`{}`))
	cancel()
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a prompt exit error", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("crash took %s to surface", elapsed)
	}
	waitFor(t, func() bool { return helper.running() == 0 })
	if _, err := helper.generate(context.Background(), node+"|ok", node, "ok", "com.openai.chat", []byte(`{}`)); err != nil {
		t.Fatalf("helper did not restart after the crash: %v", err)
	}
}

func TestHelperRestartsAfterMalformedAnswer(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_, err := helper.generate(ctx, node+"|garbage", node, "garbage", "com.openai.chat", []byte(`{}`))
	cancel()
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a prompt failure", err)
	}
	waitFor(t, func() bool { return helper.running() == 0 })
}

func TestHelperRestartsAfterOversizedAnswer(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	_, err := helper.generate(ctx, node+"|huge", node, "huge", "com.openai.chat", []byte(`{}`))
	cancel()
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v after %s, want a prompt failure", err, time.Since(started))
	}
	waitFor(t, func() bool { return helper.running() == 0 })
}

// A helper that is answering, only slowly (a burst queued behind serial
// DeviceCheck), is not restarted by its callers' timeouts: a restart would
// fail every request still queued behind them.
func TestHelperBehindIsNotRestartedByBurstTimeouts(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	if _, err := helper.generate(context.Background(), node+"|slow300", node, "slow300", "com.openai.chat", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	pid := helper.running()
	const callers = 12 // 12 x 300 ms serial; a 1 s budget times out the later ones
	var wg sync.WaitGroup
	var ok, timeouts, other atomic.Int32
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err := helper.generate(ctx, node+"|slow300", node, "slow300", "com.openai.chat", []byte(`{}`))
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, context.DeadlineExceeded):
				timeouts.Add(1)
			default:
				other.Add(1)
			}
		}()
	}
	wg.Wait()
	if other.Load() != 0 || helper.running() != pid {
		t.Fatalf("ok=%d timeouts=%d other=%d; helper pid %d -> %d (restarted while answering)",
			ok.Load(), timeouts.Load(), other.Load(), pid, helper.running())
	}
	if ok.Load() < 2 || timeouts.Load() < 3 {
		t.Fatalf("test did not exercise the burst: ok=%d timeouts=%d", ok.Load(), timeouts.Load())
	}
}

func TestHelperRestartsWhenRuntimeChanges(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	first, err := helper.generate(context.Background(), "identity-a", node, "ok", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := helper.generate(context.Background(), "identity-b", node, "ok", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if fakePid(t, first) == fakePid(t, second) {
		t.Fatal("a ChatGPT app update did not restart the helper")
	}
}

// fakeAppNode plays the ChatGPT app's Node in both roles: the one-shot script
// (it has SUB2API_ATTESTATION_BUNDLE_ID) and the long-lived helper. The mode is
// the content of the fake devicecheck.node file.
const fakeAppNode = `#!/bin/sh
mode=$(cat "$SUB2API_DEVICECHECK_MODULE")
if [ -n "$SUB2API_ATTESTATION_BUNDLE_ID" ]; then
  case "$mode" in *oneshot-slow*) sleep 0.3 ;; esac
  printf '{"v":1,"s":0,"t":"v1.oneshot-%s"}' "$$"
  exit 0
fi
case "$mode" in *helper-broken*) exit 4 ;; esac
n=0
while IFS= read -r line; do
  n=$((n+1))
  id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
  case "$line" in
    *'"ping":true'*) printf '{"id":%s,"pong":true}\n' "$id" ;;
    *) printf '{"id":%s,"header":"{\\"v\\":1,\\"s\\":0,\\"t\\":\\"v1.fake-%s-%s\\"}"}\n' "$id" "$$" "$n" ;;
  esac
done
`

// newFakeAppProvider builds a provider over a fake ChatGPT.app whose Node is
// fakeAppNode in the given mode; signals are fixed (no osascript).
func newFakeAppProvider(t *testing.T, mode string) *darwinProvider {
	t.Helper()
	app := filepath.Join(t.TempDir(), "ChatGPT.app")
	resources := filepath.Join(app, "Contents", "Resources")
	for _, dir := range []string{filepath.Join(resources, "cua_node", "bin"), filepath.Join(resources, "native")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(resources, "cua_node", "bin", "node"):    fakeAppNode,
		filepath.Join(resources, "native", "devicecheck.node"): mode,
		filepath.Join(app, "Contents", "Info.plist"): `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` +
			`<key>CFBundleIdentifier</key><string>com.openai.test</string></dict></plist>`,
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// The fake Node is /bin/sh and therefore arch independent: pin arm64 so
	// the helper and fallback paths run on any build machine, whatever arch
	// the host happens to be.
	provider := &darwinProvider{
		appSessionID: "test-session",
		appPaths:     []string{app},
		helper:       newDeviceCheckHelper(),
		fallbacks:    make(chan struct{}, maxOneShotFallbacks),
		goarch:       "arm64",
	}
	provider.signalReader = func(context.Context) (deviceSignals, error) {
		return deviceSignals{SchemaVersion: 1, Locale: "en_IN", PreferredLanguages: []string{"en-IN"}, Timezone: "Asia/Kolkata", ScreenScale: 2, AppSessionID: "test-session"}, nil
	}
	t.Cleanup(provider.helper.stop)
	return provider
}

// The production guard rejects unsupported architectures by design; pin that
// contract on every build machine. resolveRuntime fails before the app paths,
// helper, or Node process are touched, so the fake app is never used.
func TestProviderGenerateRejectedOnUnsupportedArch(t *testing.T) {
	provider := newFakeAppProvider(t, "ok")
	provider.goarch = "amd64"
	if _, err := provider.Generate(context.Background()); err == nil || !strings.Contains(err.Error(), "requires Apple Silicon") {
		t.Fatalf("expected the Apple Silicon guard error, got %v", err)
	}
}

func TestProviderConcurrentGenerateUsesTheWarmHelper(t *testing.T) {
	provider := newFakeAppProvider(t, "ok")
	const burst = 10
	var wg sync.WaitGroup
	headers := make([]string, burst)
	errs := make([]error, burst)
	for i := range burst {
		wg.Add(1)
		go func() { defer wg.Done(); headers[i], errs[i] = provider.Generate(context.Background()) }()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range burst {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if !strings.Contains(headers[i], "v1.fake-") {
			t.Fatalf("request %d used the one-shot path: %s", i, headers[i])
		}
		if seen[headers[i]] {
			t.Fatal("a proof was handed out twice")
		}
		seen[headers[i]] = true
	}
}

func TestProviderFallsBackToOneShotWhenHelperFails(t *testing.T) {
	provider := newFakeAppProvider(t, "helper-broken")
	header, err := provider.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(header, "v1.oneshot-") {
		t.Fatalf("expected the one-shot path, got %s", header)
	}
}

// When the helper is down a burst must not start a Node process per request:
// at most maxOneShotFallbacks run at once, and every request still succeeds.
func TestProviderCapsOneShotFallbacks(t *testing.T) {
	provider := newFakeAppProvider(t, "helper-broken oneshot-slow")
	const burst = 6
	var wg sync.WaitGroup
	var failures atomic.Int32
	started := time.Now()
	for range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := provider.Generate(context.Background()); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(started)
	if failures.Load() != 0 {
		t.Fatalf("%d fallbacks failed", failures.Load())
	}
	// 6 one-shots of 0.3 s, 2 at a time, take at least 3 rounds.
	if min := 3 * 300 * time.Millisecond; elapsed < min {
		t.Fatalf("burst of %d one-shots took %s; more than %d ran at once", burst, elapsed, maxOneShotFallbacks)
	}
}

func TestSignalsKeepLastGoodWhenRefreshFails(t *testing.T) {
	provider := &darwinProvider{helper: newDeviceCheckHelper()}
	fail := false
	provider.signalReader = func(context.Context) (deviceSignals, error) {
		if fail {
			return deviceSignals{}, errors.New("no screen")
		}
		return deviceSignals{SchemaVersion: 1, Locale: "en_IN"}, nil
	}
	ctx := context.Background()
	if _, err := provider.refreshSignals(ctx); err != nil {
		t.Fatal(err)
	}
	fail = true
	// Without Warm a failed read fails, exactly as in the original helper.
	if _, err := provider.refreshSignals(ctx); err == nil {
		t.Fatal("a cold provider served cached signals after a failed read")
	}
	provider.warming.Store(true)
	got, err := provider.refreshSignals(ctx)
	if err != nil || got.Locale != "en_IN" {
		t.Fatalf("refresh failure dropped good signals: %+v %v", got, err)
	}
	provider.signalsAt = time.Now().Add(-signalsStaleLimit - time.Second)
	if _, err := provider.refreshSignals(ctx); err == nil {
		t.Fatal("signals older than the stale limit were still served")
	}
}

// With Warm running, a request never re-reads signals on its own path while
// the cache is within the stale limit (the background loop refreshes them).
func TestWarmProviderServesCachedSignalsWithoutBlocking(t *testing.T) {
	reads := 0
	provider := &darwinProvider{helper: newDeviceCheckHelper()}
	provider.signalReader = func(context.Context) (deviceSignals, error) {
		reads++
		return deviceSignals{SchemaVersion: 1}, nil
	}
	if _, err := provider.cachedSignals(context.Background()); err != nil {
		t.Fatal(err)
	}
	provider.signalsAt = time.Now().Add(-signalsMaxAge - time.Minute)
	provider.warming.Store(true)
	if _, err := provider.cachedSignals(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("a warm provider read signals on the request path (reads=%d)", reads)
	}
}

// The HTTP layer end to end over the fake app: N in flight are served, the
// (N+1)th is refused 429 exactly as before, and auth is unchanged.
func TestHelperHandlerWithLimitServesNInParallel(t *testing.T) {
	release := make(chan struct{})
	var inFlight, peak atomic.Int32
	stub := helperStub{generate: func(ctx context.Context) (string, error) {
		now := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		<-release
		return testHeader, nil
	}}
	const limit = 3
	handler, err := NewHelperHandlerWithLimit(stub, testHelperToken, limit)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	remote, err := NewRemoteProvider(server.URL, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, limit)
	for range limit {
		go func() { _, err := remote.Generate(context.Background()); results <- err }()
	}
	waitFor(t, func() bool { return inFlight.Load() == limit })
	_, overflow := remote.Generate(context.Background())
	if overflow == nil || !strings.Contains(overflow.Error(), "429") {
		t.Fatalf("request beyond the limit: %v, want 429", overflow)
	}
	close(release)
	for range limit {
		if err := <-results; err != nil {
			t.Fatalf("in-limit request failed: %v", err)
		}
	}
	if peak.Load() != limit {
		t.Fatalf("peak in flight %d, want %d", peak.Load(), limit)
	}
	// Unauthenticated requests are still refused before any generation.
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/attestation", nil)
	request.Header.Set("Authorization", "Bearer wrong")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: HTTP %d", response.StatusCode)
	}
}

// A real proof is ~4 KB; the answer must carry a Content-Length (not chunked)
// and the same body bytes json.Encoder produced before.
func TestHelperAnswersWithFixedLength(t *testing.T) {
	long := `{"v":1,"s":0,"t":"v1.` + strings.Repeat("A", 5000) + `"}`
	handler, err := NewHelperHandlerWithLimit(helperStub{generate: func(context.Context) (string, error) { return long, nil }}, testHelperToken, 4)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/attestation", nil)
	request.Header.Set("Authorization", "Bearer "+testHelperToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	var want strings.Builder
	_ = json.NewEncoder(&want).Encode(remoteHelperResponse{Attestation: long})
	if len(response.TransferEncoding) != 0 || response.ContentLength != int64(len(body)) || string(body) != want.String() {
		t.Fatalf("transfer-encoding %v, content-length %d, body %d bytes (want %d, same bytes as json.Encoder)",
			response.TransferEncoding, response.ContentLength, len(body), want.Len())
	}
}

func TestHelperHandlerLimitBounds(t *testing.T) {
	for _, limit := range []int{0, -1, MaxHelperConcurrency + 1} {
		if _, err := NewHelperHandlerWithLimit(helperStub{}, testHelperToken, limit); err == nil {
			t.Fatalf("limit %d accepted", limit)
		}
	}
	for _, limit := range []int{1, 16, MaxHelperConcurrency} {
		if _, err := NewHelperHandlerWithLimit(helperStub{}, testHelperToken, limit); err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
}

// End to end over HTTP with the real provider code and the fake app: a burst
// of 10 through a limit-16 handler, every answer 200 and distinct.
func TestHelperHandlerBurstOverFakeApp(t *testing.T) {
	provider := newFakeAppProvider(t, "ok")
	handler, err := NewHelperHandlerWithLimit(provider, testHelperToken, 16)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	remote, err := NewRemoteProvider(server.URL, testHelperToken)
	if err != nil {
		t.Fatal(err)
	}
	const burst = 10
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[string]bool{}
	var problems []string
	for i := range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			header, err := remote.Generate(context.Background())
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				problems = append(problems, fmt.Sprintf("%d: %v", i, err))
			case seen[header]:
				problems = append(problems, fmt.Sprintf("%d: repeated proof", i))
			}
			seen[header] = true
		}()
	}
	wg.Wait()
	if len(problems) != 0 {
		t.Fatalf("burst problems: %v", problems)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in 3 s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A Node helper that only answers errors is restarted after helperMaxFailures
// in a row; a successful proof resets the count (a pong does not).
func TestHelperRestartsAfterRepeatedErrorAnswers(t *testing.T) {
	node := writeFakeNode(t)
	t.Run("errors only", func(t *testing.T) {
		helper := newDeviceCheckHelper()
		defer helper.stop()
		for attempt := 1; attempt <= helperMaxFailures; attempt++ {
			if _, err := helper.generate(context.Background(), node+"|error", node, "error", "com.openai.chat", []byte(`{}`)); err == nil {
				t.Fatalf("attempt %d: expected the error answer", attempt)
			}
			if attempt == helperMaxFailures {
				break
			}
			if helper.running() == 0 {
				t.Fatalf("attempt %d: restarted before %d errors", attempt, helperMaxFailures)
			}
			// A pong (no DeviceCheck involved) must not reset the count.
			if err := helper.ping(context.Background(), node+"|error", node, "error"); err != nil {
				t.Fatalf("ping: %v", err)
			}
		}
		if helper.running() != 0 {
			t.Fatalf("a helper answering only errors was not restarted after %d", helperMaxFailures)
		}
	})
	t.Run("a proof resets the count", func(t *testing.T) {
		helper := newDeviceCheckHelper()
		defer helper.stop()
		var pid int
		for request := 1; request <= 6; request++ { // error, error, proof, error, error, proof
			header, err := helper.generate(context.Background(), node+"|err2ok", node, "err2ok", "com.openai.chat", []byte(`{}`))
			if (err == nil) != (request%3 == 0) {
				t.Fatalf("request %d: header %q err %v", request, header, err)
			}
			if request == 1 {
				pid = helper.running()
			}
			if helper.running() != pid {
				t.Fatalf("request %d: helper restarted although proofs keep succeeding", request)
			}
		}
	})
}

// fakeDeviceCheckAddon stands in for devicecheck.node under the REAL helper
// script: each generateToken takes 300 ms and appends a line to <module>.calls.
const fakeDeviceCheckAddon = `const fs = require("node:fs");
exports.generateToken = () => new Promise((resolve) => setTimeout(() => {
  fs.appendFileSync(__filename + ".calls", "x\n");
  resolve({supported: true, tokenBase64: "dG9rZW4", latencyMs: 300});
}, 300));
`

func realNodeForTest(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{filepath.Join(chatGPTApplicationPath, "Contents", "Resources", "cua_node", "bin", "node")} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath("node"); err == nil {
		return path
	}
	t.Skip("no node binary to run the real helper script")
	return ""
}

// The real Node helper script skips work its caller abandoned: after a burst
// whose tail timed out, the next caller does not wait behind the abandoned
// requests, and no DeviceCheck token is generated for them.
func TestRealScriptSkipsExpiredRequests(t *testing.T) {
	node := realNodeForTest(t)
	module := filepath.Join(t.TempDir(), "devicecheck.js")
	if err := os.WriteFile(module, []byte(fakeDeviceCheckAddon), 0o644); err != nil {
		t.Fatal(err)
	}
	signals := []byte(`{"schemaVersion":1,"preferredLanguages":["en-IN"],"locale":"en_IN","timezone":"Asia/Kolkata","screenSizeSum":2000,"screenScale":2,"appSessionId":"s"}`)
	helper := newDeviceCheckHelper()
	defer helper.stop()
	identity := node + "|" + module
	if _, err := helper.generate(context.Background(), identity, node, module, "com.openai.test", signals); err != nil {
		t.Fatalf("warm-up through the real script: %v", err)
	}
	var ok, timeouts atomic.Int32
	var wg sync.WaitGroup
	for range 12 { // 12 x 300 ms serial; a 1 s budget leaves most of them abandoned
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := helper.generate(ctx, identity, node, module, "com.openai.test", signals); err == nil {
				ok.Add(1)
			} else {
				timeouts.Add(1)
			}
		}()
	}
	wg.Wait()
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := helper.generate(ctx, identity, node, module, "com.openai.test", signals); err != nil {
		t.Fatalf("next caller: %v", err)
	}
	waited := time.Since(started)
	data, _ := os.ReadFile(module + ".calls")
	calls := strings.Count(string(data), "\n")
	t.Logf("burst ok=%d timeouts=%d; next caller waited %s; generateToken calls %d", ok.Load(), timeouts.Load(), waited.Round(10*time.Millisecond), calls)
	if timeouts.Load() < 3 {
		t.Fatalf("test did not exercise abandoned requests (timeouts=%d)", timeouts.Load())
	}
	// At most one abandoned request can be mid-generation when the deadline
	// passes; everything else queued behind it is skipped.
	if waited > 1200*time.Millisecond {
		t.Fatalf("next caller waited %s behind abandoned work", waited)
	}
	if limit := int(ok.Load()) + 2 + 1; calls > limit {
		t.Fatalf("generateToken ran %d times for %d answered requests (+warm-up, next, one in flight): abandoned requests spent tokens", calls, ok.Load())
	}
}
