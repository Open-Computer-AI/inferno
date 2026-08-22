STATUS: in progress

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

(report continues as HTTP layer lands)
