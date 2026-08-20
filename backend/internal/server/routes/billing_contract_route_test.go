package routes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/handler"
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
type stubBillingPlan struct{}

func (stubBillingPlan) ListPlans(context.Context) ([]*dbent.SubscriptionPlan, error) {
	return []*dbent.SubscriptionPlan{
		{ID: 100, GroupID: 9, Name: "Pro", Price: 20, SortOrder: 1, ForSale: true},
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

// --- harness --------------------------------------------------------------

type billingRouteEnv struct {
	router *gin.Engine
	keySvc *service.OAuthKeyService
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

	billingSvc := service.NewBillingContractService(
		stubBillingBalance{balance: 42.50},
		stubBillingOrg{},
		stubBillingUsage{actualCost: 1.25},
		stubBillingPayment{},
		planSrc,
		subSrc,
		billingRouteIssuer,
	)

	r := gin.New()
	RegisterBillingContractRoutes(r, oauthH, handler.NewBillingContractHandler(billingSvc))
	return &billingRouteEnv{router: r, keySvc: keySvc}
}

func (e *billingRouteEnv) mint(t *testing.T, scope string) string {
	t.Helper()
	key, err := e.keySvc.Active(context.Background())
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   billingRouteIssuer,
		"sub":   strconv.FormatInt(billingRouteUserID, 10),
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
	require.Equal(t, "20", tier["dollarsPerMonthDisplay"])
	require.Equal(t, "100", tier["monthlyCredits"])
	require.Equal(t, true, tier["isCurrent"])
	require.Equal(t, true, tier["isEnabled"])

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
	require.Equal(t, "15", tier["dollarsPerMonthDisplay"], "$180/year normalised to $15/month, not the raw $180")
}
