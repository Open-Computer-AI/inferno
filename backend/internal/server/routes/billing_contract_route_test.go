package routes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// These tests run the REAL chain: the real RS256 key service, the real
// oauth_client registry, the real RequireOAuthScope middleware, the real
// handler and the real BillingContractService. Only the six data sources the
// service composes are faked (balance, org, usage, payment, plan catalog,
// subscription), because the point here is the WIRE — status codes, scope
// enforcement and the exact JSON keys — not the numbers.
//
// The ent client is sqlite purely to hold the signing key and one client row;
// nothing under test is a SQL behaviour.

const (
	billingRouteIssuer   = "https://portal.example.com"
	billingRouteClientID = "hermes-cli"
	billingRouteUserID   = int64(7)
)

// --- fakes for the six sources --------------------------------------------

type stubBillingBalance struct{ balance float64 }

func (s stubBillingBalance) GetUserBalance(context.Context, int64) (float64, error) {
	return s.balance, nil
}

type stubBillingOrg struct{}

func (stubBillingOrg) OrgsForUser(context.Context, int64) ([]*dbent.Org, error) {
	return []*dbent.Org{{ID: 1, Slug: "acme-1a2b", Name: "Acme"}}, nil
}
func (stubBillingOrg) RoleIn(context.Context, int64, int64) (string, error) {
	return service.OrgRoleOwner, nil
}

type stubBillingUsage struct{ actualCost float64 }

func (s stubBillingUsage) GetStatsByUser(context.Context, int64, time.Time, time.Time) (*service.UsageStats, error) {
	return &service.UsageStats{TotalActualCost: s.actualCost}, nil
}

type stubBillingPayment struct{}

func (stubBillingPayment) GetAvailableMethodLimits(context.Context) (*service.MethodLimitsResponse, error) {
	return &service.MethodLimitsResponse{GlobalMin: 5, GlobalMax: 500}, nil
}
func (stubBillingPayment) IsPaymentEnabled(context.Context) bool { return true }

// stubBillingPlan is the plan catalog GET /api/billing/subscription's
// `tiers` picker is built from -- one for-sale plan, group 9, matching
// stubBillingSubscription's active row below so isCurrent is exercised.
//
// SortOrder is deliberately LEFT AT ZERO (ruling C-2): that is
// ent/schema/subscription_plan.go:62's Default(0) and what Inferno's own plan
// editor posts, so the route harness exercises the real default case rather
// than a hand-picked positive value. tierOrder on the wire must still be >= 1.
type stubBillingPlan struct{}

func (stubBillingPlan) ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro", Price: 20, ForSale: true},
	}, nil
}

func (stubBillingPlan) GetGroupInfoMap(context.Context, []*dbent.SubscriptionPlan) map[int64]service.PlanGroupInfo {
	limit := 100.0
	return map[int64]service.PlanGroupInfo{9: {MonthlyLimitUSD: &limit}}
}

// stubBillingSubscription is the caller's own active subscription(s). The
// zero value (nil Subs) is "no active subscription" -- current:null.
type stubBillingSubscription struct {
	subs []service.UserSubscription
}

func (s stubBillingSubscription) ListActiveUserSubscriptions(context.Context, int64) ([]service.UserSubscription, error) {
	return s.subs, nil
}

// billingRouteActiveSubscription is the default active row used by
// newBillingRouteEnv: group 9, matching stubBillingPlan's plan, so isCurrent
// is exercised on the wire.
func billingRouteActiveSubscription() stubBillingSubscription {
	limit := 100.0
	return stubBillingSubscription{subs: []service.UserSubscription{{
		ID: 1, UserID: billingRouteUserID, GroupID: 9,
		ExpiresAt:       time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC),
		MonthlyUsageUSD: 30,
		Group:           &service.Group{ID: 9, Name: "Pro", MonthlyLimitUSD: &limit},
	}}}
}

// stubBillingChargeSource backs Task 5's two IMPLEMENTED writes over the
// REAL entClient the harness already builds -- GetOrder issues a REAL query
// against the REAL PaymentOrder table (mirroring PaymentService.GetOrder's
// exact error contract, payment_order.go:918-927), so the cross-tenant test
// below seeds a genuine second user's row in the SAME database rather than a
// fake stand-in. CreateOrder is a controllable stub: actually invoking a
// payment provider needs a configured Razorpay/Stripe instance this harness
// has no reason to stand up, and the wire-level property under test is the
// STATUS CODE + BODY SHAPE of the surrounding contract, not provider I/O.
type stubBillingChargeSource struct {
	entClient   *dbent.Client
	createResp  *service.CreateOrderResponse
	createErr   error
	createCalls int
}

func (s *stubBillingChargeSource) CreateOrder(_ context.Context, _ service.CreateOrderRequest) (*service.CreateOrderResponse, error) {
	s.createCalls++
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.createResp, nil
}

func (s *stubBillingChargeSource) GetOrder(ctx context.Context, orderID, userID int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, orderID)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != userID {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission for this order")
	}
	return o, nil
}

// inMemoryIdempotencyRepo satisfies service.IdempotencyRepository without
// SQL: internal/repository's real implementation (repository.
// NewIdempotencyRepository) is Postgres-only raw SQL ($1 positional
// placeholders, `ON CONFLICT ... RETURNING`) and errors out against this
// harness's sqlite backing store -- confirmed empirically (it surfaces as
// IDEMPOTENCY_STORE_UNAVAILABLE against sqlite), not assumed. The dedup
// SEMANTICS under test here (CreateProcessing's race-safe claim, a REPLAY
// returning the stored response) are exercised faithfully in-process; the
// Postgres SQL itself is a separate, already-covered concern
// (internal/repository/idempotency_repo_integration_test.go).
type inMemoryIdempotencyRepo struct {
	mu     sync.Mutex
	nextID int64
	data   map[string]*service.IdempotencyRecord
}

func newInMemoryIdempotencyRepo() *inMemoryIdempotencyRepo {
	return &inMemoryIdempotencyRepo{nextID: 1, data: make(map[string]*service.IdempotencyRecord)}
}

func (r *inMemoryIdempotencyRepo) key(scope, hash string) string { return scope + "|" + hash }

func cloneIdempotencyRecord(in *service.IdempotencyRecord) *service.IdempotencyRecord {
	if in == nil {
		return nil
	}
	out := *in
	if in.LockedUntil != nil {
		v := *in.LockedUntil
		out.LockedUntil = &v
	}
	if in.ResponseStatus != nil {
		v := *in.ResponseStatus
		out.ResponseStatus = &v
	}
	if in.ResponseBody != nil {
		v := *in.ResponseBody
		out.ResponseBody = &v
	}
	if in.ErrorReason != nil {
		v := *in.ErrorReason
		out.ErrorReason = &v
	}
	return &out
}

func (r *inMemoryIdempotencyRepo) CreateProcessing(_ context.Context, record *service.IdempotencyRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(record.Scope, record.IdempotencyKeyHash)
	if _, ok := r.data[k]; ok {
		return false, nil
	}
	rec := cloneIdempotencyRecord(record)
	rec.ID = r.nextID
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = rec.CreatedAt
	r.nextID++
	r.data[k] = rec
	record.ID, record.CreatedAt, record.UpdatedAt = rec.ID, rec.CreatedAt, rec.UpdatedAt
	return true, nil
}

func (r *inMemoryIdempotencyRepo) GetByScopeAndKeyHash(_ context.Context, scope, keyHash string) (*service.IdempotencyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return cloneIdempotencyRecord(r.data[r.key(scope, keyHash)]), nil
}

func (r *inMemoryIdempotencyRepo) TryReclaim(_ context.Context, id int64, fromStatus string, now, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		if rec.Status != fromStatus {
			return false, nil
		}
		if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
			return false, nil
		}
		rec.Status = service.IdempotencyStatusProcessing
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		rec.ErrorReason = nil
		rec.UpdatedAt = time.Now()
		return true, nil
	}
	return false, nil
}

func (r *inMemoryIdempotencyRepo) ExtendProcessingLock(_ context.Context, id int64, requestFingerprint string, newLockedUntil, newExpiresAt time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		if rec.Status != service.IdempotencyStatusProcessing || rec.RequestFingerprint != requestFingerprint {
			return false, nil
		}
		rec.LockedUntil = &newLockedUntil
		rec.ExpiresAt = newExpiresAt
		rec.UpdatedAt = time.Now()
		return true, nil
	}
	return false, nil
}

func (r *inMemoryIdempotencyRepo) MarkSucceeded(_ context.Context, id int64, responseStatus int, responseBody string, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		rec.Status = service.IdempotencyStatusSucceeded
		rec.LockedUntil = nil
		rec.ExpiresAt = expiresAt
		rec.UpdatedAt = time.Now()
		rec.ErrorReason = nil
		rec.ResponseStatus = &responseStatus
		rec.ResponseBody = &responseBody
		return nil
	}
	return errors.New("record not found")
}

func (r *inMemoryIdempotencyRepo) MarkFailedRetryable(_ context.Context, id int64, errorReason string, lockedUntil, expiresAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range r.data {
		if rec.ID != id {
			continue
		}
		rec.Status = service.IdempotencyStatusFailedRetryable
		rec.LockedUntil = &lockedUntil
		rec.ExpiresAt = expiresAt
		rec.UpdatedAt = time.Now()
		rec.ErrorReason = &errorReason
		return nil
	}
	return errors.New("record not found")
}

func (r *inMemoryIdempotencyRepo) DeleteExpired(_ context.Context, now time.Time, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var deleted int64
	for k, rec := range r.data {
		if !rec.ExpiresAt.After(now) {
			delete(r.data, k)
			deleted++
		}
	}
	return deleted, nil
}

// --- harness --------------------------------------------------------------

type billingRouteEnv struct {
	router  *gin.Engine
	keySvc  *service.OAuthKeyService
	db      *dbent.Client
	charge  *stubBillingChargeSource
	sqlConn *sql.DB
}

func newBillingRouteEnv(t *testing.T) *billingRouteEnv {
	return newBillingRouteEnvWithSubscription(t, billingRouteActiveSubscription())
}

// newBillingRouteEnvWithSubscription lets a test override what the caller's
// "current" subscription looks like -- in particular, an empty
// stubBillingSubscription{} to exercise the no-plan / current:null path.
func newBillingRouteEnvWithSubscription(t *testing.T, subSrc stubBillingSubscription) *billingRouteEnv {
	return newBillingRouteEnvWithSources(t, stubBillingPlan{}, subSrc)
}

// newBillingRouteEnvWithSources is the fully-parameterised constructor;
// newBillingRouteEnv and newBillingRouteEnvWithSubscription are its two
// common shorthands.
func newBillingRouteEnvWithSources(t *testing.T, planSrc service.BillingPlanSource, subSrc service.BillingSubscriptionSource) *billingRouteEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbName := fmt.Sprintf("file:%s?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := sql.Open("sqlite", dbName)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	entClient := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(entsql.OpenDB(dialect.SQLite, db))))
	t.Cleanup(func() { _ = entClient.Close() })

	_, err = entClient.OAuthClient.Create().
		SetClientID(billingRouteClientID).
		SetKind("FIRST_PARTY").
		SetName("hermes cli").
		SetOwnerUserID(billingRouteUserID).
		SetOrgID(1).
		SetRedirectURIOrigin("https://agent.example.com").
		SetStatus(service.ClientActive).
		Save(context.Background())
	require.NoError(t, err)

	keySvc := service.NewOAuthKeyService(entClient)
	clientSvc := service.NewOAuthClientService(entClient)
	tokenSvc := service.NewOAuthTokenService(entClient, keySvc, nil, nil, nil, billingRouteIssuer)

	oauthH := handler.NewOAuthHandler(keySvc, clientSvc, nil, nil, tokenSvc, nil)

	chargeSrc := &stubBillingChargeSource{entClient: entClient}
	idemCoord := service.NewIdempotencyCoordinator(newInMemoryIdempotencyRepo(), service.DefaultIdempotencyConfig())

	billingSvc := service.NewBillingContractService(
		stubBillingBalance{balance: 42.50},
		stubBillingOrg{},
		stubBillingUsage{actualCost: 1.25},
		stubBillingPayment{},
		planSrc,
		subSrc,
		chargeSrc,
		idemCoord,
		billingRouteIssuer,
	)

	r := gin.New()
	RegisterBillingContractRoutes(r, oauthH, handler.NewBillingContractHandler(billingSvc))
	return &billingRouteEnv{router: r, keySvc: keySvc, db: entClient, charge: chargeSrc, sqlConn: db}
}

func (e *billingRouteEnv) mint(t *testing.T, scope string) string {
	t.Helper()
	return e.mintForUser(t, billingRouteUserID, scope)
}

// mintForUser mints a token for an arbitrary user id -- used by the charge-
// status cross-tenant tests, which need a REAL dbent.User row (PaymentOrder
// has a required FK edge to User, ent/schema/payment_order.go:176-183) and
// therefore a REAL, DB-assigned id rather than the shared billingRouteUserID
// constant.
func (e *billingRouteEnv) mintForUser(t *testing.T, userID int64, scope string) string {
	t.Helper()
	key, err := e.keySvc.Active(context.Background())
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   billingRouteIssuer,
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   billingRouteClientID,
		"scope": scope,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)
	return signed
}

func (e *billingRouteEnv) get(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	return e.getPath(t, "/api/billing/state", authorization)
}

func (e *billingRouteEnv) getPath(t *testing.T, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// do is the general request helper Task 5's write tests use: any method, an
// optional JSON body, optional extra headers (Idempotency-Key in
// particular). authorization == "" sends no Authorization header at all.
func (e *billingRouteEnv) do(t *testing.T, method, path, authorization, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	req := httptest.NewRequest(method, path, nil)
	if body != "" {
		reader = strings.NewReader(body)
		req = httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// --- the wire contract ----------------------------------------------------

// TestBillingStateRouteReturnsBareJSONNotThePanelEnvelope asserts the ABSENCE
// of code/message/data.
//
// Asserting only that the fields we want are present would pass against an
// enveloped body too, because response.Success nests the object under "data"
// and the presence check would just be looking in the wrong place and finding
// nothing to complain about. A wrong envelope is not an error the client
// raises: json.loads succeeds, every payload.get() misses, and
// billing_state_from_payload returns a well-formed BillingState full of None.
// The only way to catch it is to assert the envelope's keys are not there.
func TestBillingStateRouteReturnsBareJSONNotThePanelEnvelope(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "Bearer "+env.mint(t, service.ScopeBillingRead))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	for _, k := range []string{"code", "message", "data"} {
		require.NotContains(t, body, k,
			"the panel's {code,message,data} envelope must never appear here — the client parses the raw object and an envelope is silently mis-parsed, not rejected")
	}

	// And the payload really is at the top level, in the keys
	// agent/billing_view.py::billing_state_from_payload reads.
	require.Equal(t, "42.5", body["balanceUsd"])
	require.Equal(t, true, body["cliBillingEnabled"])
	require.Equal(t, true, body["canChangePlan"])

	org, ok := body["org"].(map[string]any)
	require.True(t, ok, "org must be an object at the top level")
	require.Equal(t, "1", org["id"])
	require.Equal(t, "acme-1a2b", org["slug"])
	require.Equal(t, "OWNER", org["role"])

	bounds, ok := body["bounds"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "5", bounds["minUsd"])
	require.Equal(t, "500", bounds["maxUsd"])

	cap, ok := body["monthlyCap"].(map[string]any)
	require.True(t, ok, "the monthly spend picture is monthlyCap, NOT bounds")
	require.Nil(t, cap["limitUsd"], "no per-org monthly ceiling is modelled; null reads as 'no limit configured'")
	require.Equal(t, "1.25", cap["spentThisMonthUsd"])
	require.Equal(t, false, cap["isDefaultCeiling"])

	card, ok := body["card"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "none", card["kind"])

	auto, ok := body["autoReload"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, false, auto["enabled"])

	presets, ok := body["chargePresets"].([]any)
	require.True(t, ok, "chargePresets must be a JSON array, never null")
	require.Empty(t, presets)

	// The live bug this whole plan exists to close: without a
	// server-supplied portalUrl the client builds
	// {portal_base}/billing?topup=open, whose default base is
	// portal.nousresearch.com and whose /billing path does not exist on
	// Inferno at all (the recharge route is /purchase).
	require.Equal(t, "https://portal.example.com/purchase", body["portalUrl"])
	portal, ok := body["portalUrl"].(string)
	require.True(t, ok)
	require.NotContains(t, portal, "nousresearch.com",
		"the CLI must never be handed a link to the upstream portal's billing page")
}

// TestBillingStateRouteAdmitsAStockClientToken is the conformance case, and
// the reason this endpoint no longer demands billing:read.
//
// The token below carries EXACTLY what a shipping hermes client holds after a
// normal login: hermes_cli/auth.py's DEFAULT_NOUS_SCOPE, which is
// "inference:invoke" and nothing else. billing:read appears nowhere in the
// client, its only escalation asks for billing:manage, and our AS stores the
// requested scope verbatim -- so under the original billing:read gate this
// exact request 403'd, the client failed open to "not logged in", and the whole
// adapter was unreachable. This test is what would catch that regression.
//
// MUTATION CHECK: putting service.ScopeBillingRead back as the required scope
// fails this test at 403 vs 200.
func TestBillingStateRouteAdmitsAStockClientToken(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code,
		"a token carrying only the scope the real client requests must be admitted; body: %s", rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "42.5", body["balanceUsd"])
}

// TestBillingStateRouteAdmitsATokenWithNoScopeAtAll: RFC 6749 §3.3 makes the
// scope parameter optional, so a token minted from a scope-less request carries
// an empty claim. It is still a verified statement about who the holder is,
// which is all this endpoint needs.
func TestBillingStateRouteAdmitsATokenWithNoScopeAtAll(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "Bearer "+env.mint(t, ""))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// TestBillingStateRouteRejectsAnUnauthenticatedCall is the line that did NOT
// move when the scope requirement was dropped.
//
// Dropping the SCOPE must not drop AUTHENTICATION: this is the caller's own
// balance, and a 200 here would make every user's money readable by anyone. A
// 404 would mean the group never mounted at all.
//
// Note this endpoint fails closed TWICE, independently: RequireOAuthScope
// aborts before the handler runs, and BillingContractHandler.State refuses to
// compose anything without an identity on the context (covered in
// handler/billing_contract_handler_test.go). That redundancy is why this test
// alone cannot prove the middleware is mounted -- removing the middleware
// leaves this green, because the handler's own 401 takes over. The positive
// cases above are the discriminator: with no middleware they get 401 instead
// of 200. Both directions are asserted on purpose.
func TestBillingStateRouteRejectsAnUnauthenticatedCall(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestBillingStateRouteRejectsAForgedToken proves the verifier still runs.
// "No particular scope required" must not decay into "any bearer string".
func TestBillingStateRouteRejectsAForgedToken(t *testing.T) {
	env := newBillingRouteEnv(t)

	// Correctly shaped, signed with a key this server never issued.
	forged := mintForeignRS256Token(t)
	rec := env.get(t, "Bearer "+forged)

	require.Equal(t, http.StatusUnauthorized, rec.Code, "body: %s", rec.Body.String())
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestBillingStateRouteRejectsAnExpiredToken: same point, on the other axis.
func TestBillingStateRouteRejectsAnExpiredToken(t *testing.T) {
	env := newBillingRouteEnv(t)

	key, err := env.keySvc.Active(context.Background())
	require.NoError(t, err)
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   billingRouteIssuer,
		"sub":   strconv.FormatInt(billingRouteUserID, 10),
		"aud":   billingRouteClientID,
		"scope": service.ScopeInferenceInvoke,
		"iat":   time.Now().Add(-2 * time.Hour).Unix(),
		"exp":   time.Now().Add(-time.Hour).Unix(),
	})
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)

	rec := env.get(t, "Bearer "+signed)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// mintForeignRS256Token signs a well-formed access token with an RSA key this
// server has never seen, keeping the kid of a key it HAS seen so the failure is
// the signature check and not a missing header.
func mintForeignRS256Token(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   billingRouteIssuer,
		"sub":   strconv.FormatInt(billingRouteUserID, 10),
		"aud":   billingRouteClientID,
		"scope": service.ScopeInferenceInvoke,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = "not-a-key-this-server-issued"
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

// TestBillingStateRouteIsNotMountedUnderAPIV1 pins the mount point. The client
// hardcodes /api/billing/state (hermes_cli/nous_billing.py:481); moving it
// under the panel's versioned prefix would 404 every real call.
func TestBillingStateRouteIsNotMountedUnderAPIV1(t *testing.T) {
	env := newBillingRouteEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/state", nil)
	req.Header.Set("Authorization", "Bearer "+env.mint(t, service.ScopeBillingRead))
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ===========================================================================
// GET /api/billing/subscription
// ===========================================================================

// TestBillingSubscriptionRouteReturnsBareJSONNotThePanelEnvelope is the
// type-fidelity test the task-3 brief calls for: these three checks fail
// SILENTLY on the client (subscription_state_from_payload's fail-open
// parsing), so each is asserted against the ENCODED JSON BYTES, not the Go
// struct --
//
//   - canChangePlan must decode as a JSON bool, not the string "true"
//     (a string parses to None per subscription_view.py:236-240).
//   - tiers must decode as a JSON array, even non-empty here, never null
//     (a non-list silently becomes () per :224-229).
//   - context must be exactly "personal" or "team" (any other string
//     silently becomes "personal" per :221-222).
//
// It also asserts the absence of the panel's {code,message,data} envelope,
// same reasoning as TestBillingStateRouteReturnsBareJSONNotThePanelEnvelope.
func TestBillingSubscriptionRouteReturnsBareJSONNotThePanelEnvelope(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.getPath(t, "/api/billing/subscription", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	raw := rec.Body.Bytes()
	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))

	for _, k := range []string{"code", "message", "data"} {
		require.NotContains(t, body, k,
			"the panel's {code,message,data} envelope must never appear here")
	}

	// canChangePlan: a real JSON bool, not the string "true".
	require.Contains(t, string(raw), `"canChangePlan":true`,
		"canChangePlan must serialise as a bare JSON bool -- a quoted \"true\" parses to None on the client, not true")
	canChangePlan, ok := body["canChangePlan"].(bool)
	require.True(t, ok, "canChangePlan must decode as a Go bool, not a string")
	require.True(t, canChangePlan)

	// context: exactly "personal" or "team".
	context, ok := body["context"].(string)
	require.True(t, ok)
	require.Contains(t, []string{"personal", "team"}, context)

	// tiers: a JSON array (non-empty here), never null.
	require.Contains(t, string(raw), `"tiers":[`, "tiers must serialise as a JSON array literal")
	tiers, ok := body["tiers"].([]any)
	require.True(t, ok, "tiers must decode as a Go slice, never null")
	require.Len(t, tiers, 1)
	tier, ok := tiers[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "9", tier["tierId"])
	require.Equal(t, "Pro", tier["name"])
	require.Equal(t, "20.00", tier["dollarsPerMonthDisplay"], "always 2dp -- see billingDisplayMoney")
	require.Equal(t, "100", tier["monthlyCredits"])
	require.Equal(t, true, tier["isCurrent"])
	require.Equal(t, true, tier["isEnabled"])

	// tierOrder on the WIRE (ruling C-2). stubBillingPlan leaves SortOrder at
	// the schema default 0; the emitted rank must still be a bare JSON number
	// >= 1, because agent/subscription_view.py:373-379 and
	// ui-tui/src/components/subscriptionOverlay.tsx:381,525 all drop
	// tier_order <= 0 and the user gets a blank picker with no error.
	require.Contains(t, string(raw), `"tierOrder":1`, "tierOrder must be an unquoted JSON number >= 1; body: %s", string(raw))
	require.NotContains(t, string(raw), `"tierOrder":0`)
	require.NotContains(t, string(raw), `"tierOrder":"1"`, "tierOrder must not be a string -- _parse_tier calls int() on it")
	require.IsType(t, float64(0), tier["tierOrder"])
	require.Equal(t, float64(1), tier["tierOrder"])

	// current: a real object here (the active-subscription case).
	current, ok := body["current"].(map[string]any)
	require.True(t, ok, "current must be an object when the caller has an active subscription")
	require.Equal(t, "9", current["tierId"])
	require.Equal(t, "Pro", current["tierName"])
	require.Equal(t, "100", current["monthlyCredits"])
	require.Equal(t, "70", current["creditsRemaining"])
	require.Equal(t, false, current["cancelAtPeriodEnd"])

	org, ok := body["org"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "1", org["id"])
	require.Equal(t, "OWNER", org["role"])

	require.Equal(t, "https://portal.example.com/purchase", body["portalUrl"])
	portal, ok := body["portalUrl"].(string)
	require.True(t, ok)
	require.NotContains(t, portal, "nousresearch.com")
}

// TestBillingSubscriptionRouteReportsNoPlanAsNullCurrent pins the exact wire
// shape agent/subscription_view.py:142-144 documents: "no plan" is
// current:null -- a PRESENT key with a JSON null value, not an omitted key
// and not an object of nulls (the old all-null-object shape is gone).
func TestBillingSubscriptionRouteReportsNoPlanAsNullCurrent(t *testing.T) {
	env := newBillingRouteEnvWithSubscription(t, stubBillingSubscription{})
	rec := env.getPath(t, "/api/billing/subscription", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Contains(t, rec.Body.String(), `"current":null`,
		"a caller with no active subscription must get a PRESENT current key with a null value")

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	current, present := body["current"]
	require.True(t, present, "current must be a present key, not an omitted one")
	require.Nil(t, current)

	// tiers must still be a real array: the plan catalog is independent of
	// whether the caller has an active subscription.
	tiers, ok := body["tiers"].([]any)
	require.True(t, ok)
	require.Len(t, tiers, 1)
	tier := tiers[0].(map[string]any)
	require.Equal(t, false, tier["isCurrent"], "with no active subscription, no tier can be current")
}

// TestBillingSubscriptionRouteAdmitsAStockClientToken mirrors State's
// conformance case: a token carrying only inference:invoke (what a real
// hermes login actually holds) must be admitted, not the unsatisfiable
// billing:read.
func TestBillingSubscriptionRouteAdmitsAStockClientToken(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.getPath(t, "/api/billing/subscription", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// TestBillingSubscriptionRouteRejectsAnUnauthenticatedCall mirrors State's:
// dropping the SCOPE requirement must not drop AUTHENTICATION.
func TestBillingSubscriptionRouteRejectsAnUnauthenticatedCall(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.getPath(t, "/api/billing/subscription", "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestBillingSubscriptionRouteIsNotMountedUnderAPIV1 mirrors State's mount-
// point pin.
func TestBillingSubscriptionRouteIsNotMountedUnderAPIV1(t *testing.T) {
	env := newBillingRouteEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/subscription", nil)
	req.Header.Set("Authorization", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ===========================================================================
// The two phantom GETs (task-3 brief, Step 7).
//
// GET /api/billing/auto-top-up and GET /api/billing/subscription/pending-change
// must NOT exist. The client sends PATCH to the first (nous_billing.py:480)
// and PUT/DELETE to the second (:594, :631); it reads both states out of
// GET /api/billing/state instead, and there is no GET call site for either
// path anywhere in nous_billing.py. This pins both at 404 so a future
// well-meaning addition (F-10's exact failure mode) is a deliberate act, not
// drift back into "a route answering a question nobody asks".
// ===========================================================================

func TestPhantomGETAutoTopUpDoesNotExist(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.getPath(t, "/api/billing/auto-top-up", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusNotFound, rec.Code,
		"the client never sends GET here (it sends PATCH, nous_billing.py:480) -- a GET route would be an endpoint answering a question nobody asks")
}

func TestPhantomGETSubscriptionPendingChangeDoesNotExist(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.getPath(t, "/api/billing/subscription/pending-change", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusNotFound, rec.Code,
		"the client never sends GET here (it sends PUT/DELETE, nous_billing.py:594,:631) -- both states are read out of GET /api/billing/state instead")
}

// ===========================================================================
// ruling R-3.1 / R-3.2 -- end-to-end over real HTTP JSON.
// ===========================================================================

// stubBillingPlanMultiPeriod is a group that sells at two billing periods
// (monthly and annual) -- the exact shape ruling R-3.1 exists for.
type stubBillingPlanMultiPeriod struct{}

func (stubBillingPlanMultiPeriod) ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro Monthly", Price: 20, SortOrder: 1, ForSale: true, ValidityDays: 1, ValidityUnit: "months"},
		{ID: 101, GroupID: 9, Name: "Pro Annual", Price: 180, SortOrder: 2, ForSale: true, ValidityDays: 12, ValidityUnit: "months"},
	}, nil
}

func (stubBillingPlanMultiPeriod) GetGroupInfoMap(context.Context, []*dbent.SubscriptionPlan) map[int64]service.PlanGroupInfo {
	return nil
}

// TestBillingSubscriptionRouteCollapsesMultiPeriodGroupOnTheWire is the
// end-to-end proof, over real encoded JSON: a group selling two billing
// periods must still produce exactly one tiers[] entry, priced by its
// normalised (cheaper, per-month) option -- $180/year beats $20/month at
// $15/mo vs $20/mo.
func TestBillingSubscriptionRouteCollapsesMultiPeriodGroupOnTheWire(t *testing.T) {
	env := newBillingRouteEnvWithSources(t, stubBillingPlanMultiPeriod{}, billingRouteActiveSubscription())
	rec := env.getPath(t, "/api/billing/subscription", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	tiers, ok := body["tiers"].([]any)
	require.True(t, ok)
	require.Len(t, tiers, 1, "one group selling two billing periods must still emit exactly one tiers[] row")

	tier := tiers[0].(map[string]any)
	require.Equal(t, "9", tier["tierId"], "tierId must be the GROUP id, not either plan's own id")
	require.Equal(t, "Pro Annual", tier["name"], "the annual option's normalised $15/mo beats the monthly option's $20/mo")
	require.Equal(t, "15.00", tier["dollarsPerMonthDisplay"], "$180/year normalised to $15.00/month, not the raw $180")
}

// ===========================================================================
// Task 5 -- the 7 write endpoints, all gated on billing:manage.
//
// billing:manage is refused outright by /oauth/authorize
// (oauth_authorize_service.go:200) and no other flow can grant it, so every
// one of these 403s in production today (ruling R-5.1). Step 1 of the brief:
// "the scope gate test is the most valuable thing you will write here" --
// this section leads with it.
// ===========================================================================

// billingWrite describes one of the 7 write endpoints at its EXACT method
// and path, with a body that satisfies its own binding (where it has one)
// and the Idempotency-Key header the two money-moving endpoints require.
// Every (method, path) pair here is cited in billing_contract.go's route
// registration, which is itself cited to nous_billing.py line-by-line.
type billingWrite struct {
	name    string
	method  string
	path    string
	body    string
	headers map[string]string
}

func billingWrites() []billingWrite {
	return []billingWrite{
		{"charge", http.MethodPost, "/api/billing/charge", `{"amountUsd":10}`, map[string]string{"Idempotency-Key": "scope-gate-key-charge"}},
		{"charge_status", http.MethodGet, "/api/billing/charge/123", "", nil},
		{"subscription_preview", http.MethodPost, "/api/billing/subscription/preview", `{"subscriptionTypeId":"pro"}`, nil},
		{"subscription_upgrade", http.MethodPost, "/api/billing/subscription/upgrade", `{"subscriptionTypeId":"pro"}`, map[string]string{"Idempotency-Key": "scope-gate-key-upgrade"}},
		{"pending_change_put", http.MethodPut, "/api/billing/subscription/pending-change", `{"type":"cancellation"}`, nil},
		{"pending_change_delete", http.MethodDelete, "/api/billing/subscription/pending-change", "", nil},
		{"auto_top_up", http.MethodPatch, "/api/billing/auto-top-up", `{"enabled":true,"threshold":10,"topUpAmount":20}`, nil},
	}
}

// TestBillingWriteRoutesRejectAStockClientTokenWithoutBillingManage is Step
// 1 verbatim: a token with a VALID session but WITHOUT billing:manage --
// exactly what every real hermes login holds before a step-up
// (hermes_cli/auth.py's DEFAULT_NOUS_SCOPE == "inference:invoke") -- gets
// 403 insufficient_scope from every one of the 7, at its exact method. This
// protects a scope no flow can currently issue (ruling R-5.1): it is the
// boundary that makes all 7 unreachable in production today, and the most
// valuable test in this task.
func TestBillingWriteRoutesRejectAStockClientTokenWithoutBillingManage(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeInferenceInvoke)

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			rec := env.do(t, w.method, w.path, tok, w.body, w.headers)
			require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
			require.JSONEq(t, `{"error":"insufficient_scope"}`, rec.Body.String())
		})
	}
}

// TestBillingWriteRoutesRejectATokenWithNoScopeAtAll: RFC 6749 §3.3 makes a
// token's scope claim optional -- an empty claim must not accidentally
// satisfy billing:manage the way it satisfies State/Subscription's "" gate.
func TestBillingWriteRoutesRejectATokenWithNoScopeAtAll(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, "")

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			rec := env.do(t, w.method, w.path, tok, w.body, w.headers)
			require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestBillingWriteRoutesRejectAnUnauthenticatedCall: dropping to
// billing:manage does not relax authentication -- a request with no bearer
// at all must fail closed the same way State/Subscription do.
func TestBillingWriteRoutesRejectAnUnauthenticatedCall(t *testing.T) {
	env := newBillingRouteEnv(t)

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			rec := env.do(t, w.method, w.path, "", w.body, w.headers)
			require.Equal(t, http.StatusUnauthorized, rec.Code, "body: %s", rec.Body.String())
			require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
		})
	}
}

// TestBillingWriteRoutesAdmitABillingManageToken proves the gate is not
// simply broken closed for everyone: a token that DOES carry billing:manage
// (the only way any of these becomes reachable, per a future step-up flow
// this branch does not build) gets PAST the gate to the handler -- pinned
// here by NOT getting 403, and each endpoint's own status is checked in
// detail further down.
func TestBillingWriteRoutesAdmitABillingManageToken(t *testing.T) {
	env := newBillingRouteEnv(t)
	env.charge.createResp = &service.CreateOrderResponse{OrderID: 1}
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			rec := env.do(t, w.method, w.path, tok, w.body, w.headers)
			require.NotEqual(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		})
	}
}

// TestBillingWriteRoutesWrongMethodIs405PinningTheExactMethod is Step 7's
// "also assert the WRONG method on each path is 405". gin's default engine
// (HandleMethodNotAllowed unset) answers a wrong-method call with 404, which
// cannot distinguish "this path does not exist" from "this path exists
// under a different verb" -- exactly the ambiguity that let two wrong HTTP
// methods ship undetected earlier on this branch (F-10). Flipping
// HandleMethodNotAllowed on THIS TEST'S OWN *gin.Engine (a fresh instance
// per env, never the production router in internal/server/http.go, which
// does not set this flag anywhere in the codebase -- checked) makes gin
// answer 405 for a real method/path mismatch, confirmed empirically against
// gin's own behaviour before relying on it.
func TestBillingWriteRoutesWrongMethodIs405PinningTheExactMethod(t *testing.T) {
	env := newBillingRouteEnv(t)
	env.router.HandleMethodNotAllowed = true
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	wrongMethodFor := map[string]string{
		http.MethodPost:   http.MethodGet,
		http.MethodGet:    http.MethodPost,
		http.MethodPut:    http.MethodGet,
		http.MethodDelete: http.MethodGet,
		http.MethodPatch:  http.MethodGet,
	}

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			wrong := wrongMethodFor[w.method]
			rec := env.do(t, wrong, w.path, tok, "", nil)
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code,
				"expected 405 for %s %s (registered method is %s); body: %s", wrong, w.path, w.method, rec.Body.String())
		})
	}
}

// ---------------------------------------------------------------------------
// The 2 implementable writes.
// ---------------------------------------------------------------------------

// TestChargeRouteMissingIdempotencyKeyIs400 pins the brief's exact wording:
// "a missing header is a 400, not a default" (nous_billing.py:511-521).
func TestChargeRouteMissingIdempotencyKeyIs400(t *testing.T) {
	env := newBillingRouteEnv(t)
	env.charge.createResp = &service.CreateOrderResponse{OrderID: 1}
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPost, "/api/billing/charge", tok, `{"amountUsd":10}`, nil)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, 0, env.charge.createCalls, "a missing Idempotency-Key must never reach CreateOrder")
}

// TestChargeRouteAcceptsAndReturns202WithChargeId pins the 202 + {chargeId}
// shape (nous_billing.py:513-514 docstring; cli_billing_mixin.py:1155 reads
// result.get("chargeId")), asserted against the encoded JSON bytes.
func TestChargeRouteAcceptsAndReturns202WithChargeId(t *testing.T) {
	env := newBillingRouteEnv(t)
	env.charge.createResp = &service.CreateOrderResponse{OrderID: 4242}
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPost, "/api/billing/charge", tok, `{"amountUsd":10}`, map[string]string{"Idempotency-Key": "route-key-1"})

	require.Equal(t, http.StatusAccepted, rec.Code, "body: %s", rec.Body.String())
	require.Contains(t, rec.Body.String(), `"chargeId":"4242"`)
	for _, k := range []string{"code", "message", "data"} {
		var body map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.NotContains(t, body, k, "bare JSON, never the panel envelope")
	}
}

// TestChargeRouteSameIdempotencyKeyTwiceIsOneOrderSameChargeId is the
// brief's other Step-6 requirement, over real HTTP + the real
// *IdempotencyCoordinator backed by a real sqlite-backed repository (not a
// fake dedup re-implementation).
func TestChargeRouteSameIdempotencyKeyTwiceIsOneOrderSameChargeId(t *testing.T) {
	env := newBillingRouteEnv(t)
	env.charge.createResp = &service.CreateOrderResponse{OrderID: 555}
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)
	headers := map[string]string{"Idempotency-Key": "route-key-retry"}

	first := env.do(t, http.MethodPost, "/api/billing/charge", tok, `{"amountUsd":10}`, headers)
	second := env.do(t, http.MethodPost, "/api/billing/charge", tok, `{"amountUsd":10}`, headers)

	require.Equal(t, http.StatusAccepted, first.Code, "body: %s", first.Body.String())
	require.Equal(t, http.StatusAccepted, second.Code, "body: %s", second.Body.String())
	require.Contains(t, first.Body.String(), `"chargeId":"555"`)
	require.Contains(t, second.Body.String(), `"chargeId":"555"`, "a retried charge must resolve to the SAME chargeId")
	require.Equal(t, 1, env.charge.createCalls, "the retry must never create a second order")
}

// billingSeedForeignOrder inserts a REAL PaymentOrder row owned by a
// DIFFERENT user directly through the harness's own entClient -- the "real
// second user's order in the same database" the brief's Step 5 calls for,
// not a fake standing in for one.
// billingCreateUser inserts a REAL dbent.User row and returns its
// DB-assigned id. PaymentOrder.user_id is a REQUIRED FK edge to User
// (ent/schema/payment_order.go:176-183), so seeding an order needs an actual
// owning row to exist -- ent does not support client-chosen ids on this
// schema (no SetID on UserCreate), so the id used everywhere below is
// whatever the DB assigns, never the billingRouteUserID constant.
func billingCreateUser(t *testing.T, env *billingRouteEnv) int64 {
	t.Helper()
	u, err := env.db.User.Create().
		SetEmail(fmt.Sprintf("billing-route-test-%d@example.com", time.Now().UnixNano())).
		SetPasswordHash("test-hash").
		Save(context.Background())
	require.NoError(t, err)
	return int64(u.ID)
}

// billingSeedOrder inserts a REAL PaymentOrder row owned by ownerUserID (a
// REAL User row, per billingCreateUser above -- not a fake standing in for
// one) and returns its id.
func billingSeedOrder(t *testing.T, env *billingRouteEnv, ownerUserID int64, status string, amount float64) int64 {
	t.Helper()
	o, err := env.db.PaymentOrder.Create().
		SetUserID(ownerUserID).
		SetUserEmail("other-user@example.com").
		SetUserName("other-user").
		SetAmount(amount).
		SetPayAmount(amount).
		SetRechargeCode(fmt.Sprintf("PAY-TEST-%d-%d", ownerUserID, time.Now().UnixNano())).
		SetOutTradeNo(fmt.Sprintf("sub2_test_%d_%d", ownerUserID, time.Now().UnixNano())).
		SetPaymentType("razorpay").
		SetPaymentTradeNo("").
		SetOrderType("balance").
		SetStatus(status).
		SetClientIP("127.0.0.1").
		SetSrcHost("agent.example.com").
		SetExpiresAt(time.Now().Add(time.Hour)).
		Save(context.Background())
	require.NoError(t, err)
	return int64(o.ID)
}

// TestChargeStatusRouteForeignOrderIsPendingNeverLeaksTheOwnersData is the
// SECURITY-CRITICAL case the brief names explicitly: user 8's REAL,
// completed, $500 order lives in the SAME database env's token holder (user
// 7, billingRouteUserID) is authenticated as. GET /charge/{that-id} must
// answer {status:"pending"} -- never 404 (a cross-tenant existence oracle),
// never 403, never the real amount or status.
func TestChargeStatusRouteForeignOrderIsPendingNeverLeaksTheOwnersData(t *testing.T) {
	env := newBillingRouteEnv(t)
	callerID := billingCreateUser(t, env)
	foreignID := billingCreateUser(t, env)
	require.NotEqual(t, callerID, foreignID)
	foreignOrderID := billingSeedOrder(t, env, foreignID, service.OrderStatusCompleted, 500)
	tok := "Bearer " + env.mintForUser(t, callerID, service.ScopeBillingManage)

	rec := env.getPath(t, fmt.Sprintf("/api/billing/charge/%d", foreignOrderID), tok)

	require.Equal(t, http.StatusOK, rec.Code, "must NOT be 404 -- a 404 here is a cross-tenant existence oracle; body: %s", rec.Body.String())
	require.JSONEq(t, `{"status":"pending"}`, rec.Body.String(),
		"a foreign order must render exactly like a real pending charge -- no amount, no reason, no hint it exists")
}

// TestChargeStatusRouteUnknownIdIsPendingNeverAnError mirrors the above for
// an id with no backing row at all.
func TestChargeStatusRouteUnknownIdIsPendingNeverAnError(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.getPath(t, "/api/billing/charge/999999999", tok)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.JSONEq(t, `{"status":"pending"}`, rec.Body.String())
}

// TestChargeStatusRouteOwnCompletedOrderIsSettledWithAmount is the positive
// case alongside the two negatives above: the CALLER's own completed order
// (billingRouteUserID == 7) DOES resolve to real data, so the "always
// pending" cases above are proven to be about ownership, not a handler that
// never reports anything real.
func TestChargeStatusRouteOwnCompletedOrderIsSettledWithAmount(t *testing.T) {
	env := newBillingRouteEnv(t)
	callerID := billingCreateUser(t, env)
	ownOrderID := billingSeedOrder(t, env, callerID, service.OrderStatusCompleted, 19.99)
	tok := "Bearer " + env.mintForUser(t, callerID, service.ScopeBillingManage)

	rec := env.getPath(t, fmt.Sprintf("/api/billing/charge/%d", ownOrderID), tok)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.JSONEq(t, `{"status":"settled","amountUsd":"19.99"}`, rec.Body.String())
}

// ---------------------------------------------------------------------------
// The 5 honest refusals -- 501, or 200 {"effect":"blocked"} for preview.
// Each asserted to be NOT 404 (would read as "portal unreachable", a worse
// lie -- ruling R-5.1) and NOT a fake success.
// ---------------------------------------------------------------------------

func TestSubscriptionPreviewRouteReturns200Blocked(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPost, "/api/billing/subscription/preview", tok, `{"subscriptionTypeId":"pro"}`, nil)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "blocked", body["effect"])
	require.NotEmpty(t, body["reason"])
}

func TestSubscriptionUpgradeRouteReturns501NeverAFakeStripeStatus(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPost, "/api/billing/subscription/upgrade", tok, `{"subscriptionTypeId":"pro"}`, map[string]string{"Idempotency-Key": "upgrade-key"})

	require.Equal(t, http.StatusNotImplemented, rec.Code, "body: %s", rec.Body.String())
	require.NotEqual(t, http.StatusNotFound, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.NotEqual(t, "requires_action", body["error"])
	require.NotEqual(t, "payment_failed", body["error"])
	require.NotEqual(t, "upgraded", body["status"])
	require.NotEqual(t, "already_on_tier", body["status"])
}

func TestPendingChangePutRouteReturns501(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPut, "/api/billing/subscription/pending-change", tok, `{"type":"cancellation"}`, nil)

	require.Equal(t, http.StatusNotImplemented, rec.Code, "body: %s", rec.Body.String())
	require.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestPendingChangeDeleteRouteReturns501(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodDelete, "/api/billing/subscription/pending-change", tok, "", nil)

	require.Equal(t, http.StatusNotImplemented, rec.Code, "body: %s", rec.Body.String())
	require.NotEqual(t, http.StatusNotFound, rec.Code)
}

func TestAutoTopUpRouteReturns501(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	rec := env.do(t, http.MethodPatch, "/api/billing/auto-top-up", tok, `{"enabled":true,"threshold":10,"topUpAmount":20}`, nil)

	require.Equal(t, http.StatusNotImplemented, rec.Code, "body: %s", rec.Body.String())
	require.NotEqual(t, http.StatusNotFound, rec.Code)
}

// TestBillingWriteRoutesAreNotMountedUnderAPIV1 mirrors the read-side pin:
// the client hardcodes /api/billing/* (no /api/v1 prefix).
func TestBillingWriteRoutesAreNotMountedUnderAPIV1(t *testing.T) {
	env := newBillingRouteEnv(t)
	tok := "Bearer " + env.mint(t, service.ScopeBillingManage)

	for _, w := range billingWrites() {
		t.Run(w.name, func(t *testing.T) {
			rec := env.do(t, w.method, "/api/v1"+w.path, tok, w.body, w.headers)
			require.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}
