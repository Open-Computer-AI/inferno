# Scope: independent Inferno Live and ordinary-request concurrency

Status: proposed design, based on code inspection. No implementation, migration, configuration edit, call or deployment performed for this scope.
Source examined: the exact deployed staging source at c8c64ad765dc45afaf55423bb42c9a571e89c72c, based on Air production ca6af9af6d9fae1b9888d9038b2239f58cfd00ed. Worktree: /Users/saksham/OpenComputerV2/inferno-stage-air-parity-20261018.

## Objective and contract

Give Live sessions and ordinary model requests independent, bounded capacity on the same upstream account. Example configuration, not a validated upstream quota: Live4 and ordinary2 permits four Live sessions plus two ordinary model requests. A fifth Live session must be denied even when ordinary capacity is free; a third ordinary request must wait or be denied even when Live capacity is free. No borrowing between these budgets in the first version.

A Live session includes its sideband, in-flight creation and cleanup until finalization. Its sideband/reconnect does not acquire a second lease. Classifiers, action planners, checks and post-call model processing are ordinary requests; an external tool call alone is not an Inferno request. Multiple simultaneous ordinary requests each count once. This does not make auxiliary requests free or exempt them from upstream account limits.

Use a server-derived workload class, not a client-controlled label. The Live handler/capability identifies Live. Supported non-Live gateway operations retain ordinary semantics. Ordinary includes more than text if those operations already use the ordinary limiter; do not invent model-name tests or redesign other providers. HTTP/WebSocket Responses turns must obey the ordinary budget; idle Responses WebSocket ingress remains a separate existing connection limit.

## Recommended configuration design

Provide an explicit opt-in split policy at both account and gateway-user levels. Keep the existing shared concurrency field and behavior for unconfigured accounts/users. New policy stores two finite positive limits: live_concurrency and request_concurrency. Treat the two optional fields as one policy: both present enables split mode, both absent retains legacy shared mode. Reject half-configured/negative/zero split policies rather than silently interpreting zero as unlimited. Disabling a workload per account is not a zero-limit feature in v1; existing eligibility/configuration remains in force.

Recommend nullable typed fields on accounts and users, with an additive SQL/Ent migration, to make the admin API, validation and persistence explicit. Account.Extra could avoid account schema changes, but users have no equivalent Extra field; using unrelated global settings as a per-user policy store would introduce a second configuration system. The typed approach is broader than a JSON-only prototype and is preferable for the complete account+user feature. Final field naming remains an implementation detail to check against API conventions.

No automatic conversion of existing concurrency4 into Live4+ordinary4: that would silently increase total admitted work. Default migrated fields to NULL. Set staging limits explicitly only after validation. Live4/request2 is the initial test scenario, not an upstream guarantee or a final sizing decision. Per-user budgets must be deliberately sized for eligible account pools and other API-key users of that identity. The ordinary user budget covers every non-Live request for that user, including other API keys/clients; it is not an Athena-classifier-only reservation. Isolating Athena from other applications would require separate user/pool policy and is a different feature.

Do not silently retain the old shared account/user field as an additional binding cap in split mode: that would defeat the promised independence. If a future overall safety cap is desired, expose it explicitly as a separate documented feature. Legacy users may continue shared mode until opted in; this intentionally retains their shared bottleneck.

## Required implementation areas

### 1. Persistent policy and supported administration

- Account/user service types, repositories, nullable policy fields and additive migration.
- Create/update/bulk-edit API schemas, validation and DTO mappers; preserve unknown/omitted policy on unrelated edits.
- Account export/import/duplicate and admin CLI paths must round-trip or deliberately preserve the new policy. Existing concurrency-only commands must not implicitly enable/clear split mode.
- User batch-limit edits, redeem-code/plan concurrency adjustments and defaults must retain their current meaning for legacy fields; do not automatically rewrite split caps or change purchased entitlements.
- Include policies in scheduler snapshots and authentication/user caches, with invalidation on changes. AuthSubject currently carries only shared Concurrency; all supported inference-auth paths need correct policy resolution, including API-key and OAuth inference paths.

Relevant files include backend/ent/schema/account.go, user.go; backend/internal/service/account.go, user.go; repository/account_repo.go, user_repo.go; handler/admin/account_handler.go, account_data.go, user_handler.go; handler/dto/types.go, mappers.go; server/middleware/auth_subject.go, api_key_auth.go, oauth_inference_auth.go; scheduler/auth cache mapping code. Regenerate Ent through the repository's generation workflow when implementing, not by hand-editing generated files.

### 2. Atomic class-aware Redis admission

Redis already distinguishes ordinary and Live account/user sets. Retain them so existing leases and cleanup mechanisms remain observable.

- In shared mode, continue comparing ordinary+Live counts to the legacy cap.
- In split mode, compare only the appropriate class count to that class cap.
- Evaluate account and user policy independently. Split account + shared user: class-specific account cap and combined user cap. Shared account + split user: combined account cap and class-specific user cap. Reject a half-configured pair atomically and preserve the prior valid policy. Policy edits affect new admissions; existing leases keep their original class.
- Every acquire remains atomic across competing gateway workers. A read-then-write capacity check is not sufficient.
- Preserve TTL renewal, idempotent release, cancellation cleanup, stale-lease behavior and Redis time semantics. Enforce caps on in-flight creations, not only successful sessions. Live restart recovery currently relies on lease TTL/refresh rather than ordinary process-slot cleanup; test crash/restart and bounded expiry for any new reservation.
- Preserve API-key tracking as statistics; do not add account+user+key views as separate slot units or introduce a new key-level quota in this change.
- Review account aliases/shadow-account resolution wherever existing acquisition already resolves them; preserve existing identity semantics rather than bypassing parent-account rules.

Core files: repository/concurrency_cache.go and service/concurrency_service.go, plus their interfaces/mocks/tests.

### 3. Class-aware scheduler and waiting

Changing acquisition Lua alone is insufficient. Existing scheduler filtering, balancing and wait logic uses combined load and account.Concurrency.

- Pass workload class through capability selection, sticky/previous-response paths, primary/fallback selectors, scheduling load reads and slot acquisition.
- Live selection must use Live occupancy/limit; ordinary selection must use ordinary occupancy/limit for split accounts.
- Handle load factor/scoring without using a scheduling preference as a hard admission cap.
- Update wait plans, queue-full handling, fresh-load fallback and batch load caching. The workload class and relevant policy must participate in cache identity; Live load must not be reused as ordinary load.
- Preserve model/platform/group compatibility, proxy choice, authentication, account health/quarantine, quota gates, privacy, failover and cooldown behavior.
- All existing shared-mode paths must retain current behavior, including other providers that call common concurrency interfaces. Once a user is explicitly in split mode, every supported inference route for that user must consistently count as Live or ordinary; an unchanged provider handler must not bypass the new user request cap by continuing to supply only the legacy48-style limit. Prefer the common authentication/concurrency helper boundary over provider-specific duplicated logic. Limit account-level split enablement to supported Live-capable accounts in v1.

Core files: service/openai_account_scheduler.go, openai_gateway_scheduling.go, concurrency_service.go; scheduler cache/load structures and gateway handler wait helpers.

### 4. Live lifecycle entry and conversion

Today Live admission first takes an ordinary user slot and scheduler account slot, then takes a Live lease using a replacing allowance. This can reject Live creation merely because ordinary capacity is full, even after the final Live budget is split.

- One admission owner: the Live-aware scheduler selects an eligible Live-capable account and atomically reserves the account/user Live lease. The Live HTTP handler must not first acquire an ordinary user slot; the scheduler must not acquire an ordinary account slot for a Live request. Use this Live reservation path with either shared or split policies: shared mode retains its combined-count caps, not the old temporary double-count mechanics.
- Use one bounded Live reservation for creation, carry it into the successful session and release on creation failure/cancellation. No gap permitting the fifth simultaneous creation; no duplicate Live lease on success.
- Return the exact reservation/lease ID and release ownership with the selected account. CreateLiveCall adopts it once rather than reacquiring Live capacity. Eliminate the replacing allowance on this path because there is no ordinary slot to replace. Advanced and legacy/disabled-scheduler selectors must both support this same ownership contract. This changes admission plumbing while preserving legacy shared-cap semantics.
- Continue requiring both account and user admission. An abandoned partial acquisition must release what it acquired.
- Sideband attach, reconnect and observer takeover must not take another Live reservation.
- Preserve existing session-ended/terminal-404 cleanup and the current accounting/teardown duration behavior.

Core files: handler/openai_live.go; service/openai_live.go, openai_live_types.go and lifecycle interfaces/tests.

### 5. Admin UI and visibility

- Create/edit/bulk-edit controls expose shared versus split mode and both caps clearly.
- Account/user tables and detail views show Live current/limit, ordinary current/limit and an optional descriptive total. For example: Live3/4, requests1/2, total4; never present total4 as 'Live4'.
- Preserve existing current_concurrency as a total for compatibility; add per-class fields rather than silently changing its meaning.
- Keep API-key counts as a breakdown of the same requests, not another sum.
- Policy changes invalidate snapshots and refresh the display. Typecheck and focused UI tests cover payload round-trip and clearing a policy back to shared mode.
- Repository contains frontend and inferno-frontend variants. Verify which build produces the deployed assets before choosing implementation targets; keep any actively supported admin variant compatible. Rebuild/deploy the selected frontend for this feature rather than reusing old assets that cannot display the new controls.

Targets include frontend/src/components/account/CreateAccountModal.vue, EditAccountModal.vue, BulkEditAccountModal.vue; views/admin/AccountsView.vue, UsersView.vue; frontend API types and equivalent supported inferno-frontend files.

### 6. Actionable errors and telemetry

Differentiate Live account capacity, Live user capacity, ordinary account capacity and ordinary user capacity in machine-readable error/metric fields, while preserving compatible HTTP429/retry behavior. A generic 'Live concurrency limit reached' cannot diagnose which gate bound.

Record current/limit per class, waits, rejects, lease age and cleanup failures. Keep request payloads, audio, SDP, credentials and proof bodies out of telemetry.

Recommended small follow-up in Athena: attach run ID and request purpose (planner, classifier/check, post-call) to requests and propagate them into Inferno's usage/lease diagnostics. These are diagnostic labels only, never inputs that can grant a different concurrency class or evade authorization. This is required for exact per-call attribution, but the limiter can be implemented/verified independently.

## Acceptance and validation

1. One split account configured Live4/requests2 admits four Live sessions and two ordinary requests concurrently; does not admit a fifth Live or third ordinary request, regardless of spare capacity in the other class.
2. Saturate ordinary budget first: Live creation still succeeds up to its own cap. Saturate Live first: ordinary requests still succeed up to their cap.
3. Mirror tests for user caps across multiple accounts and API keys. Shared-mode fixtures remain unchanged; mixed shared/split account/user policy works as documented.
4. Concurrent acquire race, duplicate acquire, failed/canceled creation, retries, response stream cancellation, normal hangup, abrupt disconnect, terminal404/410, observer takeover and gateway restart do not leak/double-release slots.
5. Sideband reconnect never consumes another Live slot. Classifier hedges count independently and release the losing attempt on cancellation.
6. Scheduler and wait-plan tests cover class-aware sticky/load-balance/fallback behavior, full-class exclusion, cache key separation and no accidental unlimited behavior.
7. Admin create/edit/bulk/import/export/CLI preserves policy, rejects partial invalid policies and leaves unrelated settings untouched. Frontend displays true separate occupancy.
8. Meaningful Redis integration/parallel tests prove atomic caps; Go unit, vet and repository-supported lint run. Frontend uses documented typecheck, lint:check and test:run commands. Installed older linter limitation must be resolved or explicitly reported, not bypassed.
9. Isolated/synthetic load test holds four real WebRTC sessions plus bounded planner/classification traffic against staging, verifies audio/context/tool behavior, stable latency and no cross-class rejections or leaked leases. Provision eligible test-account capacity and check backend resources first; do not use new handset calls as load generators.
10. At least one properly authorized real Athena call validates the end-to-end platform path after synthetic acceptance, with telemetry correlation and separate slot observation. Permission for earlier phone tests is not a standing instruction to redial during a future implementation.

## Rollout

- Implement in a fresh worktree from deployed staging commit c8c64ad76, with focused commits/PR and independent adversarial review.
- Add nullable schema fields and deploy new code in shared-compatible mode first. Do not auto-enable policies.
- Staging only; do not change the Air, its attestation helper, production database, production domain or other AWS services.
- Because old gateway binaries enforce shared counting, do not run old/new enforcement together while enabling split mode. Drain active staging calls, replace all affected staging gateway workers, verify version/policy caches, then opt selected accounts/user into split mode.
- Never flush Redis or erase live leases to switch mode. Keep counters truthful and verify active calls finish/leases settle before policy transitions. Before reverting to an old gateway binary, disable split policies and drain in-flight calls so old shared enforcement cannot silently reinterpret the enabled configuration.
- Start with the explicit example budget only on a controlled eligible test account/user. Verify upstream behavior under combined load before broader enablement. Separate local budgets do not isolate provider auth/quota/rate limits or create upstream capacity.
- Production rollout and Air upgrades are separate later work, not part of this staging scope.

## Explicitly outside this change

Remote attestation/helper/tunnel, billing/duration semantics, removing the observed post-disconnect cleanup delay, changing classifier prompts or disabling checks/hedges, global provider account quota/rate policy, Athena organization call limits/SIP concurrency, new public APIs for customers and automatic upstream capacity increases. These may still limit total call throughput after the shared slot coupling is removed.

## Completion evidence

Reviewed source commit and deployed binary SHA; enabled account/user policies; per-class Redis occupancy and rejected-limit evidence; true simultaneous4Live+2ordinary test; release/cancel/restart evidence; frontend screenshots and API round-trip; unaffected production/Air checks; complete test outputs; adversarial review; PR left unmerged unless separately authorized.

## Independent scope review

Planning PASS after review of admission ownership, mixed-policy semantics, user-wide enforcement, scheduler class propagation/cache identity, Redis compatibility and crash/restart behavior. These are implementation acceptance gates, not claims that the feature has been built or load-tested.
