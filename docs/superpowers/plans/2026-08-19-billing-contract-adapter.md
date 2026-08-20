# The `/api/billing/*` contract adapter — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Make the unmodified hermes client show Inferno's real billing state — balance, plan, usage, top-up — instead of degraded/unknown states and links to Nous's billing page.

**Architecture:** One `BillingContractService` composes Inferno's **existing user-facing services** and is exposed at the Nous-shaped paths the client calls. No new billing functionality, no schema changes, no admin routes.

**Tech Stack:** Go 1.26, Gin, ent/Postgres.

**Spec:** `docs/superpowers/specs/2026-08-19-billing-contract-adapter-design.md`

## Global Constraints

- **Bare JSON, never the panel envelope.** These are Nous-shaped endpoints like `/api/oauth/*`, not `/api/v1/*`. The client parses the raw object; `{code,message,data}` breaks it silently.
- **User-facing services only.** The caller holds a *user's* OAuth token. Nothing here may reach an admin route or an admin service method.
- **Reads on `billing:read`, writes on `billing:manage`.** `billing:manage` is never granted at login and `/oauth/authorize` refuses it — so writes ship built and unreachable. That is deliberate; do not relax it.
- **Fail loud on our side, quiet on theirs.** The client fails open (`build_billing_state` → `logged_in=false`). Never 500 the whole response because one optional aggregate failed — return the fields that resolved and log the rest.
- **Infrastructure faults → 500, never an auth error.**
- **No new tables, no migrations.** If a field needs one, it is out of scope; report it.
- **Use the cached service, not the repo, wherever Inferno already caches.** This
  endpoint is polled by agents; ~25 Redis cache layers already exist
  (`billing_cache`, `dashboard_cache`, `identity_cache`, `api_key_cache`, …).
  Reaching past them to a repo turns a cached read into a query storm.
- Gates every task: `cd backend && go build ./... && go vet ./... && go vet -tags unit ./... && go test ./... && go test -tags unit ./...`. Both tagged runs — 398 of 1132 test files carry `//go:build unit`. Plus `bash inferno-frontend/scripts/check-divergence.sh`, undeclared must stay at **19**; declare touched files in its `DECLARED` list AND `GOAL.md`. Never put backticks or `$(` inside `DECLARED`.
- Every behaviour needs a test that fails without it, proven by a **compiling** mutation.

---

### Task 1: `BillingContractService` and `GET /api/billing/state`

**Files:**
- Create: `backend/internal/service/billing_contract.go`, `billing_contract_test.go`
- Create: `backend/internal/handler/billing_contract_handler.go`, `_test.go`
- Create: `backend/internal/server/routes/billing_contract.go`
- Modify: `backend/internal/server/router.go`, `internal/service/wire.go`, `internal/handler/wire.go`

**Interfaces — Produces:**
```go
type BillingContractService struct{ /* userSvc, orgSvc, subSvc, usageSvc, payCfgSvc */ }

// State composes the client's overview screen. Partial results are normal:
// a nil section means "could not resolve", never "zero".
func (s *BillingContractService) State(ctx context.Context, userID int64) (*BillingStateView, error)
```

**Consumes (all existing, all user-scoped, and CACHED where a cache exists):**
- `BillingCacheService.GetUserBalance(ctx, userID)` → balance. **Not**
  `userSvc.GetByID`. Inferno already caches the balance in Redis with async
  write workers and jitter (`billing_cache.go`, `billing_cache_service.go:311`),
  and balance is the hottest field in this response. Every deploy is DB+Redis,
  so the cache is never absent.
- `OrgService.OrgsForUser(ctx, userID)` + role lookup — the same calls `OAuthHandler.Account` already makes
- `SubscriptionService.ListActiveUserSubscriptions(ctx, userID)`
- `UsageService.GetStatsByUser(ctx, userID, monthStart, now)` → month-to-date
  spend. Prefer the dashboard cache (`dashboard_cache.go`) if it already serves
  this window; check before adding an uncached aggregate to a polled endpoint.
- `PaymentConfigService.GetAvailableMethodLimits(ctx)` → min/max top-up

**The response shape** (what `agent/billing_view.py` parses):
```json
{
  "logged_in": true,
  "org": {"id":"1","slug":"…","name":"…","role":"OWNER","can_change_plan":true},
  "balance_usd": "999.99",
  "cli_billing_enabled": true,
  "charge_presets": ["10","25","50"],
  "min_usd": "5", "max_usd": "500",
  "card": {"kind":"none"},
  "auto_reload": {"enabled": false},
  "bounds": {"limit_usd": null, "spent_this_month_usd": "0.02", "is_default_ceiling": false}
}
```

- [ ] **Step 1: Write the failing test**

```go
func TestBillingStateReportsBalanceOrgAndSpend(t *testing.T) {
	svc, fx := newBillingContractFixture(t)   // user id 7, balance 42.50, org OWNER
	fx.recordUsage(7, 1.25)

	got, err := svc.State(context.Background(), 7)
	require.NoError(t, err)

	require.True(t, got.LoggedIn)
	require.Equal(t, "42.5", got.BalanceUSD)
	require.Equal(t, "OWNER", got.Org.Role)
	require.Equal(t, "1.25", got.Bounds.SpentThisMonthUSD)
	require.Equal(t, "none", got.Card.Kind,
		"Inferno has no card vault; reporting anything else makes the CLI offer a one-click charge that cannot work")
	require.False(t, got.AutoReload.Enabled)
}
```

- [ ] **Step 2: Run it, watch it fail** — `go test -tags unit ./internal/service/ -run TestBillingState -v`. Expected: undefined.

- [ ] **Step 3: Implement `State`**

Compose the five sources. Each optional section is resolved independently: a failure logs and leaves that section nil, it does **not** abort. Only a failure to resolve the *user* is fatal (no balance = nothing useful to say).

- [ ] **Step 4: Test partial degradation**

With the usage source erroring, `State` still returns a balance and org, `Bounds` is nil, and the error is logged. Mutation: make the usage error abort the whole call → this test fails.

- [ ] **Step 5: Mount the route**

`GET /api/billing/state`, bare JSON, behind `middleware.RequireOAuthScope(…, service.ScopeBillingRead)`. Registered like `/api/oauth/*` — root-level group, not `/api/v1`.

- [ ] **Step 6: Test the wire contract**

Response is bare JSON with no `code`/`message`/`data` keys — assert their **absence**, since a wrong envelope is silently mis-parsed rather than erroring. A token without `billing:read` gets `403 insufficient_scope`.

- [ ] **Step 7: Gates and commit**

---

### Task 2: `GET /api/analytics/usage`

**Files:** modify the Task 1 service/handler/routes files; tests beside them.

**Interfaces:**
- Consumes: `UsageService.ListByUser(ctx, userID, params)`, `GetStatsByUser`
- Produces: `func (s *BillingContractService) Usage(ctx context.Context, userID int64, p pagination.PaginationParams) (*UsageView, error)`

- [ ] **Step 1: Write the isolation test first — it is the important one**

```go
func TestUsageReturnsOnlyTheCallersRows(t *testing.T) {
	svc, fx := newBillingContractFixture(t)
	fx.recordUsage(7, 1.00)   // ours
	fx.recordUsage(8, 99.00)  // a second real user, in the same database

	got, err := svc.Usage(context.Background(), 7, firstPage)
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, int64(7), got.Items[0].UserID)
}
```

A second user's data must be **present in the database**, or this test passes against an implementation with no filter at all.

- [ ] **Step 2: Run it, watch it fail.**
- [ ] **Step 3: Implement `Usage`.**
- [ ] **Step 4: Run it, watch it pass.**
- [ ] **Step 5: Mutation-prove the filter** — drop the `userID` predicate (compiling), show the test failing with user 8's row present.
- [ ] **Step 6: Mount `GET /api/analytics/usage` on `billing:read`; gates and commit.**

---

### Task 3: `GET /api/billing/subscription` and `/pending-change`, `GET /api/billing/auto-top-up`

**Files:** as Task 2.

**Interfaces:**
- Consumes: `SubscriptionService.ListActiveUserSubscriptions`, `GetActiveSubscription(ctx, userID, groupID)`
- Produces: `Subscription(ctx, userID) (*SubscriptionView, error)`

- [ ] **Step 1: Failing test** — a user with an active subscription gets its plan name, period and status; a user with none gets a well-formed "no plan" object, **not** an error and not a 404.

- [ ] **Step 2: Run, fail. Step 3: Implement. Step 4: Run, pass.**

- [ ] **Step 5: The two honest-gap endpoints**

`GET /api/billing/auto-top-up` → `{"enabled": false}`.
`GET /api/billing/subscription/pending-change` → `{"pending": null}`.

Both are truthful: Inferno models neither. Test that they answer 200 with those shapes rather than 404 — a 404 reads to the client as "portal unreachable", which is a worse lie than "not enabled".

- [ ] **Step 6: Gates and commit.**

---

### Task 4: `subscription` in `/api/oauth/account`

**Files:** modify `backend/internal/handler/oauth_handler.go`, its test.

The client reads `payload["subscription"]`. #4 omitted it deliberately — "unverified mapping". Task 3 builds that mapping, so it is no longer a guess.

- [ ] **Step 1: Failing test** — a user with an active subscription sees it in the account payload; a user without sees the key **absent** (not null), which the client reads as "no plan".
- [ ] **Step 2: Run, fail. Step 3: Implement, reusing Task 3's mapper — not a second one. Step 4: Run, pass.**
- [ ] **Step 5:** `tool_access` stays omitted. Add a test asserting it is absent, so a future permissive default is a deliberate act and not a drift.
- [ ] **Step 6: Gates and commit.**

---

### Task 5: The write endpoints — built, gated, unreachable

**Files:** as Task 2.

```
POST /api/billing/charge            create a top-up order
GET  /api/billing/charge/{id}       its status
POST /api/billing/subscription/upgrade
POST /api/billing/subscription/preview
PUT  /api/billing/auto-top-up       → 501, with a reason
```

All on `billing:manage` — the scope nothing can currently grant.

- [ ] **Step 1: Write the gate test FIRST**

Every write endpoint, with a token carrying `billing:read` but not `billing:manage`, returns `403 insufficient_scope`. This is the boundary protecting a scope no flow issues; it is worth more than the handlers.

- [ ] **Step 2: Run, fail. Step 3: Implement `charge` over `PaymentService` order creation; `upgrade`/`preview` over the plans surface. Step 4: Run, pass.**

- [ ] **Step 5:** `PUT /api/billing/auto-top-up` → `501` with a body naming the reason (no stored payment method). Test that it is 501 and not 404 or 200.

- [ ] **Step 6: Mutation-prove the gate** — drop `billing:manage` to `billing:read` on one write route (compiling), show the gate test failing.

- [ ] **Step 7: Gates and commit.**

---

### Task 6: Conformance against the real client

The deliverable is **evidence**, and it decides whether this plan worked.

- [ ] **Step 1: Stand up the runbook's `t8` infra** (`backend/scripts/oauth-conformance.md` §1-3), obtain a real token via the unmodified `hermes_cli.auth._nous_device_code_login`, fund the user, bind a real upstream account.

- [ ] **Step 2: Drive the real CLI**

```
uv run hermes /usage
uv run hermes /subscription
uv run hermes /topup
```

Record verbatim. **The pass condition is that none of them mention `portal.nousresearch.com`** and `/usage` shows Inferno's real numbers.

- [ ] **Step 3: Assert the balance matches**

The figure the CLI prints equals `users.balance` in Postgres. Not "looks right" — compared.

- [ ] **Step 4: Prove isolation end to end** — a second funded user in the same database; the first user's `/usage` never shows the second's rows.

- [ ] **Step 5: Prove the write gate on the wire** — `POST /api/billing/charge` with the real token → `403 insufficient_scope`, because the device grant never issues `billing:manage`.

- [ ] **Step 6: Prove fail-open** — stop Redis (or force the usage source to error), re-run `/usage`: the CLI degrades cleanly and `/api/billing/state` still returns a balance.

- [ ] **Step 7: Extend `backend/scripts/oauth-conformance.md` with the billing leg; tear down every container. Commit.**

---

## Self-review notes

- **Spec coverage:** `billing/state` → T1. `analytics/usage` → T2. `subscription`/`pending-change`/`auto-top-up` read → T3. Account `subscription` field → T4. Writes + scope boundary → T5. All seven "done means" criteria → T6.
- **Task 1 is first** because every later task reuses its service, fixture and route group. Its partial-degradation test is the one that encodes the client's fail-open contract.
- **The two riskiest tests are written before their implementations**: T2's isolation test (needs a second user's data actually present) and T5's scope gate (protects a scope nothing can grant, so nothing else would catch a regression).
- **Deliberately deferred:** stored cards, auto top-up, the `billing:manage` step-up device flow, per-org monthly ceilings, and retiring `/api/v1/payment/*`. All are named non-goals in the spec.
- **No migrations.** If any task finds it needs one, that is a signal the field belongs to a later change — report rather than add.
