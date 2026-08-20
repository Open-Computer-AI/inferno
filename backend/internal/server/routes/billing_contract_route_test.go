package routes

import (
	"context"
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
// handler and the real BillingContractService. Only the four data sources the
// service composes are faked, because the point here is the WIRE — status
// codes, scope enforcement and the exact JSON keys — not the numbers.
//
// The ent client is sqlite purely to hold the signing key and one client row;
// nothing under test is a SQL behaviour.

const (
	billingRouteIssuer   = "https://portal.example.com"
	billingRouteClientID = "hermes-cli"
	billingRouteUserID   = int64(7)
)

// --- fakes for the four sources ------------------------------------------

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

// --- harness --------------------------------------------------------------

type billingRouteEnv struct {
	router *gin.Engine
	keySvc *service.OAuthKeyService
}

func newBillingRouteEnv(t *testing.T) *billingRouteEnv {
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
	req := httptest.NewRequest(http.MethodGet, "/api/billing/state", nil)
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

	require.Equal(t, "https://portal.example.com/purchase", body["portalUrl"])
}

// TestBillingStateRouteRejectsATokenWithoutBillingRead is the scope boundary.
// The token below is exactly what a shipping hermes client holds after a
// normal login (hermes_cli/auth.py's DEFAULT_NOUS_SCOPE) — see
// task-1-report.md F2, which escalates that this makes the endpoint
// unreachable by an unmodified client.
func TestBillingStateRouteRejectsATokenWithoutBillingRead(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.JSONEq(t, `{"error":"insufficient_scope"}`, rec.Body.String())
}

// TestBillingStateRouteRejectsAnUnauthenticatedCall proves the route is gated
// by RequireOAuthScope at all: a 404 here would mean the group never mounted,
// and a 200 would mean the money of an arbitrary user is public.
func TestBillingStateRouteRejectsAnUnauthenticatedCall(t *testing.T) {
	env := newBillingRouteEnv(t)
	rec := env.get(t, "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
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
