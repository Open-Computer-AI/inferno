//go:build darwin

package liveattestation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeNode speaks the helper protocol. The mode comes in through the module path
// (the helper passes it as SUB2API_DEVICECHECK_MODULE); headers carry the fake's
// pid and request count so a test can tell a reused process from a new one.
const fakeNode = `#!/bin/sh
mode="$SUB2API_DEVICECHECK_MODULE"
n=0
while IFS= read -r line; do
  n=$((n+1))
  id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
  [ "$mode" = hang ] && sleep 30
  [ "$mode" = slow-first ] && [ "$n" = 1 ] && sleep 0.6
  case "$line" in
    *'"ping":true'*) printf '{"id":%s,"pong":true}\n' "$id" ;;
    *) printf '{"id":%s,"header":"{\\"v\\":1,\\"s\\":0,\\"t\\":\\"v1.fake-%s-%s\\"}"}\n' "$id" "$$" "$n" ;;
  esac
  [ "$mode" = exit-after-one ] && exit 0
done
`

func writeFakeNode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(path, []byte(fakeNode), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeToken(t *testing.T, header string) string {
	t.Helper()
	if err := validateHeader(header); err != nil {
		t.Fatalf("header %q: %v", header, err)
	}
	var value struct {
		V int    `json:"v"`
		T string `json:"t"`
	}
	if err := json.Unmarshal([]byte(header), &value); err != nil || value.V != 1 {
		t.Fatalf("header %q does not decode: %v", header, err)
	}
	return value.T
}

// fakePid returns the "<pid>" part of "v1.fake-<pid>-<n>".
func fakePid(t *testing.T, header string) string {
	parts := strings.Split(fakeToken(t, header), "-")
	if len(parts) != 3 {
		t.Fatalf("unexpected fake token in %q", header)
	}
	return parts[1]
}

func TestHelperReusesOneProcess(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	ctx := context.Background()
	first, err := helper.generate(ctx, node+"|ok", node, "ok", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := helper.generate(ctx, node+"|ok", node, "ok", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if fakePid(t, first) != fakePid(t, second) {
		t.Fatalf("helper was restarted between calls: %s then %s", first, second)
	}
	if err := helper.ping(ctx, node+"|ok", node, "ok"); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestHelperRestartsAfterExit(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	ctx := context.Background()
	first, err := helper.generate(ctx, node+"|exit-after-one", node, "exit-after-one", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := helper.generate(ctx, node+"|exit-after-one", node, "exit-after-one", "com.openai.chat", []byte(`{}`)); err == nil {
		t.Fatal("expected an error from the exited helper")
	}
	third, err := helper.generate(ctx, node+"|exit-after-one", node, "exit-after-one", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatalf("helper did not restart: %v", err)
	}
	if fakePid(t, first) == fakePid(t, third) {
		t.Fatal("expected a new helper process after the exit")
	}
}

func TestHelperTimeoutKeepsHelperUntilRepeatedFailures(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	for attempt := 1; attempt <= helperMaxFailures; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		started := time.Now()
		_, err := helper.generate(ctx, node+"|hang", node, "hang", "com.openai.chat", []byte(`{}`))
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("attempt %d: error = %v, want deadline exceeded", attempt, err)
		}
		if elapsed := time.Since(started); elapsed > 2*time.Second {
			t.Fatalf("timeout took %s", elapsed)
		}
		if stopped := helper.cmd == nil; stopped != (attempt == helperMaxFailures) {
			t.Fatalf("attempt %d: helper stopped = %v", attempt, stopped)
		}
	}
	if _, err := helper.generate(context.Background(), node+"|ok", node, "ok", "com.openai.chat", []byte(`{}`)); err != nil {
		t.Fatalf("helper did not recover: %v", err)
	}
}

func TestHelperSkipsLateAnswerAfterTimeout(t *testing.T) {
	node, helper := writeFakeNode(t), newDeviceCheckHelper()
	defer helper.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, err := helper.generate(ctx, node+"|slow-first", node, "slow-first", "com.openai.chat", []byte(`{}`))
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	pid := helper.cmd.Process.Pid
	header, err := helper.generate(context.Background(), node+"|slow-first", node, "slow-first", "com.openai.chat", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	// Same process, and the answer is the second request's (the first one's late
	// answer was skipped by its ID).
	if helper.cmd == nil || helper.cmd.Process.Pid != pid || !strings.HasSuffix(fakeToken(t, header), "-2") {
		t.Fatalf("expected the second answer from the same helper, got %s", header)
	}
}

func TestHelperLockHonoursDeadline(t *testing.T) {
	helper := newDeviceCheckHelper()
	helper.lock <- struct{}{} // another request holds the helper
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := helper.generate(ctx, "x", "/nonexistent", "ok", "com.openai.chat", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded while waiting for the lock", err)
	}
}

func TestSignalsCachedUntilMaxAge(t *testing.T) {
	reads := 0
	provider := &darwinProvider{helper: newDeviceCheckHelper()}
	provider.signalReader = func(context.Context) (deviceSignals, error) {
		reads++
		return deviceSignals{SchemaVersion: 1, Locale: "en_IN"}, nil
	}
	ctx := context.Background()
	for range 3 {
		if _, err := provider.cachedSignals(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if reads != 1 {
		t.Fatalf("signals read %d times, want 1", reads)
	}
	provider.signalsAt = time.Now().Add(-signalsMaxAge - time.Second)
	if _, err := provider.cachedSignals(ctx); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Fatalf("stale signals were not read again (reads=%d)", reads)
	}
}

func TestBundleIdentifierCachedUntilPlistChanges(t *testing.T) {
	app := filepath.Join(t.TempDir(), "ChatGPT.app")
	plist := filepath.Join(app, "Contents", "Info.plist")
	if err := os.MkdirAll(filepath.Dir(plist), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(id string, mtime time.Time) {
		body := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string></dict></plist>`
		if err := os.WriteFile(plist, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(plist, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	provider := &darwinProvider{helper: newDeviceCheckHelper()}
	ctx := context.Background()
	stamp := time.Now().Add(-time.Hour).Truncate(time.Second)
	write("com.openai.aaaa", stamp)
	if id, err := provider.bundleIdentifier(ctx, app); err != nil || id != "com.openai.aaaa" {
		t.Fatalf("first read = %q, %v", id, err)
	}
	write("com.openai.bbbb", stamp) // same size and mtime: served from the cache
	if id, _ := provider.bundleIdentifier(ctx, app); id != "com.openai.aaaa" {
		t.Fatalf("expected the cached id, got %q", id)
	}
	write("com.openai.bbbb", stamp.Add(time.Minute)) // app updated
	if id, _ := provider.bundleIdentifier(ctx, app); id != "com.openai.bbbb" {
		t.Fatalf("expected the new id after the plist changed, got %q", id)
	}
}

// TestRealDeviceCheckHelperMatchesOneShot runs the installed ChatGPT app's
// DeviceCheck through both paths and checks they build the same header.
func TestRealDeviceCheckHelperMatchesOneShot(t *testing.T) {
	if testing.Short() {
		t.Skip("uses the installed ChatGPT app")
	}
	provider := NewProvider().(*darwinProvider)
	defer provider.helper.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	nodePath, modulePath, bundleID, err := provider.resolveRuntime(ctx)
	if err != nil {
		t.Skipf("ChatGPT app not usable here: %v", err)
	}
	signals, err := provider.cachedSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	signalsJSON, _ := json.Marshal(signals)
	fromHelper, err := provider.helper.generate(ctx, runtimeIdentity(nodePath, modulePath), nodePath, modulePath, bundleID, signalsJSON)
	if err != nil {
		t.Fatalf("helper: %v", err)
	}
	oneShot, err := generateOnce(ctx, nodePath, modulePath, bundleID, signalsJSON)
	if err != nil {
		t.Fatalf("one-shot: %v", err)
	}
	for name, header := range map[string]string{"helper": fromHelper, "one-shot": oneShot} {
		token := fakeToken(t, header)
		payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "v1."))
		if err != nil || !strings.HasPrefix(token, "v1.") {
			t.Fatalf("%s token does not decode: %v", name, err)
		}
		for _, want := range []string{"token", "bundle_id", bundleID, signals.Locale, signals.AppSessionID} {
			if !strings.Contains(string(payload), want) {
				t.Fatalf("%s token is missing %q", name, want)
			}
		}
	}
	if fromHelper == oneShot {
		t.Fatal("expected a fresh DeviceCheck token per request")
	}
}
