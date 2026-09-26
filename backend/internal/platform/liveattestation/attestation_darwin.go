//go:build darwin

package liveattestation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	chatGPTApplicationPath = "/Applications/ChatGPT.app"
	attestationTimeout     = 5 * time.Second
	// A warm helper answers in tens of milliseconds. Past this budget the call
	// falls back to a one-shot process within attestationTimeout.
	helperTimeout = 2 * time.Second
	// Screen, locale and time zone change rarely. Warm refreshes them in the
	// background so a Live call does not wait on osascript; a cache older than
	// signalsMaxAge (no Warm, or osascript failing) is read again on the call.
	signalsRefreshInterval = 5 * time.Minute
	signalsMaxAge          = 15 * time.Minute
	// Health check for the idle helper; a dead helper is restarted before the
	// next call needs it.
	helperPingInterval = time.Minute
)

type darwinProvider struct {
	appSessionID string
	appPaths     []string
	signalReader func(context.Context) (deviceSignals, error)

	mu        sync.Mutex
	signals   deviceSignals
	signalsAt time.Time
	bundle    bundleEntry

	helper   *deviceCheckHelper
	warmOnce sync.Once
}

type bundleEntry struct {
	plist   string
	modTime time.Time
	size    int64
	id      string
}

type deviceSignals struct {
	SchemaVersion      int      `json:"schemaVersion"`
	PreferredLanguages []string `json:"preferredLanguages"`
	Locale             string   `json:"locale"`
	Timezone           string   `json:"timezone"`
	ScreenSizeSum      int      `json:"screenSizeSum"`
	ScreenScale        float64  `json:"screenScale"`
	AppSessionID       string   `json:"appSessionId"`
}

type macOSSignals struct {
	Locale    string   `json:"locale"`
	Languages []string `json:"languages"`
	Timezone  string   `json:"timezone"`
	Width     float64  `json:"width"`
	Height    float64  `json:"height"`
	Scale     float64  `json:"scale"`
}

func NewProvider() Provider {
	paths := []string{chatGPTApplicationPath}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		paths = append(paths, filepath.Join(home, "Applications", "ChatGPT.app"))
	}
	provider := &darwinProvider{
		appSessionID: uuid.NewString(),
		appPaths:     paths,
		helper:       newDeviceCheckHelper(),
	}
	provider.signalReader = provider.readSignals
	return provider
}

func (p *darwinProvider) Check(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, attestationTimeout)
	defer cancel()
	_, _, _, err := p.resolveRuntime(checkCtx)
	return err
}

// Warm prepares attestation before the first Live call and keeps it ready:
// it reads the device signals, starts the DeviceCheck helper, refreshes the
// signals every few minutes and health-checks the helper. Failures are left
// to Generate, which reports them on the call that needs attestation.
func (p *darwinProvider) Warm() {
	p.warmOnce.Do(func() { go p.keepWarm() })
}

func (p *darwinProvider) keepWarm() {
	p.warmStep(true)
	signalsTicker := time.NewTicker(signalsRefreshInterval)
	defer signalsTicker.Stop()
	pingTicker := time.NewTicker(helperPingInterval)
	defer pingTicker.Stop()
	for {
		select {
		case <-signalsTicker.C:
			p.warmStep(true)
		case <-pingTicker.C:
			p.warmStep(false)
		}
	}
}

func (p *darwinProvider) warmStep(refreshSignals bool) {
	ctx, cancel := context.WithTimeout(context.Background(), attestationTimeout)
	defer cancel()
	nodePath, modulePath, _, err := p.resolveRuntime(ctx)
	if err != nil {
		return
	}
	if refreshSignals {
		_, _ = p.refreshSignals(ctx)
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, helperTimeout)
	defer pingCancel()
	_ = p.helper.ping(pingCtx, nodePath, modulePath)
}

func (p *darwinProvider) resolveRuntime(ctx context.Context) (string, string, string, error) {
	if runtime.GOARCH != "arm64" {
		return "", "", "", errors.New("live attestation currently requires Apple Silicon; Intel macOS is not supported")
	}
	appPath, err := p.findApplication()
	if err != nil {
		return "", "", "", err
	}
	resourcesPath := filepath.Join(appPath, "Contents", "Resources")
	nodePath := filepath.Join(resourcesPath, "cua_node", "bin", "node")
	modulePath := filepath.Join(resourcesPath, "native", "devicecheck.node")
	for filePath, label := range map[string]string{
		nodePath:   "bundled Node.js runtime",
		modulePath: "DeviceCheck native module",
	} {
		if info, statErr := os.Stat(filePath); statErr != nil || info.IsDir() {
			return "", "", "", fmt.Errorf("%w: ChatGPT app is missing its %s", ErrChatGPTAppMissing, label)
		}
	}
	bundleID, err := p.bundleIdentifier(ctx, appPath)
	if err != nil {
		return "", "", "", err
	}
	return nodePath, modulePath, bundleID, nil
}

func (p *darwinProvider) Generate(ctx context.Context) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, attestationTimeout)
	defer cancel()
	nodePath, modulePath, bundleID, err := p.resolveRuntime(runCtx)
	if err != nil {
		return "", err
	}
	signals, err := p.cachedSignals(runCtx)
	if err != nil {
		return "", err
	}
	signalsJSON, err := json.Marshal(signals)
	if err != nil {
		return "", fmt.Errorf("encode Live attestation signals: %w", err)
	}

	helperCtx, helperCancel := context.WithTimeout(runCtx, helperTimeout)
	header, helperErr := p.helper.generate(helperCtx, nodePath, modulePath, bundleID, signalsJSON)
	helperCancel()
	if helperErr == nil && validateHeader(header) == nil {
		return header, nil
	}
	// The helper only saves the process start; any failure takes the original
	// one-shot path, which also reports the real reason.
	return generateOnce(runCtx, nodePath, modulePath, bundleID, signalsJSON)
}

func generateOnce(ctx context.Context, nodePath, modulePath, bundleID string, signalsJSON []byte) (string, error) {
	command := exec.CommandContext(ctx, nodePath, "-e", deviceCheckScript)
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"SUB2API_DEVICECHECK_MODULE=" + modulePath,
		"SUB2API_ATTESTATION_BUNDLE_ID=" + bundleID,
		"SUB2API_ATTESTATION_SIGNALS=" + string(signalsJSON),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("ChatGPT DeviceCheck token generation timed out")
		}
		reason := strings.TrimSpace(stderr.String())
		if len(reason) > 240 {
			reason = reason[:240]
		}
		if reason == "" {
			reason = err.Error()
		}
		return "", fmt.Errorf("ChatGPT DeviceCheck token generation failed: %s", reason)
	}
	header := strings.TrimSpace(stdout.String())
	if err := validateHeader(header); err != nil {
		return "", err
	}
	return header, nil
}

func validateHeader(header string) error {
	if len(header) < 20 || len(header) > 16*1024 || !json.Valid([]byte(header)) {
		return errors.New("ChatGPT DeviceCheck returned a malformed attestation")
	}
	return nil
}

func (p *darwinProvider) findApplication() (string, error) {
	for _, appPath := range p.appPaths {
		info, err := os.Stat(appPath)
		if err == nil && info.IsDir() {
			return appPath, nil
		}
	}
	return "", ErrChatGPTAppMissing
}

// bundleIdentifier re-reads Info.plist only when it changes (an app update).
func (p *darwinProvider) bundleIdentifier(ctx context.Context, appPath string) (string, error) {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	info, err := os.Stat(infoPlist)
	if err != nil {
		return "", fmt.Errorf("%w: cannot read its bundle identifier", ErrChatGPTAppMissing)
	}
	p.mu.Lock()
	cached := p.bundle
	p.mu.Unlock()
	if cached.id != "" && cached.plist == infoPlist && cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		return cached.id, nil
	}
	bundleID, err := readBundleIdentifier(ctx, appPath)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	p.bundle = bundleEntry{plist: infoPlist, modTime: info.ModTime(), size: info.Size(), id: bundleID}
	p.mu.Unlock()
	return bundleID, nil
}

func readBundleIdentifier(ctx context.Context, appPath string) (string, error) {
	infoPlist := filepath.Join(appPath, "Contents", "Info.plist")
	output, err := exec.CommandContext(
		ctx,
		"/usr/bin/plutil",
		"-extract",
		"CFBundleIdentifier",
		"raw",
		infoPlist,
	).Output()
	if err != nil {
		return "", fmt.Errorf("%w: cannot read its bundle identifier", ErrChatGPTAppMissing)
	}
	bundleID := strings.TrimSpace(string(output))
	if !strings.HasPrefix(bundleID, "com.openai.") {
		return "", errors.New("the installed ChatGPT app has an unexpected bundle identifier")
	}
	return bundleID, nil
}

func (p *darwinProvider) cachedSignals(ctx context.Context) (deviceSignals, error) {
	p.mu.Lock()
	if !p.signalsAt.IsZero() && time.Since(p.signalsAt) < signalsMaxAge {
		signals := p.signals
		p.mu.Unlock()
		return signals, nil
	}
	p.mu.Unlock()
	return p.refreshSignals(ctx)
}

func (p *darwinProvider) refreshSignals(ctx context.Context) (deviceSignals, error) {
	signals, err := p.signalReader(ctx)
	if err != nil {
		return deviceSignals{}, err
	}
	p.mu.Lock()
	p.signals, p.signalsAt = signals, time.Now()
	p.mu.Unlock()
	return signals, nil
}

func (p *darwinProvider) readSignals(ctx context.Context) (deviceSignals, error) {
	const script = `ObjC.import("Foundation"); ObjC.import("AppKit");
const screen = $.NSScreen.mainScreen;
const frame = screen.frame;
JSON.stringify({
  locale: ObjC.unwrap($.NSLocale.currentLocale.localeIdentifier),
  languages: ObjC.deepUnwrap($.NSLocale.preferredLanguages),
  timezone: ObjC.unwrap($.NSTimeZone.localTimeZone.name),
  width: Number(frame.size.width),
  height: Number(frame.size.height),
  scale: Number(screen.backingScaleFactor)
})`
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", script).Output()
	if err != nil {
		return deviceSignals{}, fmt.Errorf("read macOS signals for Live attestation: %w", err)
	}
	var values macOSSignals
	if err := json.Unmarshal(output, &values); err != nil {
		return deviceSignals{}, fmt.Errorf("decode macOS signals for Live attestation: %w", err)
	}
	locale := truncateSignal(values.Locale, 64, "unknown")
	languages := values.Languages
	if len(languages) == 0 {
		languages = []string{locale}
	}
	if len(languages) > 16 {
		languages = languages[:16]
	}
	for index := range languages {
		languages[index] = truncateSignal(languages[index], 64, locale)
	}
	scale := values.Scale
	if scale <= 0 {
		scale = 1
	}
	return deviceSignals{
		SchemaVersion:      1,
		PreferredLanguages: languages,
		Locale:             locale,
		Timezone:           truncateSignal(values.Timezone, 64, "unknown"),
		ScreenSizeSum:      max(0, int(values.Width+values.Height+0.5)),
		ScreenScale:        scale,
		AppSessionID:       truncateSignal(p.appSessionID, 128, uuid.NewString()),
	}, nil
}

func truncateSignal(value string, limit int, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// deviceCheckHelper keeps one ChatGPT-bundled Node process with the DeviceCheck
// module loaded, so a Live call does not start a process (slow after idle on a
// memory-pressed Mac). Requests are serialized and every one still asks
// DeviceCheck for a fresh token.
type deviceCheckHelper struct {
	// lock is a one-slot semaphore instead of a mutex so a caller gives up
	// when its deadline passes rather than queueing behind a stuck request.
	lock       chan struct{}
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	lines      <-chan string
	done       chan struct{}
	nodePath   string
	modulePath string
	seq        uint64
}

func newDeviceCheckHelper() *deviceCheckHelper {
	return &deviceCheckHelper{lock: make(chan struct{}, 1)}
}

type helperRequest struct {
	ID       uint64          `json:"id"`
	Ping     bool            `json:"ping,omitempty"`
	BundleID string          `json:"bundleId,omitempty"`
	Signals  json.RawMessage `json:"signals,omitempty"`
}

type helperResponse struct {
	ID     uint64 `json:"id"`
	Header string `json:"header,omitempty"`
	Error  string `json:"error,omitempty"`
	Pong   bool   `json:"pong,omitempty"`
}

func (h *deviceCheckHelper) generate(ctx context.Context, nodePath, modulePath, bundleID string, signalsJSON []byte) (string, error) {
	response, err := h.call(ctx, nodePath, modulePath, helperRequest{BundleID: bundleID, Signals: signalsJSON})
	if err != nil {
		return "", err
	}
	if response.Error != "" {
		return "", fmt.Errorf("attestation helper: %s", response.Error)
	}
	return response.Header, nil
}

func (h *deviceCheckHelper) ping(ctx context.Context, nodePath, modulePath string) error {
	response, err := h.call(ctx, nodePath, modulePath, helperRequest{Ping: true})
	if err == nil && !response.Pong {
		err = errors.New("attestation helper did not answer the health check")
	}
	return err
}

func (h *deviceCheckHelper) call(ctx context.Context, nodePath, modulePath string, request helperRequest) (helperResponse, error) {
	select {
	case h.lock <- struct{}{}:
		defer func() { <-h.lock }()
	case <-ctx.Done():
		return helperResponse{}, ctx.Err()
	}
	if h.cmd == nil || h.nodePath != nodePath || h.modulePath != modulePath {
		h.stopLocked()
		if err := h.startLocked(nodePath, modulePath); err != nil {
			return helperResponse{}, err
		}
	}
	h.seq++
	request.ID = h.seq
	payload, err := json.Marshal(request)
	if err != nil {
		return helperResponse{}, err
	}
	if _, err := h.stdin.Write(append(payload, '\n')); err != nil {
		h.stopLocked()
		return helperResponse{}, fmt.Errorf("attestation helper write: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			// Stop it so a late answer is never read as the next request's.
			h.stopLocked()
			return helperResponse{}, ctx.Err()
		case line, ok := <-h.lines:
			if !ok {
				h.stopLocked()
				return helperResponse{}, errors.New("attestation helper exited")
			}
			var response helperResponse
			if err := json.Unmarshal([]byte(line), &response); err != nil {
				h.stopLocked()
				return helperResponse{}, errors.New("attestation helper sent a malformed answer")
			}
			if response.ID != request.ID {
				continue
			}
			return response, nil
		}
	}
}

func (h *deviceCheckHelper) startLocked(nodePath, modulePath string) error {
	command := exec.Command(nodePath, "-e", deviceCheckHelperScript)
	command.Env = []string{
		"PATH=/usr/bin:/bin",
		"SUB2API_DEVICECHECK_MODULE=" + modulePath,
	}
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("start attestation helper: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("start attestation helper: %w", err)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start attestation helper: %w", err)
	}
	lines := make(chan string, 4)
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-done:
			}
		}
		close(lines)
		_ = command.Wait()
	}()
	h.cmd, h.stdin, h.lines, h.done = command, stdin, lines, done
	h.nodePath, h.modulePath = nodePath, modulePath
	return nil
}

// stop ends the helper process; the gateway keeps it for its lifetime.
func (h *deviceCheckHelper) stop() {
	h.lock <- struct{}{}
	defer func() { <-h.lock }()
	h.stopLocked()
}

func (h *deviceCheckHelper) stopLocked() {
	if h.cmd == nil {
		return
	}
	close(h.done)
	_ = h.stdin.Close()
	if h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
	}
	h.cmd, h.stdin, h.lines, h.done = nil, nil, nil, nil
}

// deviceCheckEncoder is shared by the one-shot script and the helper so both
// produce the same header.
const deviceCheckEncoder = `
const addon = require(process.env.SUB2API_DEVICECHECK_MODULE);

function head(major, value) {
  if (value < 24) return Buffer.from([major + value]);
  if (value <= 255) return Buffer.from([major + 24, value]);
  if (value <= 65535) {
    const out = Buffer.allocUnsafe(3);
    out[0] = major + 25;
    out.writeUInt16BE(value, 1);
    return out;
  }
  const out = Buffer.allocUnsafe(5);
  out[0] = major + 26;
  out.writeUInt32BE(value, 1);
  return out;
}
function uint(value) { return head(0, value); }
function text(value) {
  const body = Buffer.from(value, "utf8");
  return Buffer.concat([head(96, body.length), body]);
}
function float(value) {
  if (Number.isSafeInteger(value) && value >= 0) return uint(value);
  const out = Buffer.allocUnsafe(9);
  out[0] = 251;
  out.writeDoubleBE(value, 1);
  return out;
}
function array(values) { return Buffer.concat([head(128, values.length), ...values]); }
function map(entries) {
  return Buffer.concat([head(160, entries.length), ...entries.flatMap(([key, value]) => [uint(key), value])]);
}
function field(key, value) { return Buffer.concat([text(key), text(value)]); }
function base64url(value) {
  return value.toString("base64").replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/u, "");
}

async function attest(signals, bundleID) {
  const result = await addon.generateToken();
  if (!result || !result.supported) throw new Error("DeviceCheck is not supported on this Mac");
  if (!result.tokenBase64) throw new Error("DeviceCheck returned no token");
  const fingerprint = map([
    [0, uint(signals.schemaVersion)],
    [1, array(signals.preferredLanguages.map(text))],
    [2, text(signals.locale)],
    [3, text(signals.timezone)],
    [4, uint(signals.screenSizeSum)],
    [5, float(signals.screenScale)],
    [6, text(signals.appSessionId)]
  ]);
  const fields = [
    field("token", result.tokenBase64),
    field("bundle_id", bundleID),
    Buffer.concat([text("f"), head(64, fingerprint.length), fingerprint])
  ];
  if (result.latencyMs != null) {
    fields.push(Buffer.concat([text("t"), float(result.latencyMs)]));
  }
  const token = "v1." + base64url(Buffer.concat([Buffer.from([160 + fields.length]), ...fields]));
  return JSON.stringify({v: 1, s: 0, t: token});
}
`

const deviceCheckScript = deviceCheckEncoder + `
(async () => {
  const signals = JSON.parse(process.env.SUB2API_ATTESTATION_SIGNALS);
  process.stdout.write(await attest(signals, process.env.SUB2API_ATTESTATION_BUNDLE_ID));
})().catch((error) => {
  process.stderr.write(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
});`

// deviceCheckHelperScript answers one JSON line per request line:
// {"id","bundleId","signals"} -> {"id","header"} or {"id","error"};
// {"id","ping":true} -> {"id","pong":true}. It exits when stdin closes.
const deviceCheckHelperScript = deviceCheckEncoder + `
const readline = require("node:readline");
const reply = (message) => process.stdout.write(JSON.stringify(message) + "\n");
let queue = Promise.resolve();
readline.createInterface({input: process.stdin})
  .on("line", (line) => {
    queue = queue.then(async () => {
      let id = 0;
      try {
        const request = JSON.parse(line);
        id = request.id;
        if (request.ping) return reply({id, pong: true});
        reply({id, header: await attest(request.signals, request.bundleId)});
      } catch (error) {
        reply({id, error: error instanceof Error ? error.message : String(error)});
      }
    });
  })
  .on("close", () => queue.then(() => process.exit(0)));`
