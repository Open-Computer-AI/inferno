# Agent Registry + Chronos Cron Signer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Inferno answer the two portal surfaces Hermes already speaks — the desktop's cloud agent list, and the Chronos one-shot scheduler that gives an agent unattended work.

**Architecture:** Two ent tables (`agents`, `agent_cron_fires`) and six endpoints, all authenticated with the OAuth token Inferno already issues. Agents self-register at boot; `status` comes from a `last_seen_at` heartbeat, not a cross-service call. Fires are rows in the database, timed by the existing in-memory `TimingWheelService` and rehydrated from those rows on boot — the table is the truth, the wheel is a cache.

**Tech Stack:** Go 1.26, Gin, ent (codegen ORM), `golang-jwt/jwt/v5` (RS256, reusing sub-project #4's `OAuthKeyService`), PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-08-22-agent-registry-and-chronos-design.md`

## Global Constraints

- **The gate is `cd backend && go test -tags unit ./...`** — the FULL tagged run, never a package subset. 398 of 1132 test files carry `//go:build unit`; a subset silently skips them.
- **Undeclared divergence must stay at exactly 9.** Script: `./inferno-frontend/scripts/check-divergence.sh` from the repo root (exits 1 at that baseline — expected). Declare touched files in BOTH its `DECLARED` list and `GOAL.md`. **`DECLARED` is a double-quoted shell string — backticks and `$(` inside it EXECUTE.** The baseline is 9, NOT the 19 used on the billing branch; the merge declared the Razorpay set.
- **`TestFilterGrokFreeQuotaAccounts*` is a known pre-existing upstream flake.** Re-run that package; never "fix" it.
- **Bare JSON on every endpoint.** Never the panel's `{code,message,data}` envelope.
- **Valid token, no particular scope.** `agents:read`/`agents:manage` exist in the vocabulary and NO client requests either — verified against `hermes_cli/auth.py` and `plugins/cron_providers/chronos/`. Gating on them repeats the `billing:read` defect exactly.
- **No key, method, or status ships without a `file:line` citation** to the client that reads it. If it can't be cited, report it as a question instead of emitting it.
- **The client fails open.** `trimCloudAgents` drops an agent whose `id` is not a string rather than erroring. Assert against encoded JSON bytes wherever the wire type matters.
- **Mutations must COMPILE.** A mutation that breaks the build proves nothing.
- Run `go generate ./ent` after any schema change; commit the generated files.

---

### Task 1: Schema and migrations

**Files:**
- Create: `backend/ent/schema/agent.go`, `backend/ent/schema/agent_cron_fire.go`
- Create: `backend/migrations/911_agents.sql`, `backend/migrations/912_agent_cron_fires.sql`
- Modify: `inferno-frontend/scripts/check-divergence.sh`, `GOAL.md`

**Interfaces:**
- Produces: ent types `dbent.Agent` and `dbent.AgentCronFire`, tables `agents` and `agent_cron_fires`.

- [ ] **Step 1: Write the Agent schema**

Follow `backend/ent/schema/oauth_device_authorization.go` exactly for style — `entsql.Annotation{Table: ...}`, `mixins.TimeMixin{}`.

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Agent is one registered Hermes agent. Inferno owns IDENTITY only -- which
// user and org an agent belongs to, what to call it, and where its dashboard
// lives. VM lifecycle stays in oc-platform; oc_platform_user_id is the link
// between Inferno's int64 users and oc-platform's UUID users, which exists
// nowhere else.
type Agent struct {
	ent.Schema
}

func (Agent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agents"}}
}

func (Agent) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (Agent) Fields() []ent.Field {
	return []ent.Field{
		// public_id is what GET /api/agents emits as `id`. The desktop
		// requires a STRING (apps/desktop/electron/main.ts:7922 drops any
		// agent whose id is not one), so this is never the int64 row id.
		field.String("public_id").MaxLen(64).NotEmpty().Unique(),
		field.Int64("user_id"),
		field.Int64("org_id"),
		field.String("name").MaxLen(200).NotEmpty(),
		field.String("dashboard_url").MaxLen(500).Default(""),
		field.String("oc_platform_user_id").MaxLen(64).Optional(),
		field.Time("last_seen_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (Agent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("org_id"),
	}
}
```

- [ ] **Step 2: Write the AgentCronFire schema**

```go
package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// AgentCronFire is ONE armed one-shot. Chronos is not a cron engine: the agent
// arms a single fire_at and re-arms after each fire, so there is no recurrence
// rule here by design.
//
// This table is the TRUTH. TimingWheelService is in-memory, so a restart drops
// every pending timer; rows are rehydrated into the wheel on boot. Nothing
// errors when that is broken -- the work simply never happens.
type AgentCronFire struct {
	ent.Schema
}

func (AgentCronFire) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "agent_cron_fires"}}
}

func (AgentCronFire) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (AgentCronFire) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("agent_row_id"),
		field.String("job_id").MaxLen(200).NotEmpty(),
		field.Time("fire_at"),
		field.String("callback_url").MaxLen(500).NotEmpty(),
		// dedup_key is "{job_id}:{fire_at}" (plugins/cron_providers/chronos/
		// _nas_client.py:96-109). UNIQUE is what makes re-arming idempotent AT
		// THE DATABASE rather than by a read-then-write that races itself --
		// the agent's cold-start reconcile re-arms everything it wants.
		field.String("dedup_key").MaxLen(300).NotEmpty().Unique(),
		field.String("schedule_id").MaxLen(64).NotEmpty(),
		field.Enum("state").Values("armed", "fired", "cancelled").Default("armed"),
		field.Int("attempts").Default(0),
		field.String("last_error").MaxLen(500).Default(""),
	}
}

func (AgentCronFire) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("agent_row_id"),
		index.Fields("state", "fire_at"),
	}
}
```

- [ ] **Step 3: Run codegen**

Run: `cd backend && go generate ./ent && go build ./...`
Expected: new files under `backend/ent/agent*.go` and `backend/ent/agentcronfire*.go`; build clean.

- [ ] **Step 4: Write the migrations**

`backend/migrations/911_agents.sql`:

```sql
CREATE TABLE IF NOT EXISTS agents (
    id                  BIGSERIAL PRIMARY KEY,
    public_id           VARCHAR(64)  NOT NULL UNIQUE,
    user_id             BIGINT       NOT NULL,
    org_id              BIGINT       NOT NULL,
    name                VARCHAR(200) NOT NULL,
    dashboard_url       VARCHAR(500) NOT NULL DEFAULT '',
    oc_platform_user_id VARCHAR(64),
    last_seen_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agents_user_id_idx ON agents (user_id);
CREATE INDEX IF NOT EXISTS agents_org_id_idx  ON agents (org_id);
```

`backend/migrations/912_agent_cron_fires.sql`:

```sql
CREATE TABLE IF NOT EXISTS agent_cron_fires (
    id           BIGSERIAL PRIMARY KEY,
    agent_row_id BIGINT       NOT NULL,
    job_id       VARCHAR(200) NOT NULL,
    fire_at      TIMESTAMPTZ  NOT NULL,
    callback_url VARCHAR(500) NOT NULL,
    dedup_key    VARCHAR(300) NOT NULL UNIQUE,
    schedule_id  VARCHAR(64)  NOT NULL,
    state        VARCHAR(16)  NOT NULL DEFAULT 'armed',
    attempts     INTEGER      NOT NULL DEFAULT 0,
    last_error   VARCHAR(500) NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_cron_fires_agent_idx ON agent_cron_fires (agent_row_id);
CREATE INDEX IF NOT EXISTS agent_cron_fires_due_idx   ON agent_cron_fires (state, fire_at);
```

- [ ] **Step 5: Declare every touched file**

Add each new/modified path to `check-divergence.sh`'s `DECLARED` (plain unquoted paths — backticks EXECUTE in that string) and a `D8` row to `GOAL.md`. Then run `./inferno-frontend/scripts/check-divergence.sh` from the repo root and confirm **exactly 9** undeclared.

- [ ] **Step 6: Gates and commit**

Run: `cd backend && go test -tags unit ./...` → exit 0.

---

### Task 2: `AgentRegistryService` — register and list

**Files:**
- Create: `backend/internal/service/agent_registry.go`, `backend/internal/service/agent_registry_test.go`

**Interfaces:**
- Consumes: `dbent.Agent` from Task 1.
- Produces:
  - `func (s *AgentRegistryService) Register(ctx context.Context, userID, orgID int64, in RegisterAgentInput) (*AgentView, error)`
  - `func (s *AgentRegistryService) ListForUser(ctx context.Context, userID, orgID int64) ([]AgentView, error)`
  - `type AgentView struct { ID, Name, Status, DashboardURL, DashboardGatewayState string }`

- [ ] **Step 1: Write the isolation test FIRST — it is the important one**

```go
func TestListForUserReturnsOnlyTheCallersAgents(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.seedAgent(7, 1, "ours")
	fx.seedAgent(8, 2, "someone else's") // a SECOND real user, same database

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ours", got[0].Name)
}
```

A second user's agent must be **present in the database**, or this passes against an implementation with no filter at all.

- [ ] **Step 2: Run it, watch it fail.** `cd backend && go test -tags unit -run TestListForUser ./internal/service/`

- [ ] **Step 3: Write the status test**

`status` and `dashboardGatewayState` are free-form display strings rendered verbatim (`i18n/en.ts:812` → `cloudStatusLabel: status => "Status: ${status}"`), so they must read as prose, not as enum tokens.

```go
func TestStatusIsHumanReadableProseDerivedFromHeartbeat(t *testing.T) {
	svc, fx := newAgentRegistryFixture(t)
	fx.now = time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	fx.seedAgentSeenAt(7, 1, "fresh", fx.now.Add(-30*time.Second))
	fx.seedAgentSeenAt(7, 1, "stale", fx.now.Add(-3*time.Hour))
	fx.seedAgentSeenAt(7, 1, "never", time.Time{})

	got, err := svc.ListForUser(context.Background(), 7, 1)
	require.NoError(t, err)
	byName := map[string]AgentView{}
	for _, a := range got {
		byName[a.Name] = a
	}
	require.Equal(t, "online", byName["fresh"].Status)
	require.Equal(t, "last seen 3h ago", byName["stale"].Status)
	require.Equal(t, "never connected", byName["never"].Status)
}
```

- [ ] **Step 4: Implement `Register` and `ListForUser`.**

`Register` is an UPSERT keyed on `(user_id, public_id)` so a rebooting agent heartbeats rather than duplicating itself. It sets `last_seen_at = now()` on every call. Revoked agents (`revoked_at IS NOT NULL`) are excluded from `ListForUser`.

- [ ] **Step 5: Run, pass.**

- [ ] **Step 6: Mutation-prove the filter**

Drop the `user_id` predicate from `ListForUser` in a way that still COMPILES, run Step 1's test, and confirm it fails with 2 agents instead of 1. Revert.

- [ ] **Step 7: Gates and commit.**

---

### Task 3: `POST /api/agents/register` and `GET /api/agents`

**Files:**
- Create: `backend/internal/handler/agent_handler.go`, `backend/internal/handler/agent_handler_test.go`
- Create: `backend/internal/server/routes/agents.go`, `backend/internal/server/routes/agents_route_test.go`
- Modify: `backend/internal/handler/wire.go`, `backend/internal/service/wire.go`, `backend/internal/server/router.go`, `backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes: `AgentRegistryService.Register` / `.ListForUser` from Task 2.
- Produces: the two routes; response JSON `{agents:[...], org:{...}}`.

- [ ] **Step 1: Write the wire-shape test FIRST**

Keys are camelCase and cited to `apps/desktop/electron/main.ts:7918-7928`.

```go
func TestAgentsRouteEmitsTheDesktopsExactShape(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.seedAgent(7, 1, "box-1")

	rec := env.get(t, "/api/agents", "Bearer "+env.mint(t, service.ScopeInferenceInvoke))
	require.Equal(t, http.StatusOK, rec.Code)

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
```

- [ ] **Step 2: Run, fail. Step 3: Implement handler + routes. Step 4: Run, pass.**

- [ ] **Step 5: The 409 org-selection test**

`main.ts:7849` treats 409 as "multi-org user has not picked an org" and reads the org list out of the error body to render a picker.

```go
func TestAgentsRouteAnswers409WithTheOrgListWhenMultiOrgAndNoOrgParam(t *testing.T) {
	env := newAgentRouteEnv(t)
	env.seedOrgs(7, "acme", "globex") // user 7 belongs to TWO orgs

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
```

A single-org user must NOT get a 409 — add that case too.

- [ ] **Step 6: Auth test** — no token → 401; a valid token with any scope → 200. Do NOT gate on `agents:read` (Global Constraints).

- [ ] **Step 7: Gates and commit.**

---

### Task 4: `/api/agent-cron/{provision,cancel,list}`

**Files:**
- Create: `backend/internal/service/agent_cron.go`, `backend/internal/service/agent_cron_test.go`
- Modify: `backend/internal/handler/agent_handler.go`, `backend/internal/server/routes/agents.go` and their tests

**Interfaces:**
- Consumes: `dbent.AgentCronFire` (Task 1), `AgentRegistryService` (Task 2).
- Produces:
  - `func (s *AgentCronService) Provision(ctx context.Context, agentRowID int64, in ProvisionInput) (*ArmedFireView, error)`
  - `func (s *AgentCronService) Cancel(ctx context.Context, agentRowID int64, jobID string) error`
  - `func (s *AgentCronService) ListArmed(ctx context.Context, agentRowID int64) ([]ArmedFireView, error)`
  - `type ArmedFireView struct { JobID, FireAt, ScheduleID string }`

Request/response keys are cited to `plugins/cron_providers/chronos/_nas_client.py:96-120`.

- [ ] **Step 1: Write the idempotency test FIRST**

The agent's cold-start reconcile re-arms everything it wants, so a duplicate arm is normal traffic, not an error path.

```go
func TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	in := ProvisionInput{
		JobID:            "job-1",
		FireAt:           "2026-09-01T10:00:00Z",
		AgentCallbackURL: "https://agent.example/",
		DedupKey:         "job-1:2026-09-01T10:00:00Z",
	}

	first, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err)
	second, err := svc.Provision(context.Background(), 1, in)
	require.NoError(t, err, "a re-arm is normal reconcile traffic, never an error")

	require.Equal(t, first.ScheduleID, second.ScheduleID, "same fire, same schedule_id")
	require.Equal(t, 1, fx.countFires(), "the UNIQUE dedup_key is what enforces this")
}
```

- [ ] **Step 2: Run, fail. Step 3: Implement. Step 4: Run, pass.**

- [ ] **Step 5: The cross-tenant test**

```go
func TestListArmedReturnsOnlyThisAgentsFires(t *testing.T) {
	svc, fx := newAgentCronFixture(t)
	fx.seedFire(1, "ours")
	fx.seedFire(2, "another agent's") // a SECOND agent, same database

	got, err := svc.ListArmed(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "ours", got[0].JobID)
}
```

Then mutation-prove it: drop the `agent_row_id` predicate (compiling) and show the failure.

- [ ] **Step 6: Cancel test** — cancelling marks `state = cancelled`, and a cancelled fire is absent from `ListArmed`. Cancelling an unknown `job_id` is a no-op returning nil, not an error — the agent cancels optimistically.

- [ ] **Step 7: Gates and commit.**

---

### Task 5: The firer — mint, POST, retry, rehydrate

**Files:**
- Create: `backend/internal/service/agent_cron_firer.go`, `backend/internal/service/agent_cron_firer_test.go`
- Modify: `backend/internal/service/wire.go`, `backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes: `AgentCronService` (Task 4), `OAuthKeyService.Active(ctx) (*SigningKey, error)` (existing), `TimingWheelService.Schedule(name string, delay time.Duration, fn func())` (existing).
- Produces: `func (f *AgentCronFirer) RehydrateOnBoot(ctx context.Context) (int, error)` and `func (f *AgentCronFirer) FireNow(ctx context.Context, fireID int64) error`.

- [ ] **Step 1: Write the JWT-claims test FIRST — this is the security boundary**

Cited to `plugins/cron_providers/chronos/verify.py:95,125-141`. The `purpose` claim is what stops a general agent JWT being replayed against `/api/cron/fire`.

```go
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
```

- [ ] **Step 2: Run, fail. Step 3: Implement `mintFireToken`.**

Model it on `OAuthTokenService.mintAccessToken` (`oauth_token_service.go:219-258`): `keySvc.Active(ctx)`, `jwt.NewWithClaims(jwt.SigningMethodRS256, claims)`, `tok.Header["kid"] = key.Kid`, `tok.SignedString(key.Private)`. Refuse to mint with a blank issuer, exactly as that function does.

- [ ] **Step 4: Run, pass.**

- [ ] **Step 5: Mutation-prove the purpose claim**

Remove `"purpose": "cron_fire"` from the claims map (compiling) and show Step 1's test failing. This is the one mutation that matters most in this plan: without that claim the fire endpoint accepts any agent token.

- [ ] **Step 6: The response-contract tests**

Both rules are from `hermes_cli/web_routers/cron.py`'s own docstring, and both are easy to invert.

```go
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
```

- [ ] **Step 7: The rehydration test — the requirement most likely to ship broken**

`TimingWheelService` is in-memory. Nothing errors when rehydration is missing; the work simply never happens.

```go
func TestRehydrateOnBootRearmsPendingFiresAndSkipsTerminalOnes(t *testing.T) {
	f, fx := newFirerFixture(t)
	fx.seedFire("armed", fx.now.Add(time.Hour))    // must be re-armed
	fx.seedFire("armed", fx.now.Add(-time.Minute)) // overdue while we were down: fires once
	fx.seedFire("fired", fx.now.Add(time.Hour))    // must NOT re-fire
	fx.seedFire("cancelled", fx.now.Add(time.Hour))

	n, err := f.RehydrateOnBoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n, "only armed rows; a fired row must never re-fire on rehydrate")
	require.Equal(t, 2, fx.wheel.scheduled)
}
```

- [ ] **Step 8: Mutation-prove rehydration** — make `RehydrateOnBoot` a no-op returning `(0, nil)` (compiling) and show Step 7's test failing.

- [ ] **Step 9: Wire `RehydrateOnBoot` into server startup**, after the timing wheel starts. Gates and commit.

---

### Task 6: Conformance against a real agent

**Files:**
- Modify: `backend/scripts/oauth-conformance.md`

This task produces evidence, not code. Everything before it tests our *reading* of the client; only this tests the client.

- [ ] **Step 1: Bring up the stack** — the runbook in `backend/scripts/oauth-conformance.md` (Postgres 15932, Redis 16979, `go build -tags embed`, `SERVER_FRONTEND_URL` set). Get a real device-flow token for `hermes-cli`.

- [ ] **Step 2: Register an agent and list it**

```bash
curl -s -X POST $B/api/agents/register -H "Authorization: Bearer $AT" \
  -H 'Content-Type: application/json' \
  -d '{"public_id":"agent:conformance","name":"Conformance Box","dashboard_url":"https://agent.localtest.me"}'
curl -s -H "Authorization: Bearer $AT" $B/api/agents | python3 -m json.tool
```

Assert `id` is a JSON **string**, all five keys present, no `code`/`message`/`data`.

- [ ] **Step 3: Parse the response with the desktop's own projection**

Port `trimCloudAgents` (`main.ts:7918-7929`) verbatim into a throwaway script and confirm the agent survives it. An agent silently dropped here is exactly the failure this task exists to catch.

- [ ] **Step 4: Arm a fire against a local listener and capture the JWT**

Run a listener on `127.0.0.1:19099` that logs the `Authorization` header and returns 200. Provision a fire ~30s out. When it arrives, decode the JWT and assert `purpose=cron_fire`, `aud`, `exp`, `iss`, and that it verifies against `$B/.well-known/jwks.json`.

- [ ] **Step 5: Verify with the agent's OWN verifier**

```bash
"$FORK/.venv/bin/python" -c "
import sys; sys.path.insert(0, '$FORK')
from plugins.cron_providers.chronos.verify import get_fire_verifier
claims = get_fire_verifier()(token=open('/tmp/fire-token.txt').read().strip(),
    expected_audience='agent:conformance',
    jwks_or_key='http://127.0.0.1:18480/.well-known/jwks.json',
    issuer='http://127.0.0.1:18480')
print('VERIFIED' if claims else 'REJECTED', claims)
"
```

`VERIFIED` here is the deliverable. Anything else means the JWT is wrong regardless of what our unit tests say.

- [ ] **Step 6: Restart with a fire armed** — arm one ~2 minutes out, restart Inferno, confirm it still arrives. This is done-criterion 6 and the one most likely to be quietly missed.

- [ ] **Step 7: Record the run** in `oauth-conformance.md`, stating exactly what was seeded. A run against an empty table proves nothing; say what data it used.

- [ ] **Step 8: Gates and commit.**

---

## Self-review notes

- **Spec coverage:** tables → T1; `GET /api/agents` + registration → T2, T3; 409 org selection → T3; the three `/api/agent-cron/*` endpoints → T4; JWT minting, retry semantics, rehydration → T5; all eight "done means" criteria → T6.
- **Deferred by the spec, not missing here:** the oc-platform liveness call (`oc_platform_user_id` is stored, never read), and widening auto-approve.
- **Riskiest tests, written before their implementations:** T2's isolation test (a second user's agent actually present), T5's `purpose` claim (without it any agent token fires any job), and T5's rehydration test (silent when broken).
