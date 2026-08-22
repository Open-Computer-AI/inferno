package service

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/agentcronfire"
	"github.com/Wei-Shaw/sub2api/ent/enttest"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// firerTestIssuer is the fixed `iss` every fixture-minted fire token carries,
// mirroring the fixed issuer literal every other OAuth-token test in this
// package uses.
const firerTestIssuer = "https://portal.example"

// fakeCronWheel is a counting stand-in for *TimingWheelService: RehydrateOnBoot's
// contract is "exactly these rows get scheduled", which does not need go-zero's
// real 1-second tick to prove -- see TimingWheelService.Schedule/Cancel for the
// real methods this structurally satisfies.
type fakeCronWheel struct {
	mu        sync.Mutex
	scheduled int
	fns       map[string]func()
}

func newFakeCronWheel() *fakeCronWheel {
	return &fakeCronWheel{fns: make(map[string]func())}
}

func (w *fakeCronWheel) Schedule(name string, _ time.Duration, fn func()) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.scheduled++
	w.fns[name] = fn
}

func (w *fakeCronWheel) Cancel(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.fns, name)
}

// firerFixture is the real ent client (AgentCronFire + Agent + SecuritySecret
// all live on the one client, same as agentCronFixture/newPaymentConfigServiceTestClient),
// a fake HTTP agent endpoint whose response is controlled per-call, and a fake
// timing wheel so RehydrateOnBoot's arming can be counted without waiting on a
// real timer.
type firerFixture struct {
	t      *testing.T
	client *dbent.Client
	keySvc *OAuthKeyService
	wheel  *fakeCronWheel
	issuer string
	now    time.Time

	server *httptest.Server

	mu         sync.Mutex
	respStatus int
	respBody   string

	agentRowID  int64
	agentPubID  string
	armedFireID int64

	seq int
}

func newFirerTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	dbName := fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()),
	)
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err, "open sqlite")
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err, "enable foreign keys")

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// newFirerFixture wires an AgentCronFirer over an isolated in-memory sqlite
// client, a fake timing wheel, and a real *OAuthKeyService (so mintFireToken
// signs against an actual RSA key and JWKS() can serve it back out).
func newFirerFixture(t *testing.T) (*AgentCronFirer, *firerFixture) {
	t.Helper()

	client := newFirerTestClient(t)
	keySvc := NewOAuthKeyService(client)
	wheel := newFakeCronWheel()

	fx := &firerFixture{
		t:          t,
		client:     client,
		keySvc:     keySvc,
		wheel:      wheel,
		issuer:     firerTestIssuer,
		now:        time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
		respStatus: http.StatusOK,
		respBody:   `{}`,
	}

	fx.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fx.mu.Lock()
		status, body := fx.respStatus, fx.respBody
		fx.mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fx.server.Close)

	agentRow, err := client.Agent.Create().
		SetPublicID("agent:test-agent").
		SetUserID(1).
		SetOrgID(1).
		SetName("test agent").
		Save(context.Background())
	require.NoError(t, err, "seed owning agent row")
	fx.agentRowID = agentRow.ID
	fx.agentPubID = agentRow.PublicID

	f := NewAgentCronFirer(client, keySvc, wheel, fx.issuer)
	f.now = func() time.Time { return fx.now }

	armed := fx.seedFire("armed", fx.now.Add(time.Hour))
	fx.armedFireID = armed.ID

	return f, fx
}

// seedFire creates one fire row owned by the fixture's agent, in the given
// state, at the given fire_at, with its callback_url pointed at the fixture's
// fake HTTP server.
func (fx *firerFixture) seedFire(state string, fireAt time.Time) *dbent.AgentCronFire {
	fx.t.Helper()
	fx.seq++
	row, err := fx.client.AgentCronFire.Create().
		SetAgentRowID(fx.agentRowID).
		SetJobID(fmt.Sprintf("job-%d", fx.seq)).
		SetFireAt(fireAt).
		SetCallbackURL(fx.server.URL).
		SetDedupKey(fmt.Sprintf("job-%d:%s", fx.seq, fireAt.Format(time.RFC3339))).
		SetScheduleID(fmt.Sprintf("cron_seed_%d", fx.seq)).
		SetState(agentcronfire.State(state)).
		Save(context.Background())
	require.NoError(fx.t, err, "seed fire")
	return row
}

// agentResponds sets what the fake HTTP agent endpoint returns for every
// subsequent FireNow call, until the next agentResponds.
func (fx *firerFixture) agentResponds(status int, body string) {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	fx.respStatus = status
	fx.respBody = body
}

func (fx *firerFixture) reload(id int64) *dbent.AgentCronFire {
	fx.t.Helper()
	row, err := fx.client.AgentCronFire.Get(context.Background(), id)
	require.NoError(fx.t, err, "reload fire %d", id)
	return row
}

func (fx *firerFixture) stateOf(id int64) string {
	return string(fx.reload(id).State)
}

func (fx *firerFixture) attemptsOf(id int64) int {
	return fx.reload(id).Attempts
}

// parseWithJWKS verifies tok against the JWKS OAuthKeyService actually
// serves (svc.JWKS), not against the raw private key -- a key-based
// assertion would pass even with a wrong kid, and the agent resolves the
// verification key by kid alone.
func (fx *firerFixture) parseWithJWKS(t *testing.T, tok string) jwt.MapClaims {
	t.Helper()

	jwks, err := fx.keySvc.JWKS(context.Background())
	require.NoError(t, err, "JWKS")
	keys, ok := jwks["keys"].([]map[string]any)
	require.True(t, ok, "JWKS keys shape")
	require.NotEmpty(t, keys, "JWKS keys non-empty")

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	var claims jwt.MapClaims
	parsed, err := parser.ParseWithClaims(tok, jwt.MapClaims{}, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		for _, k := range keys {
			if k["kid"] == kid {
				return rsaPublicKeyFromTestJWK(k)
			}
		}
		return nil, fmt.Errorf("kid %q not present in served JWKS", kid)
	})
	require.NoError(t, err, "parse fire token against served JWKS")
	require.True(t, parsed.Valid, "fire token must verify")
	claims, ok = parsed.Claims.(jwt.MapClaims)
	require.True(t, ok, "claims shape")
	return claims
}

// rsaPublicKeyFromTestJWK reverses OAuthKeyService.JWKS' own encoding
// (base64url n/e, no padding on the exponent) back into an *rsa.PublicKey.
func rsaPublicKeyFromTestJWK(k map[string]any) (*rsa.PublicKey, error) {
	nStr, _ := k["n"].(string)
	eStr, _ := k["e"].(string)
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// ---------------------------------------------------------------------------
// Step 1: the JWT-claims test -- the security boundary of the whole feature.
// ---------------------------------------------------------------------------

// TestFireTokenCarriesEveryClaimTheAgentVerifierRequires is task-5-brief.md
// Step 1, verbatim. Cited to plugins/cron_providers/chronos/verify.py:95,125-141
// (read-only client repo): the `purpose` claim is what stops a general agent
// access token being replayed against /api/cron/fire.
func TestFireTokenCarriesEveryClaimTheAgentVerifierRequires(t *testing.T) {
	f, fx := newFirerFixture(t)
	tok, err := f.mintFireToken(context.Background(), "agent:abc", "job-1")
	require.NoError(t, err)

	claims := fx.parseWithJWKS(t, tok) // verify via the SERVED JWKS, not the raw key

	require.Equal(t, "cron_fire", claims["purpose"],
		"verify.py:140 rejects any token without purpose==cron_fire -- without this "+
			"claim an ordinary agent access token would be replayable against /api/cron/fire")
	require.Equal(t, "agent:abc", claims["aud"], "verify.py:125 requires aud")
	require.NotEmpty(t, claims["exp"], "verify.py:125 requires exp")
	require.Equal(t, fx.issuer, claims["iss"], "verify.py:133 checks issuer")
}

// TestMintFireTokenRefusesBlankIssuer mirrors mintAccessToken's own refusal
// (oauth_token_service.go's ErrIssuerNotConfigured): an unverifiable token
// is worse than no token.
func TestMintFireTokenRefusesBlankIssuer(t *testing.T) {
	f, _ := newFirerFixture(t)
	f.issuer = ""

	_, err := f.mintFireToken(context.Background(), "agent:abc", "job-1")
	require.ErrorIs(t, err, ErrIssuerNotConfigured)
}

// ---------------------------------------------------------------------------
// Step 6: the response-contract tests.
// ---------------------------------------------------------------------------

// TestNon2xxIsRetriedAndJobGone200IsNot is task-5-brief.md Step 6, verbatim.
// Both rules are cited to hermes_cli/web_routers/cron.py's own docstring
// (read-only client repo) and both invert easily.
func TestNon2xxIsRetriedAndJobGone200IsNot(t *testing.T) {
	f, fx := newFirerFixture(t)

	// 503: the agent's gateway is still booting. cron.py returns this
	// DELIBERATELY so the portal retries.
	fx.agentResponds(503, `{"error":"gateway unreachable"}`)
	require.NoError(t, f.FireNow(context.Background(), fx.armedFireID))
	require.Equal(t, "armed", fx.stateOf(fx.armedFireID), "non-2xx stays armed for retry")
	require.Equal(t, 1, fx.attemptsOf(fx.armedFireID))

	// 200: the job is gone (cancelled/completed). Retrying an intentionally
	// absent fire is the bug this guards.
	fx.agentResponds(200, `{"status":"job not found"}`)
	require.NoError(t, f.FireNow(context.Background(), fx.armedFireID))
	require.Equal(t, "fired", fx.stateOf(fx.armedFireID), "200 is terminal, never retried")
}

// TestFireNowRecordsLastError proves the "record last_error" half of the
// retry contract, which TestNon2xxIsRetriedAndJobGone200IsNot's state/attempts
// assertions alone would not catch if last_error were silently dropped.
func TestFireNowRecordsLastError(t *testing.T) {
	f, fx := newFirerFixture(t)

	fx.agentResponds(503, `{"error":"gateway unreachable"}`)
	require.NoError(t, f.FireNow(context.Background(), fx.armedFireID))

	row := fx.reload(fx.armedFireID)
	require.NotEmpty(t, row.LastError, "non-2xx must record last_error")
}

// TestFireNowIsIdempotentOnAlreadyFiredRow guards the other direction of the
// rehydration invariant: even called directly (not just via the wheel), FireNow
// must never re-POST a fire that is not armed.
func TestFireNowIsIdempotentOnAlreadyFiredRow(t *testing.T) {
	f, fx := newFirerFixture(t)
	fired := fx.seedFire("fired", fx.now.Add(time.Hour))

	calls := 0
	fx.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(200)
	})

	require.NoError(t, f.FireNow(context.Background(), fired.ID))
	require.Equal(t, 0, calls, "a non-armed row must never be POSTed to")
	require.Equal(t, "fired", fx.stateOf(fired.ID))
}

// ---------------------------------------------------------------------------
// Step 7: the rehydration test.
// ---------------------------------------------------------------------------

// TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes is
// task-5-brief.md Step 7, verbatim. TimingWheelService is in-memory: nothing
// errors when rehydration is missing, the work simply never happens.
func TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes(t *testing.T) {
	f, fx := newFirerFixture(t)
	// newFirerFixture already seeded one armed fire (fx.armedFireID) an hour
	// out; add the brief's exact remaining rows.
	fx.seedFire("armed", fx.now.Add(-time.Minute)) // overdue while we were down: fires once
	fx.seedFire("fired", fx.now.Add(time.Hour))    // must NOT re-fire
	fx.seedFire("cancelled", fx.now.Add(time.Hour))

	n, err := f.RehydrateOnBoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n, "only armed rows; a fired row must never re-fire on rehydrate")
	require.Equal(t, 2, fx.wheel.scheduled)
}

// TestFireNowPostsAuthorizationBearer is a smoke test that FireNow actually
// sends the minted token as an Authorization: Bearer header on the callback
// POST -- the mint tests prove the token's claims; this proves it is used.
func TestFireNowPostsAuthorizationBearer(t *testing.T) {
	f, fx := newFirerFixture(t)

	var gotAuth string
	var gotBody map[string]any
	fx.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(200)
	})

	require.NoError(t, f.FireNow(context.Background(), fx.armedFireID))
	require.True(t, strings.HasPrefix(gotAuth, "Bearer "), "callback must carry a bearer token")

	claims := fx.parseWithJWKS(t, strings.TrimPrefix(gotAuth, "Bearer "))
	require.Equal(t, fx.agentPubID, claims["aud"], "aud must be the owning agent's public_id")
	require.Equal(t, "cron_fire", claims["purpose"])
}
