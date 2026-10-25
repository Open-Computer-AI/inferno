STATUS: complete (fix round 1/5 addressed)

# Task 4: /api/agent-cron/{provision,cancel,list}

## Plan

1. `AgentCronService` (`backend/internal/service/agent_cron.go`) + tests
   (`agent_cron_test.go`) -- TDD per the brief's Steps 1-6, plus a few extra
   isolation/validation tests in the same spirit as `agent_registry_test.go`.
2. Add `AgentRegistryService.ResolveOwnedAgentRowID` -- the piece the brief
   calls out ("you will need to resolve public_id -> row id for the
   caller's own agent, scoped to the caller") -- keyed off the OAuth
   bearer's `aud` claim (the agent's own client_id, which per
   `RegisterAgentInput`'s doc comment IS the agent's public_id), never a
   request-body field the real chronos client
   (`_nas_client.py`) doesn't send.
3. `AgentHandler`: three new methods (`ProvisionCron`, `CancelCron`,
   `ListCron`) mounted at `/api/agent-cron/{provision,cancel,list}`, same
   file as the existing `List`/`Register`.
4. `routes/agents.go`: mount the three routes on the same `RequireOAuthScope("")`
   gate as `/api/agents` -- no `agents:read`/`agents:manage`.
5. Wire `AgentCronService` through `wire.go`/`wire_gen.go`/`handler.go` by
   hand (no `wire` binary on PATH; task 3 did the same).
6. Route-level tests mirroring `agents_route_test.go`'s harness, including
   a genuine cross-tenant fixture for list/cancel.
7. Gate: `cd backend && go test -tags unit ./...` (timeout 600000), full run.
8. Divergence: declare every touched/created file in
   `inferno-frontend/scripts/check-divergence.sh`'s `DECLARED` and a new
   `GOAL.md` ledger row (D11). Baseline confirmed before touching anything:
   9 undeclared files (`auth_email_binding.go`, `auth_oauth_email_flow.go`,
   `balance_notify_service.go`, `content_moderation.go`,
   `domain_constants.go`, `payment_order_result_test.go`,
   `setting_features.go`, `setting_service_update_test.go`,
   `totp_service.go`) -- none of these are mine; the exit code must stay 1
   with exactly these 9 and nothing else.

## Design decisions / things the brief left implicit

- **Wire contract keys** are exact: `plugins/cron_providers/chronos/_nas_client.py`
  (found in `~/.cache/uv/archive-v0/.../plugins/cron_providers/chronos/_nas_client.py`,
  not inside this repo -- the brief's citation path
  `plugins/cron_providers/chronos/_nas_client.py:96-120` matches that file 1:1).
  - `POST /api/agent-cron/provision` body: `{job_id, fire_at, agent_callback_url, dedup_key}`.
    Response cited as "e.g. `{schedule_id}`" -- I emit `{job_id, fire_at, schedule_id}`,
    all snake_case, bare JSON.
  - `POST /api/agent-cron/cancel` body: `{job_id}`. Response: bare `{}` on
    success; the client only checks the HTTP status.
  - `GET /api/agent-cron/list`: no query params. Response:
    `{"armed": [{job_id, fire_at, schedule_id}, ...]}` -- `list_armed()`
    reads `data.get("armed")` and type-checks it's a list.
- **Which agent is calling?** The python client sends none of
  {job_id, fire_at, ...} an agent identifier -- it authenticates with "the
  agent's existing Nous Portal access token". `RegisterAgentInput.PublicID`'s
  doc comment already establishes `public_id` IS the agent's OAuth
  `client_id`. `RequireOAuthScope` publishes the token's verified `aud`
  claim as `OAuthContextKeyClientID` -- "the CALLER'S IDENTITY, which
  handlers act on" (oauth_scope.go's own doc comment). So: resolve the
  calling agent from `aud`, not from a body field the real client never
  sends. `AgentRegistryService.ResolveOwnedAgentRowID(ctx, userID, publicID)`
  looks the agent up by `public_id`, 404s if absent, 403s
  (`AGENT_CRON_NOT_OWNED_BY_CALLER`) if it belongs to a different
  `user_id` -- mirrors `Register`'s own hijack guard (ruling T2-2) instead
  of inventing a second ownership pattern.
- **Provision's idempotent re-arm** is `INSERT ... ON CONFLICT (dedup_key)
  DO UPDATE SET updated_at = now()` -- `OnConflictColumns(dedup_key)` +
  `.UpdateUpdatedAt()` only (never `UpdateNewValues()`, which would also
  silently overwrite `schedule_id` on a re-arm and break "same fire, same
  schedule_id"). Touching `updated_at` is what forces a real `UPDATE` so
  `RETURNING id` still resolves on conflict, mirroring why `Register` needs
  its own upsert shape.
- **Cancel is scoped to `agent_row_id`** in the `WHERE`, not just `job_id`
  -- `job_id` is not necessarily globally unique across agents, so an
  unscoped cancel could let one agent cancel another agent's fire by
  guessing its `job_id`. Proven by
  `TestCancelDoesNotAffectAnotherAgentsFireWithTheSameJobID`.
- **`schedule_id`** has no external scheduler in this design (Task 1's
  brief: Chronos hides the real scheduler entirely, NAS/Inferno owns
  nothing external) -- it's minted here as `"cron_" + uuid.NewString()`,
  the same pattern several other services in this codebase already use for
  opaque ids (`antigravity_gateway_service.go`, `backup_service.go`, etc).

## Step 1: idempotency test, mutation-proven

`TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire` written verbatim
from the brief. RED before implementation (no `AgentCronService` existed),
GREEN after. Mutation proof (compiles): swapped
`.UpdateUpdatedAt()` for `.UpdateScheduleID()` on the upsert's conflict
clause -- re-arm now overwrites `schedule_id` on every call. Failure:

```
Error Trace:  agent_cron_test.go:122
Not equal:
expected: "cron_d9c25cbf-4eb9-47be-bc1a-4b336375a9d3"
actual  : "cron_13b7051e-f5bd-4ac5-975c-58cc7ac8051f"
Messages: same fire, same schedule_id
--- FAIL: TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire
```

Reverted (verified `diff` empty against the pre-mutation copy), rebuilt
clean.

## Step 5: cross-tenant test, mutation-proven

`TestListArmedReturnsOnlyThisAgentsFires` written verbatim from the brief,
plus `fx.seedFire(2, ...)` seeding a genuinely different `agent_row_id` in
the same database (not vacuous). Mutation proof (compiles): dropped the
`agentcronfire.AgentRowIDEQ(agentRowID)` predicate from `ListArmed`'s
`Where`. Failure:

```
Error: "[{ours 2026-09-01T10:00:00Z cron_seed_1} {another agent's 2026-09-01T10:00:00Z cron_seed_2}]"
       should have 1 item(s), but has 2
--- FAIL: TestListArmedReturnsOnlyThisAgentsFires
```

Reverted, rebuilt clean.

## Step 6: cancel tests

- `TestCancelMarksStateCancelledAndRemovesFromListArmed`: state flips to
  `cancelled`, then absent from `ListArmed`.
- `TestCancelOnAnUnknownJobIDIsANoOp`: no error on an unrecognized job_id.
- `TestCancelDoesNotAffectAnotherAgentsFireWithTheSameJobID`: cross-tenant
  isolation for Cancel specifically (job_id is not globally unique).

All green on first implementation (`internal/service` full run, see below).

## HTTP layer

`AgentHandler` gains three methods (`ProvisionCron`, `CancelCron`,
`ListCron`), same file as `List`/`Register`; `AgentHandler` now also holds
`cronSvc *service.AgentCronService` (constructor signature changed to a
3rd param -- every caller updated: `wire_gen.go`, `agents_route_test.go`).
`routes/agents.go` mounts `/api/agent-cron/{provision,cancel,list}` on the
identical `RequireOAuthScope("")` gate as `/api/agents` -- no
`agents:read`/`agents:manage`.

**Calling-agent resolution** (`agentCronHandlerAgentRowID`): the wire
contract's request bodies carry no agent-id field at all
(`_nas_client.py`'s `provision`/`cancel`/`list_armed` bodies are exactly
`{job_id, fire_at, agent_callback_url, dedup_key}` / `{job_id}` / none).
The agent authenticates with its own access token, whose `aud` IS its
`public_id` (`RegisterAgentInput.PublicID`'s doc comment) --
`RequireOAuthScope` already publishes that as `OAuthContextKeyClientID`,
documented in `middleware/oauth_scope.go` as "the CALLER'S IDENTITY, which
handlers act on". So the handler reads `aud` from context and calls the
new `AgentRegistryService.ResolveOwnedAgentRowID(ctx, userID, publicID)`
rather than trusting a body field a real client never sends.

**Response shapes** (bare JSON, snake_case throughout -- this endpoint set
IS the real client's hardcoded contract, unlike `/api/agents/register`'s
"ours alone" snake_case-request/camelCase-response split):
- `POST /api/agent-cron/provision` -> `{job_id, fire_at, schedule_id}`.
- `POST /api/agent-cron/cancel` -> `{}` on success (the real client only
  checks HTTP status).
- `GET /api/agent-cron/list` -> `{"armed": [{job_id, fire_at, schedule_id}, ...]}`,
  matching `list_armed()`'s `data.get("armed")` read exactly.

## Route tests (`internal/server/routes/agent_cron_route_test.go`)

Extended the existing `agentRouteEnv` harness (`agents_route_test.go`) with
`mintForClient` (token whose `aud` is an arbitrary client_id, not the
shared "hermes-cli" desktop client every other test in that file uses) and
`registerAgentClient` (creates BOTH the `OAuthClient` row -- so the token
verifies -- and the matching `Agent` row -- so `ResolveOwnedAgentRowID`
finds it -- for one `public_id`).

9 new tests, all real-chain (real RS256 keys, real middleware, real
services, sqlite ent): wire-shape happy path, idempotent re-arm returns the
same `schedule_id` at the HTTP layer too, a token for an unregistered
`public_id` is refused, cross-tenant list isolation with TWO real
registered agent clients under the SAME user (not vacuous -- each
provisions its own fire, the caller only ever sees its own), cancel's
round trip (provision -> cancel -> absent from list), cancel-on-unknown-job
is a 200 no-op, unauthenticated -> 401 on all three routes, any valid scope
admitted (no agents:read/manage), and the `/api/v1` mount-point pin.

## Gates

**Divergence** (`./inferno-frontend/scripts/check-divergence.sh` from repo
root): confirmed baseline BEFORE touching anything -- 9 undeclared files,
exit 1 (`auth_email_binding.go`, `auth_oauth_email_flow.go`,
`balance_notify_service.go`, `content_moderation.go`, `domain_constants.go`,
`payment_order_result_test.go`, `setting_features.go`,
`setting_service_update_test.go`, `totp_service.go` -- all pre-existing,
none of them mine). After every change and after the final commit:

```
divergence: base baeac1f3de · 242 file(s) differ · 253 declared

  UNDECLARED DIVERGENCE:
    backend/internal/service/auth_email_binding.go
    backend/internal/service/auth_oauth_email_flow.go
    backend/internal/service/balance_notify_service.go
    backend/internal/service/content_moderation.go
    backend/internal/service/domain_constants.go
    backend/internal/service/payment_order_result_test.go
    backend/internal/service/setting_features.go
    backend/internal/service/setting_service_update_test.go
    backend/internal/service/totp_service.go
EXIT=1
```

Same 9 files, same exit code, as the pre-task baseline -- ends at exactly 9
as required. Declared: `backend/internal/service/agent_cron.go`,
`agent_cron_test.go`, `backend/internal/server/routes/agent_cron_route_test.go`
as new D11 entries in both `check-divergence.sh`'s `DECLARED` and
`GOAL.md`'s ledger; the touched-but-already-declared files
(`agent_handler.go`, `routes/agents.go`, `agents_route_test.go`,
`agent_registry.go`, `agent_registry_test.go`, `service/wire.go`,
`cmd/server/wire_gen.go`) needed no new declaration, D9/D10's rows already
cover them (noted explicitly in D11's row).

**Full gate**: `cd backend && go test -tags unit ./...` (timeout 600000).

```
ok  	github.com/Wei-Shaw/sub2api/internal/handler	(cached)
ok  	github.com/Wei-Shaw/sub2api/internal/handler/admin	(cached)
ok  	github.com/Wei-Shaw/sub2api/internal/repository	4.145s
ok  	github.com/Wei-Shaw/sub2api/internal/server	2.702s
ok  	github.com/Wei-Shaw/sub2api/internal/server/middleware	14.432s
ok  	github.com/Wei-Shaw/sub2api/internal/server/routes	12.871s
ok  	github.com/Wei-Shaw/sub2api/internal/service	161.977s
...
```

52 packages `ok`, 0 `FAIL` lines, 61 packages with no test files (`grep -c
"^ok"` / `grep -i FAIL` / `grep -c "no test files"` against the full log).
Neither known flake (`TestFilterGrokFreeQuotaAccounts*`,
`TestApplyCodexFingerprintClientMetadataRaw*`) fired this run.

## Divergence from the brief

- The brief's Files block doesn't mention `agent_registry.go`/
  `agent_registry_test.go` as files this task touches, but resolving
  `public_id -> agent_row_id` scoped to the caller (explicitly called out
  in the brief's Interfaces section) has nowhere else to live that doesn't
  duplicate `AgentRegistryService`'s existing ownership-check pattern
  (ruling T2-2). Added `ResolveOwnedAgentRowID` there instead of
  re-implementing agent lookup inside `AgentCronService`, and declared it
  under D9 in the divergence ledger (D9 already covers that file).
- The brief's `ArmedFireView{JobID, FireAt, ScheduleID string}` doesn't
  specify `FireAt`'s format on the way out. Used RFC3339 (matching the
  request's own format) rather than passing through the driver's raw
  `time.Time` formatting, so a value written by `Provision` and read back
  by `ListArmed` is byte-identical regardless of which one produced it.
- Nothing else in the brief was found to be wrong; the exact wire contract
  keys and the "cancel is a no-op, not an error" behaviour matched
  `_nas_client.py` precisely once located (it is not in this repo -- found
  under `~/.cache/uv/archive-v0/.../plugins/cron_providers/chronos/_nas_client.py`,
  matching the brief's citation path 1:1).

## Fix round 1/5 (review: spec ❌ / quality not approved -- 1 Critical, 1 Important, 1 accepted Minor)

### CRITICAL -- Provision let one agent collide into another agent's row

`agent_cron.go`'s `OnConflictColumns(agentcronfire.FieldDedupKey)` targeted a
column that was field-level (globally) `.Unique()`. `dedup_key` is
`{job_id}:{fire_at}` and `job_id` lives in the CALLING AGENT's own
namespace -- two different agents legitimately arm identically-named jobs.
Reviewer reproduced: agent 2 provisioning agent 1's `dedup_key` got HTTP 200
carrying agent 1's `job_id`/`schedule_id`, no row of its own, agent 1's
`updated_at` touched. Same shape as the T2-2 `Register` hijack; the reasoning
did not get carried across from `public_id` to `dedup_key`.

**Ruling T4-1 (structural fix, not just a guard):**

1. `ent/schema/agent_cron_fire.go`: dropped `dedup_key`'s field-level
   `.Unique()`, added a composite `index.Fields("agent_row_id",
   "dedup_key").Unique()`. `go generate ./ent` regenerated only
   `ent/migrate/schema.go` (uniqueness is a pure DB-constraint property, no
   CRUD/validator code touches it).
2. `backend/migrations/913_agent_cron_fires_dedup_key_per_agent.sql`: drops
   912's inline constraint (Postgres's default name for a column-level
   `UNIQUE`, `agent_cron_fires_dedup_key_key`) and creates the composite
   index under the exact name ent generates
   (`agentcronfire_agent_row_id_dedup_key`), styled after
   `910_api_key_oauth_client_uniq_live_only.sql`.
3. `agent_cron.go`'s `Provision`: `OnConflictColumns(agentcronfire.FieldAgentRowID,
   agentcronfire.FieldDedupKey)` -- targets the composite index, so a
   foreign agent's row can no longer be a conflict target at all.
4. Kept a post-upsert `if row.AgentRowID != agentRowID { return
   fmt.Errorf(...) }` assertion as defense in depth -- the composite index
   should make this unreachable; asserted anyway per the instruction.

**New test** `TestProvisionByTwoDifferentAgentsWithTheSameDedupKeyGivesEachItsOwnRow`:
two agents provision the identical `dedup_key`, each gets its own row and
its own `schedule_id`, `countFires() == 2`. Existing same-agent idempotency
test (`TestProvisionTwiceWithTheSameDedupKeyArmsExactlyOneFire`) verified
still green, unmodified.

**Mutation proof** (compiles): reverted `OnConflictColumns` to
`agentcronfire.FieldDedupKey` alone (the pre-fix single-column target).
Against the now-composite-only schema this doesn't silently collide (as it
did against Postgres's old single-column constraint) -- sqlite has no
matching constraint to target at all, so the two-agent test fails with:

```
Error: agent cron: provision job "daily-report": SQL logic error:
ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint (1)
--- FAIL: TestProvisionByTwoDifferentAgentsWithTheSameDedupKeyGivesEachItsOwnRow
```

Proof that the composite target is load-bearing (a schema/query mismatch is
the strongest form of "this wiring matters" available once the old
single-column constraint no longer exists to collide against). Reverted,
diffed empty against the pre-mutation copy, rebuilt clean.

### IMPORTANT -- ruling T4-2: ResolveOwnedAgentRowID accepted a revoked agent

`ListForUser` already filters `agent.RevokedAtIsNil()` (with its own test);
`ResolveOwnedAgentRowID` -- the entry point for the entire
`/api/agent-cron/*` surface -- did not. Revocation is the intended kill
switch, and cron is precisely the capability that keeps running UNATTENDED
after revocation if unchecked. Nothing sets `revoked_at` in production code
yet (unreachable today), fixed before it needed to be reachable.

Fix: added `agent.RevokedAtIsNil()` to the `Where` in
`ResolveOwnedAgentRowID`. New test
`TestResolveOwnedAgentRowIDRejectsARevokedAgent`: a revoked agent's
`public_id` no longer resolves, even for its own owner.

### MINOR (accepted) -- cross-tenant route coverage used the same user_id twice

`TestAgentCronListRouteReturnsOnlyThisAgentsArmedFires` seeded two agents
both under `agentRouteUserID`, so it could not distinguish `user_id`
isolation from `agent_row_id` isolation at the route layer -- only the
service-layer tests had a genuinely different `user_id`.

Added two route tests:
- `TestAgentCronListRouteIsolatesTwoGenuinelyDifferentUsers`: two real,
  distinct users (7 and 9), each with their own registered agent; each
  user's list call sees only their own fire.
- `TestAgentCronProvisionRouteRejectsATokenWhoseSubDoesNotOwnTheAgentNamedByAud`:
  a validly-signed token whose `sub` (9) does NOT own the agent its `aud`
  names (registered to user 7) -> 403. Proves `agentCronHandlerAgentRowID`'s
  ownership check is reachable and enforced at the HTTP layer, not just
  exercised in the service's own unit tests.

### No action

Reviewer's Minor #4 (`org_id` absent from `ResolveOwnedAgentRowID`) --
deliberate and correct per the reviewer's own note: `public_id` is globally
unique and names one already-registered agent, unlike `ListForUser`'s
multi-org browsing case. Left as-is.

### Divergence for this round

Declared `backend/migrations/913_agent_cron_fires_dedup_key_per_agent.sql`
in both `check-divergence.sh`'s `DECLARED` list (under D8, since
`agent_cron_fire.go` and `migrate/schema.go` were already declared there)
and a new sub-note continuing D8 in `check-divergence.sh`. Corrected GOAL.md's
D8 row: it previously documented `dedup_key` as globally `UNIQUE`, which
this round makes false -- rewrote the description and the "re-apply after
rebase" column to describe the composite-index behaviour and the
912-then-913 apply order.

### Commits (5, one per fix)

- `8c9cf248ce` -- schema + migration 913 (composite unique index)
- `f3f3b46499` -- service: target composite index in Provision's upsert +
  two-agent test + mutation proof
- `d682ba8b55` -- service: ResolveOwnedAgentRowID excludes revoked agents +
  test
- `1a561d5232` -- route tests: genuinely-different-user cross-tenant
  coverage + forged-token ownership test
- `7aa6d87228` -- divergence ledger: declare 913, correct D8's description

### Gates after this round

Divergence: `./inferno-frontend/scripts/check-divergence.sh` from repo root
-- exit 1, same 9 pre-existing undeclared files as the original baseline
(`auth_email_binding.go`, `auth_oauth_email_flow.go`,
`balance_notify_service.go`, `content_moderation.go`,
`domain_constants.go`, `payment_order_result_test.go`,
`setting_features.go`, `setting_service_update_test.go`,
`totp_service.go`). None of these are mine; count unchanged by this round.

Full gate: `cd backend && go test -tags unit ./...` (timeout 600000) -- 52
packages `ok`, 0 `FAIL` lines. Neither known flake
(`TestFilterGrokFreeQuotaAccounts*`, `TestApplyCodexFingerprintClientMetadataRaw*`)
fired.
