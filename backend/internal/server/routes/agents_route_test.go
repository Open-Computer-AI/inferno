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
	"github.com/Wei-Shaw/sub2api/ent/orgmember"
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
// AgentHandler, the real AgentRegistryService and the real OrgService, all
// over a real (sqlite) ent client -- mirroring billing_contract_route_test.go,
// the closest sibling. The point here is the WIRE -- status codes, the
// camelCase / bare-JSON shape and the org-selection branching -- not any SQL
// behaviour, which agent_registry_test.go and org_service already cover.

const (
	agentRouteIssuer   = "https://portal.example.com"
	agentRouteClientID = "hermes-cli"
	agentRouteUserID   = int64(7)
)

// agentRouteSeq gives every seeded agent a distinct, short public_id --
// public_id is capped at 64 chars (ent/schema/agent.go) and globally
// unique, so a t.Name()+timestamp id is both too long and unnecessary: each
// test already runs against its own isolated in-memory database.
var agentRouteSeq int

// noopCronArmer satisfies service.AgentCronService's unexported cronArmer
// interface (ruling T5-1) for these wire/routing tests: this file asserts
// status codes, JSON shape and org-selection branching, never the timing
// wheel's real arming behavior, which internal/service/agent_cron_test.go
// and agent_cron_firer_test.go already cover directly. Deliberately a
// silent no-op rather than a real *service.AgentCronFirer -- standing one
// of those up here would need a real OAuthKeyService key, a running timing
// wheel and an HTTP client that none of these tests exercise or assert on.
type noopCronArmer struct{}

func (noopCronArmer) ArmNow(context.Context, int64, time.Time) {}

// --- harness ---------------------------------------------------------------

type agentRouteEnv struct {
	router *gin.Engine
	keySvc *service.OAuthKeyService
	db     *dbent.Client
}

// newAgentRouteEnv seeds ONE default org ("acme", id 1) owned by
// agentRouteUserID -- the common case, and what makes
// TestAgentsRouteEmitsTheDesktopsExactShape's bare env.seedAgent(7, 1, ...)
// call (no org seeding of its own) work: the org it references already
// exists.
func newAgentRouteEnv(t *testing.T) *agentRouteEnv {
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
		SetClientID(agentRouteClientID).
		SetKind("FIRST_PARTY").
		SetName("hermes cli").
		SetOwnerUserID(agentRouteUserID).
		SetOrgID(1).
		SetRedirectURIOrigin("https://agent.example.com").
		SetStatus(service.ClientActive).
		Save(context.Background())
	require.NoError(t, err)

	env := &agentRouteEnv{db: entClient}
	// "default-org", not "acme": tests call seedOrgs with their own slugs
	// (often "acme") to REPLACE this membership, and the org slug index is
	// unique, so this default must never collide with a slug a test picks.
	env.seedOrgs(t, agentRouteUserID, "default-org")

	keySvc := service.NewOAuthKeyService(entClient)
	clientSvc := service.NewOAuthClientService(entClient)
	tokenSvc := service.NewOAuthTokenService(entClient, keySvc, nil, nil, nil, agentRouteIssuer)
	orgSvc := service.NewOrgService(entClient)
	agentSvc := service.NewAgentRegistryService(entClient)
	agentCronSvc := service.NewAgentCronService(entClient, noopCronArmer{})

	oauthH := handler.NewOAuthHandler(keySvc, clientSvc, orgSvc, nil, tokenSvc, nil)
	agentH := handler.NewAgentHandler(agentSvc, orgSvc, agentCronSvc)

	r := gin.New()
	RegisterAgentRoutes(r, oauthH, agentH)

	env.router = r
	env.keySvc = keySvc
	return env
}

// seedAgent creates one agent row directly (mirroring
// agent_registry_test.go's fixture helper of the same name), with a
// public_id unique across the whole fixture.
func (e *agentRouteEnv) seedAgent(t *testing.T, userID, orgID int64, name string) *dbent.Agent {
	t.Helper()
	agentRouteSeq++
	row, err := e.db.Agent.Create().
		SetPublicID(fmt.Sprintf("agent:test:%d", agentRouteSeq)).
		SetUserID(userID).
		SetOrgID(orgID).
		SetName(name).
		SetLastSeenAt(time.Now()).
		Save(context.Background())
	require.NoError(t, err)
	return row
}

// seedOrgs REPLACES userID's org memberships with exactly the given slugs --
// dropping any earlier membership (including newAgentRouteEnv's default
// "acme" one) so a test asserting an exact org count is never thrown off by
// the harness's own setup. Each new org is a distinct row (a fresh
// autoincrement id), so the multi-org 409 test genuinely proves the picker
// lists two DIFFERENT orgs, not the same row twice.
func (e *agentRouteEnv) seedOrgs(t *testing.T, userID int64, slugs ...string) []*dbent.Org {
	t.Helper()
	ctx := context.Background()

	_, err := e.db.OrgMember.Delete().Where(orgmember.UserIDEQ(userID)).Exec(ctx)
	require.NoError(t, err)

	out := make([]*dbent.Org, 0, len(slugs))
	for _, slug := range slugs {
		o, err := e.db.Org.Create().
			SetSlug(slug).
			SetName(strings.ToUpper(slug[:1]) + slug[1:]).
			Save(ctx)
		require.NoError(t, err)

		_, err = e.db.OrgMember.Create().
			SetOrgID(o.ID).
			SetUserID(userID).
			SetRole(service.OrgRoleOwner).
			Save(ctx)
		require.NoError(t, err)
		out = append(out, o)
	}
	return out
}

func (e *agentRouteEnv) mint(t *testing.T, scope string) string {
	t.Helper()
	return e.mintForUser(t, agentRouteUserID, scope)
}

func (e *agentRouteEnv) mintForUser(t *testing.T, userID int64, scope string) string {
	t.Helper()
	return e.mintForClient(t, userID, agentRouteClientID, scope)
}

// mintForClient mints a token whose `aud` names clientID -- the
// /api/agent-cron/* routes resolve the CALLING agent from this claim
// (agent_handler.go's agentCronHandlerAgentRowID), not from a request-body
// field, so a agent-cron test needs a token whose aud is the agent's own
// public_id, not the shared "hermes-cli" desktop client agentRouteClientID
// every other route test in this file uses.
func (e *agentRouteEnv) mintForClient(t *testing.T, userID int64, clientID, scope string) string {
	t.Helper()
	key, err := e.keySvc.Active(context.Background())
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":   agentRouteIssuer,
		"sub":   strconv.FormatInt(userID, 10),
		"aud":   clientID,
		"scope": scope,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tok.Header["kid"] = key.Kid
	signed, err := tok.SignedString(key.Private)
	require.NoError(t, err)
	return signed
}

// registerAgentClient creates BOTH the pieces /api/agent-cron/* needs to
// identify a calling agent: an OAuthClient whose client_id is the agent's
// own public_id (so a token minted with mintForClient(t, userID, publicID,
// ...) verifies), and the matching Agent row (so
// AgentRegistryService.ResolveOwnedAgentRowID finds it) -- owned by the
// same userID and orgID so the ownership check in agent_registry.go passes.
func (e *agentRouteEnv) registerAgentClient(t *testing.T, userID, orgID int64, publicID string) *dbent.Agent {
	t.Helper()
	ctx := context.Background()

	_, err := e.db.OAuthClient.Create().
		SetClientID(publicID).
		SetKind("FIRST_PARTY").
		SetName(publicID).
		SetOwnerUserID(userID).
		SetOrgID(orgID).
		SetRedirectURIOrigin("https://agent.example.com").
		SetStatus(service.ClientActive).
		Save(ctx)
	require.NoError(t, err)

	row, err := e.db.Agent.Create().
		SetPublicID(publicID).
		SetUserID(userID).
		SetOrgID(orgID).
		SetName(publicID).
		SetLastSeenAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
	return row
}

func (e *agentRouteEnv) get(t *testing.T, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e *agentRouteEnv) do(t *testing.T, method, path, authorization, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// --- the wire contract ------------------------------------------------------

// TestAgentsRouteEmitsTheDesktopsExactShape pins the exact wire shape
// task-3-brief.md's Step 1 specifies, cited to
// apps/desktop/electron/main.ts:7918-7928.
func TestAgentsRouteEmitsTheDesktopsExactShape(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.seedAgent(t, 7, 1, "box-1")

	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	raw := rec.Body.String()
	// The desktop DROPS any agent whose id is not a string (main.ts:7922),
	// so a numeric id removes the agent from the user's list with no error.
	require.Contains(t, raw, `"id":"`, "id must be a JSON string, never a number")
	require.NotContains(t, raw, `"code":`, "bare JSON, never the panel envelope")

	var body struct {
		Agents []map[string]any `json:"agents"`
		Org    map[string]any   `json:"org"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	require.Len(t, body.Agents, 1)
	for _, k := range []string{"id", "name", "status", "dashboardUrl", "dashboardGatewayState"} {
		require.Contains(t, body.Agents[0], k, "key read by main.ts:7923-7927")
	}
	require.Contains(t, body.Org, "slug")
}

// TestAgentsRouteAnswers409WithTheOrgListWhenMultiOrgAndNoOrgParam:
// main.ts:7849 treats 409 as "multi-org user has not picked an org" and
// reads the org list out of the error body to render a picker.
func TestAgentsRouteAnswers409WithTheOrgListWhenMultiOrgAndNoOrgParam(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.seedOrgs(t, 7, "acme", "globex") // user 7 belongs to TWO orgs

	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusConflict, rec.Code)

	var body struct {
		Error string           `json:"error"`
		Orgs  []map[string]any `json:"orgs"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "org_selection_required", body.Error)
	require.Len(t, body.Orgs, 2, "the picker needs the list, or the user is stuck")
}

// TestAgentsRouteHonorsExplicitOrgQueryParam is ruling T3-2: a user in two
// orgs passes ?org=<slug> for one they ARE a member of. agent_handler.go's
// resolveOrg (:122-130) must scope the response to THAT org, not silently
// fall back to the caller's first membership -- Task 2's ListForUser now
// filters on org_id as well as user_id (ruling T2-1), so this parameter is
// what decides what a multi-org user sees.
//
// An agent is seeded in EACH org so the assertion can actually distinguish
// them: a single-agent fixture would pass this test even with no org
// filtering at all.
func TestAgentsRouteHonorsExplicitOrgQueryParam(t *testing.T) {
	env := newAgentRouteEnv(t)
	orgs := env.seedOrgs(t, 7, "acme", "globex") // user 7 belongs to BOTH
	env.seedAgent(t, 7, orgs[0].ID, "acme-agent")
	env.seedAgent(t, 7, orgs[1].ID, "globex-agent")

	rec := env.get(t, "/api/agents?org=acme", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	var body struct {
		Agents []map[string]any `json:"agents"`
		Org    map[string]any   `json:"org"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "acme", body.Org["slug"])
	require.Len(t, body.Agents, 1, "?org=acme must return ONLY acme's agents")
	require.Equal(t, "acme-agent", body.Agents[0]["name"])
}

// TestAgentsRouteRejectsOrgQueryParamForNonMemberOrg is ruling T3-2's other
// direction: a user passes ?org=<slug> for an org they are NOT a member of.
// The failure mode being guarded is a silent fallback to an org the caller
// DOES belong to -- an unrecognized slug must 403, not quietly substitute
// another org and leak its agents -- so this asserts both the status code
// AND that the caller's own org's agent never appears in the response.
func TestAgentsRouteRejectsOrgQueryParamForNonMemberOrg(t *testing.T) {
	env := newAgentRouteEnv(t) // default env: user 7 belongs only to "default-org" (id 1)
	env.seedAgent(t, 7, 1, "my-own-agent")

	rec := env.get(t, "/api/agents?org=not-a-member-org", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	require.JSONEq(t, `{"error":"org_not_found"}`, rec.Body.String(),
		"must be a clean 403, not the caller's own org's agents silently substituted in")
	require.NotContains(t, rec.Body.String(), "my-own-agent",
		"a non-member ?org= must never fall back to leaking the caller's own org")
}

// TestAgentsRouteAdmitsASingleOrgUserWithoutAPicker: a single-org user must
// NOT get a 409 -- most users belong only to their personal org, and the
// desktop must not force a picker on every single-org user.
func TestAgentsRouteAdmitsASingleOrgUserWithoutAPicker(t *testing.T) {
	env := newAgentRouteEnv(t) // default env is already single-org ("default-org")

	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusOK, rec.Code, "a single-org user must not see the multi-org picker; body: %s", rec.Body.String())
}

// TestAgentsRouteRejectsAnUnauthenticatedCall: no token -> 401.
func TestAgentsRouteRejectsAnUnauthenticatedCall(t *testing.T) {
	env := newAgentRouteEnv(t)
	rec := env.get(t, "/api/agents", "")

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestAgentsRouteAdmitsAnyValidScope proves the endpoint does NOT gate on
// agents:read or agents:manage (Global Constraints) -- a valid token with a
// scope that names neither must still be admitted, exactly like a real
// hermes login (which only ever holds inference:invoke).
//
// MUTATION CHECK: requiring agents:read/agents:manage on this route fails
// this test at 403 vs 200 -- the exact defect three billing:read endpoints
// shipped with on the previous branch.
func TestAgentsRouteAdmitsAnyValidScope(t *testing.T) {
	env := newAgentRouteEnv(t)
	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))

	require.Equal(t, http.StatusOK, rec.Code,
		"a token carrying only the scope a real client requests must be admitted; body: %s", rec.Body.String())
}

// TestAgentsRouteAdmitsATokenWithNoScopeAtAll: RFC 6749 §3.3 makes scope
// optional, so a token minted from a scope-less request carries an empty
// claim. It is still a verified statement about who the holder is.
func TestAgentsRouteAdmitsATokenWithNoScopeAtAll(t *testing.T) {
	env := newAgentRouteEnv(t)
	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, ""))

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
}

// ===========================================================================
// POST /api/agents/register
// ===========================================================================

// TestAgentsRegisterRouteRegistersAndEmitsTheAgentShape is the round trip:
// register, then confirm the response carries the same camelCase agent
// shape GET /api/agents does.
func TestAgentsRegisterRouteRegistersAndEmitsTheAgentShape(t *testing.T) {
	env := newAgentRouteEnv(t)

	rec := env.do(t, http.MethodPost, "/api/agents/register",
		"Bearer "+env.mint(t, service.ScopeInferenceInvoke),
		`{"public_id":"agent:abc","name":"box-1","dashboard_url":"https://dash.example.com/abc"}`)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	raw := rec.Body.String()
	require.NotContains(t, raw, `"code":`, "bare JSON, never the panel envelope")

	var body struct {
		Agent map[string]any `json:"agent"`
		Org   map[string]any `json:"org"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	require.Equal(t, "agent:abc", body.Agent["id"])
	require.IsType(t, "", body.Agent["id"], "id must be a JSON string")
	require.Equal(t, "box-1", body.Agent["name"])
	require.Contains(t, body.Org, "slug")
}

// TestAgentsRegisterRouteRejectsHijackingAnotherUsersPublicID surfaces
// AGENT_PUBLIC_ID_OWNED_BY_ANOTHER_USER as 403, never 500 -- ruling T2-2.
func TestAgentsRegisterRouteRejectsHijackingAnotherUsersPublicID(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.seedOrgs(t, 8, "someone-elses-org")

	// User 8 registers "agent:shared" first.
	rec := env.do(t, http.MethodPost, "/api/agents/register",
		"Bearer "+env.mintForUser(t, 8, service.ScopeInferenceInvoke),
		`{"public_id":"agent:shared","name":"first-owner"}`)
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

	// User 7 (this fixture's default caller) tries to claim the same
	// public_id.
	rec = env.do(t, http.MethodPost, "/api/agents/register",
		"Bearer "+env.mint(t, service.ScopeInferenceInvoke),
		`{"public_id":"agent:shared","name":"hijacker"}`)
	require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
}

// TestAgentsRegisterRouteRejectsAnUnauthenticatedCall mirrors GET's:
// dropping the SCOPE requirement must not drop AUTHENTICATION.
func TestAgentsRegisterRouteRejectsAnUnauthenticatedCall(t *testing.T) {
	env := newAgentRouteEnv(t)
	rec := env.do(t, http.MethodPost, "/api/agents/register", "", `{"public_id":"agent:abc"}`)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":"invalid_token"}`, rec.Body.String())
}

// TestAgentsRouteIsNotMountedUnderAPIV1 pins the mount point, mirroring
// billing_contract_route_test.go's equivalent pin: the desktop hardcodes
// /api/agents, not the panel's versioned prefix.
func TestAgentsRouteIsNotMountedUnderAPIV1(t *testing.T) {
	env := newAgentRouteEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
