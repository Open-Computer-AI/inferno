# Separate Live and Ordinary Concurrency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Follow repository routing: bounded implementation in dedicated Luna threads, parent-controlled operations, and independent adversarial review. No host mutation is implied by this planning document.

**Goal:** Allow independently bounded Live and ordinary inference concurrency on an opted-in account and user, without changing existing shared-limit behavior by default.

**Architecture:** Preserve the existing separate Redis Live/ordinary sets. Add an explicit nullable split policy, make admission and scheduler load class-aware, and give each Live creation exactly one account/user reservation that becomes the session lease. Expose separate controls and occupancy in the admin API/UI; enable only on staging after proving both budgets under simultaneous load.

**Tech Stack:** Go 1.27, PostgreSQL/Ent, Redis Lua, Vue/TypeScript, existing WebRTC/Athena validation harnesses. No new runtime dependency.

**Spec:** `docs/superpowers/specs/2026-10-18-split-live-concurrency-scope.md`.

## Global Constraints

- Start implementation in a NEW worktree from deployed staging commit `c8c64ad765dc45afaf55423bb42c9a571e89c72c`; read the applicable AGENTS.md first.
- Example acceptance policy: account Live4/requests2. This is a scheduler test policy, not a claim about upstream account capacity.
- Nullable paired fields `live_concurrency` and `request_concurrency`: both NULL means legacy shared policy; both positive means split. Reject incomplete/nonpositive pairs. No borrowing.
- Account and user policy are independently evaluated. In split mode the legacy field is not an additional binding total cap.
- Workload is derived by the server: Live endpoint/capability versus ordinary inference. Caller headers/model names cannot select a privileged class.
- Preserve API-key counts as statistics and `current_concurrency` as an aggregate. Idle Responses WebSocket ingress retains its separate existing connection limit.
- Ordinary user capacity covers all supported inference providers and API keys for that user, not only Athena classifiers.
- Staging only when execution reaches deployment. Do not change the Air, production database/runtime/domain, attestation helper/tunnel, credentials, billing or teardown behavior.
- No Redis flush, automatic limit expansion, classifier/hedge changes, new phone calls, or production rollout in this plan.
- Use repository generation for Ent outputs; never hand-edit generated code. Discover the next migration identifier at execution time.

## Review Focus

- Full ordinary capacity must not prevent Live creation: Task 4 saturation tests.
- A legacy provider/auth path must not bypass split user caps: Task 5 route matrix.
- In-flight creation/cancellation/restart must not leak or oversubscribe Live leases: Tasks 3–4 crash/race tests.
- Clearing or partially updating a policy must not silently remove limits: Tasks 1–2 API/DB tests.
- Shared load-cache results and old gateway workers must not reinterpret split capacity: Task 4 cache tests and Task 8 rollout gates.

## Proposed interfaces and ownership

Add focused policy definitions in `backend/internal/service/concurrency_policy.go`:

- `ConcurrencyClass` with server-only constants `ConcurrencyClassRequest` and `ConcurrencyClassLive`.
- `ConcurrencyPolicy { Shared int; Live *int; Request *int }`.
- `ValidateConcurrencyPolicy(policy ConcurrencyPolicy) error` and `EffectiveConcurrencyLimit(policy ConcurrencyPolicy, class ConcurrencyClass) (limit int, shared bool, err error)`.
- `ConcurrencySnapshot { Request int; Live int; Total int }`, where Total is always Request+Live.
- `LiveAdmissionRequest { AccountID int64; AccountPolicy ConcurrencyPolicy; UserID int64; UserPolicy ConcurrencyPolicy; APIKeyID int64; LeaseID string }`.
- `LiveAdmissionResult { Acquired bool; DeniedScope string; Snapshot ConcurrencySnapshot }`, with denied scope account/user; Redis/service errors remain Go errors.

Extend the Live cache/service with atomic `AcquireLiveReservation(ctx context.Context, request LiveAdmissionRequest) (LiveAdmissionResult, error)`. The scheduler passes the generated leaseID and owns release until it transfers the reservation to `CreateLiveCall`. The call record keeps the same ID and existing refresh/release paths.

Extend ordinary admission/load methods with explicit policy/class inputs. Preserve legacy wrappers for shared-mode callers only; update every inference caller that can serve a split user. Do not use unrelated context headers as the sole source of policy. Final signatures must be consistent across interface, implementation, mocks and callers in the same commit.

Admin writes use an optional `concurrency_policy` object: omitted preserves policy; `{"mode":"shared"}` clears both nullable fields; `{"mode":"split","live":4,"requests":2}` sets both. This avoids ambiguous JSON null-versus-omitted pointer patches. Return stored nullable limits plus `concurrency_mode` and separate current counts. Keep existing `concurrency` writes independent.

## Task 1: Policy semantics and tests

**Files:** Create `backend/internal/service/concurrency_policy.go`, `concurrency_policy_test.go`; modify service `account.go`, `user.go`.

**Consumes:** Existing Concurrency values and nil pointer conventions. **Produces:** Policy types/helpers above, plus `LiveConcurrency *int` and `RequestConcurrency *int` on account/user.

- [ ] Write `TestConcurrencyPolicyLegacyAndSplit`: shared4 selects combined4 for either class; split4/2 selects Live4/request2; nil/nil retains shared; half pairs, zero and negative split limits fail.
- [ ] Run `GOTOOLCHAIN=go1.27.0 go test -tags=unit ./internal/service -run '^TestConcurrencyPolicy'` from backend; confirm new tests fail before implementation.
- [ ] Implement the helpers and add mixed account/user policy table fixtures. Preserve legacy zero/unlimited semantics only for shared policy.
- [ ] Rerun the focused command; require PASS. Commit `feat: define independent Live and request concurrency policies`.

## Task 2: Persistence, administration and cache propagation

**Files:** Modify `backend/ent/schema/account.go`, `user.go`; add the next additive SQL migration under `backend/migrations`; repository `account_repo.go`, `user_repo.go`; admin account/user handlers/services; DTO `types.go`, `mappers.go`; `api_key_auth_cache*.go`, `scheduler_snapshot_service.go` and cache mappers. Add focused policy repository/admin tests.

**Consumes:** Task1 policy. **Produces:** Persisted opt-in policy, explicit patch contract, policy-aware cached account/user objects.

- [ ] Write tests for NULL defaults, atomic complete split update, omitted-policy preservation, explicit shared reset, invalid half-pair rollback, bulk update and cache invalidation.
- [ ] Add nullable integer columns to accounts/users with a DB check: both NULL OR both non-NULL and >0. Include explicit non-NULL checks; SQL CHECK permits UNKNOWN otherwise.
- [ ] Run repository `make generate` from backend to regenerate Ent/wiring as required; inspect output and retain only necessary generation changes.
- [ ] Wire repository mappings/create/update, admin patch/create/bulk, DTOs and auth/scheduler cache payloads. Old cached snapshots missing new fields must decode to shared mode. Invalidate affected user/account caches on policy edits.
- [ ] Cover account import/export/duplicate and existing admin CLI body pass-through. Existing concurrency-only edits, user batch limits and redeem/default logic must preserve split policy.
- [ ] Run focused service/handler tests plus repository migration/integration tests using an isolated Postgres DB. Verify migration on populated shared fixtures and backward-compatible reads. Commit `feat: persist opt-in concurrency policies`.

## Task 3: Atomic Redis budgets and separate occupancy

**Files:** Modify `backend/internal/repository/concurrency_cache.go`, service `concurrency_service.go`; add `concurrency_cache_split_test.go`, extend Live/integration tests and service mocks.

**Consumes:** Task1 policy, Task2 stored fields. **Produces:** Atomic Live reservation, class-aware ordinary admission and load snapshots, stable aggregate APIs.

- [ ] Write table tests for all four mixed account/user modes: shared/shared, split/shared, shared/split, split/split. Verify which scope rejects.
- [ ] Write `TestSplitConcurrencyIndependentBudgets`: acquire4Live+2requests; fifthLive and thirdrequest fail; repeat with either class saturated first. Mirror at user scope across two accounts/API keys.
- [ ] Implement Lua class-aware counts using existing key namespaces. A Live reservation atomically checks account/user and inserts account/user/API-key Live entries once; no replacing allowance. Ordinary acquisition checks its policy and keeps normal release ownership.
- [ ] Preserve Redis time, expiry cleanup, lease renewal and idempotent release. Missing/malformed policy must return an error, not accidentally admit unlimited work.
- [ ] Run `GOTOOLCHAIN=go1.27.0 go test -tags=unit ./internal/repository ./internal/service -run 'Test(SplitConcurrency|LiveLease|ConcurrencyPolicy)'`.
- [ ] Run real-Redis concurrent integration tests with many competing acquires: peak admitted4/2, no dropped/duplicate ownership, cancellations release and expired leases recover. Test startup cleanup does not erase other workers' Live leases.
- [ ] Commit `feat: enforce atomic Live and request concurrency budgets`.

## Task 4: Scheduler integration and single Live reservation

**Files:** Modify `backend/internal/service/openai_account_scheduler.go`, `openai_gateway_scheduling.go`, `openai_live.go`, `openai_live_types.go`, `concurrency_service.go`; handler `openai_live.go`; associated scheduler/Live lifecycle tests.

**Consumes:** Atomic reservation and policy-aware load. **Produces:** All scheduler variants select/acquire by class; Live creation adopts a single lease.

- [ ] Write failing tests for Live admission while ordinary slots are full and vice versa; run with advanced scheduler enabled/disabled, sticky selection, load balance, fresh-load fallback and failed first account.
- [ ] Propagate server-derived class through scheduling request, candidate filtering, class load rates, wait plans, fresh reads and fallback. Load-cache key includes class and policy/effective limit. Keep load-factor preference distinct from enforcement caps.
- [ ] Remove ordinary user acquisition from Live handler. Live account selection atomically reserves account/user Live capacity using the existing auth identity; return lease ownership to CreateLiveCall.
- [ ] Generate one leaseID per account attempt, adopt it in the successful record, and release on every failed/canceled creation. Ensure the next failed-over attempt has its own ownership. Do not reacquire or add replacing allowance after success.
- [ ] Cover simultaneous fifth creation, cancellation after upstream success/before record save, record-save failure, attestation/upstream failure, retries, sideband attach/reconnect, normal/abrupt close, terminal404/410, observer takeover and expiry. Creation reservations must survive a slow upstream request: renew at the existing lease interval while owned, and stop renewal on transfer/failure. Abort/release if renewal is lost.
- [ ] Retain account model/group/health/proxy/alias eligibility and existing Live finalization/accounting. Add policy-change-in-flight test: existing lease is retained; new admissions obey the new cap.
- [ ] Run `GOTOOLCHAIN=go1.27.0 go test -tags=unit ./internal/service ./internal/handler ./internal/repository` and require new tests plus existing shared fixtures PASS. Commit `feat: schedule Live sessions with independent reservations`.

## Task 5: User-wide enforcement and error/metric breakdown

**Files:** Modify middleware `auth_subject.go`, API-key/Google/OAuth auth paths; shared inference concurrency helpers/callers; handler `concurrency_error_response.go`; service `ops_concurrency.go`; API-key/account/user stats handlers and DTOs. Discover actual shared helper definitions with `rg 'func .*TryAcquireUserSlot|AcquireUserSlot' backend/internal` before editing.

**Consumes:** Cached user policy and class-aware helpers. **Produces:** Consistent ordinary user enforcement across routes, diagnostic rejection scope and per-class counts.

- [ ] Write route tests using a split user on multiple API keys/providers: Responses HTTP/WS turn, chat/completions and other existing ordinary inference handlers all count toward request cap; Live counts toward Live cap. Idle Responses WS is not an active request.
- [ ] Carry the validated user policy in auth context and route every user-slot acquire through the common class-aware helper; find and eliminate legacy-max bypasses for split users.
- [ ] Return/log class, account/user scope and capacity-exhausted reason without credential/payload details. Preserve HTTP429 compatibility and transient retry behavior; carry the specific rejection from Lua/scheduler rather than guessing at handler level.
- [ ] Add separate live/request counts while keeping aggregate current_concurrency exactly their sum. API-key statistics remain views of those same units.
- [ ] Test auth-cache invalidation and cross-provider behavior; run handler/middleware/service unit tests. Commit `feat: expose separate concurrency enforcement and diagnostics`.

## Task 6: Admin UI, API types and frontend artifact

**Files:** Active frontend account Create/Edit/BulkEdit modals, account/user views, API types and locale strings; corresponding supported `inferno-frontend` surfaces only where build provenance proves they are active. Add focused Vitest tests.

**Consumes:** Task2 write contract and Task5 counts. **Produces:** Reviewable controls for shared/split policy and honest separate occupancy.

- [ ] Verify which frontend build produces staging assets. Record package/build command and component paths; do not update only a dormant UI variant.
- [ ] Write UI tests: shared mode initially unchanged; split4/2 submits one complete object; unrelated edit omits policy; reset submits explicit shared mode; bulk edit changes only selected records; invalid values cannot submit.
- [ ] Add mode selector and Live/request inputs for accounts/users; show Live3/4, Requests1/2 and descriptive total4. Keep the legacy shared field visibly associated with shared mode.
- [ ] Test partial API responses/refresh and old accounts; preserve policy on imports/other edits and live display updates.
- [ ] In the active frontend run `pnpm typecheck`, `pnpm lint:check`, `pnpm test:run` and `pnpm build` using the existing package manager. Do not edit locks or add dependencies. Commit `feat: administer independent Live and request limits`.

## Task 7: Whole-branch verification and independent review

**Files:** Tests above and release/runbook documentation. No production configuration writes.

- [ ] Run from backend: `GOTOOLCHAIN=go1.27.0 go test -tags=unit ./...`, `GOTOOLCHAIN=go1.27.0 go vet ./...` and `golangci-lint run ./...` with a Go1.27-compatible linter. Report any tooling blocker explicitly.
- [ ] Run isolated Postgres/Redis integration suites and a multi-worker reservation test. Run frontend checks from Task6. Add no tests that call real phones automatically.
- [ ] Independent adversarial reviewer re-runs core policy/race/Live/scheduler tests, checks scope against this plan and verifies API/cache ownership. Repair every critical/high finding and repeat affected checks.
- [ ] Open/update a focused PR from the implementation worktree; do not merge. Keep shared policy default throughout this gate.

## Task 8: Staging rollout and simultaneous-load acceptance

**Operational artifacts:** Record commands and sanitized outputs under a new dated operations directory. Remote changes are limited to the dedicated staging instance and its new additive schema/policies.

- [ ] Recheck staging host architecture (`aarch64`), current binary/version, active calls, credentials/config hashes, account eligibility, helper health and DB backup. Capture current schema/policies/counters and resources. Do not copy credentials into reports.
- [ ] Build Linux ARM64 with the active embedded UI and exact implementation commit. Verify candidate SHA and execute `--version` on EC2 BEFORE stopping the existing gateway.
- [ ] Drain staging calls; deploy additive migration and compatible gateway/UI while policies are NULL. No Redis flush or replacement of DB/Redis/helper/tunnel. Verify all affected workers run the same new version, health/UI/API and shared-mode regression smoke.
- [ ] Enable Live4/requests2 on one controlled eligible account and split user limits sized for the test. Use existing authorized key and a bounded isolated test route/pool; avoid changing shared production group membership. Snapshot and restore temporary staging test policies/routing after acceptance unless explicitly selected as ongoing settings.
- [ ] Hold four real WebRTC Live sessions on that account. Verify voice/audio/sideband remain healthy while two ordinary requests are intentionally held in flight; verify fifthLive and thirdrequest deny/wait at the correct class/scope. Repeat with ordinary saturation first. Instrument Redis while requests are active, not just completed usage rows.
- [ ] Test user limits across accounts/keys, canceled hedge/request, failed creation, reconnect and teardown. Require zero test-owned leases after completion and no rising orphan count/429 in the other class. Preserve unrelated callers' counters.
- [ ] Record p50/p95 create, planner and speech-response latency under the controlled load, compare with shared-mode baseline, and investigate upstream429/errors before enabling more accounts. A new local allowance does not establish upstream capacity.
- [ ] No new handset call under this plan. If the user later requests one, perform a single explicit end-to-end Athena call and correlate its Live/request occupancy; never auto-redial.
- [ ] Final adversarial gate reviews actual simultaneous-load evidence, enabled final policies, UI/API round-trip and unchanged Air/production checks. Leave PR unmerged and report any unverified condition.

Rollback: disable split policies on the affected staging records, drain calls/leases, then restore the previous gateway/UI. Keep additive nullable columns; do not drop data or flush Redis. Never revert to shared-only workers while split policies remain enabled.

## Separate follow-up: exact per-call attribution

After the limiter is accepted, add bounded run-ID and purpose labels to Athena model requests and propagate them into Inferno diagnostics. This needs its own source/consumer review and tests; labels must never select the capacity class. It is not required for class-limit enforcement, but without it successful historical usage cannot prove exact per-call peak slots, especially for canceled hedges.

## Self-review and definition of done

- [ ] Scope coverage: Tasks1–2 policy/persistence/admin/cache; Task3 atomic enforcement; Task4 scheduler/Live ownership; Task5 user-wide paths/diagnostics; Task6 UI; Tasks7–8 review/real load/rollout.
- [ ] Review Focus failures each have a named owning task/test. No split default expansion and no old/new enforcement mix.
- [ ] Interface, ownership and patch naming are consistent across all tasks; resolve actual caller signatures before writing code rather than inventing adapters that bypass policy.
- [ ] Completion means simultaneous4Live+2ordinary proven, excess admissions rejected, cleanup verified, relevant checks passed, UI functional, reviewed commit/SHA recorded and production/Air untouched. Merely compiling or showing counters is insufficient.
