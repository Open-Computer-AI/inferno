# Selective upstream reconciliation — 2026-09-22

This is the durable execution record for the reconciliation goal. It is kept in
the candidate worktree only; the protected baseline is never edited.

## Source snapshot

| role | ref | value |
|---|---|---|
| candidate worktree | `port/inferno-selective-upstream-20260922` | `eb6008f2464f92a450992a5a1c50d08dc9425af` (functional tip; this ledger update follows) |
| protected baseline | `baseline/gpt-live-working-20260922` | `7d3a6099bfa5d14d95253de7ac1864a9b330040e` |
| upstream | `upstream/main` | `1c0a69c0ceddb2fd21581c17ab09f6c500b89ba1` |
| rollback tag | `checkpoint/pre-reconciliation-8aa1e5de0` | candidate HEAD before this run |

The candidate was clean before this checkpoint. The protected baseline has
pre-existing untracked runtime artifacts; they are intentionally left alone.

The paired image-backfill/security lane is now promoted as candidate commits
`3f73489ed`, `83f567559`, and `71b47700d`. The source-to-candidate mapping is
recorded in `IMAGE-BACKFILL-DISPOSITIONS-20260922.tsv`. The image account toggle
was retained; unrelated request-ID UI context was omitted because the candidate
does not contain that state/helper contract. Backend image, URL-validator, and
HTTP-upstream focused tests passed; the two account-modal test files passed
81 tests and Vue typecheck passed. ESLint remains an environment blocker only:
the candidate dependency tree lacks the `vue-eslint-parser` symlink.

The latest backend audit also closed eight rows as `SKIP (already represented)`:
`a10ff1255`, `5fb5dfb34`, `222181efd`, `a77423066`, `9ad386569`, `01bd9b71a`,
`81fd85300`, and `9dc4c40bf`. Their behavior is already present in later
candidate commits; the exact paths and focused-test evidence are in
`BACKEND-LOW-RISK-DISPOSITIONS-20260922.tsv`.

Post-image full backend unit gate (`go test -tags=unit ./...`) is otherwise
green: all packages passed except the four baseline image-model tests
`TestOpenAIImagesResponsesDriverAndImageModels`,
`TestOpenAIImagesRejectedDriverDoesNotCoolImageModel`,
`TestGPTImage25PricingDoesNotUseLegacyImageRates`, and
`TestGPTImage25UsagePreservesImageInputTokens`. The same four failures were
reproduced before the image lane; no new failure class appeared.

A follow-up backend block closed six more rows as `SKIP (already represented)`:
`fdc2ec17e`, `5e4958c88`, `af90a9bd1`, `62198286e`, `e9e3c46cb`, and
`4c1f920d5`. These cover pricing reload, mapped-model scheduling, delegation,
gateway admission/error responses, proxy attribution, queue/timeout handling,
and raw-stream behavior already carried by later candidate commits.

The backend half of `28f673e4c` is promoted as `5a6c593af`: native Gemini
custom-model-list handling was hand-merged around the candidate's allowlist and
forced-Antigravity ordering. Its frontend GroupsView/locale changes remain out
of this lane and are reserved for the June-surface review.

The safe frontend mirror lane `df64b5f368` is promoted as `05da43ff8`. It only
changes account action-menu viewport positioning and its tests; 27 focused
Vitest tests, Vue typecheck, and diff checks passed.

The serial gateway/WS lanes are now promoted and cross-referenced in
`GATEWAY-WS-DISPOSITIONS-20260922.tsv`: model-not-found failover (`4bfeeafb0`),
later-turn quota recovery (`0697c857e`), and the ordered execution-scope
cluster (`07d807db4`, `67a8f705b`, `e73fc94f`, `f7f3479f`, `ee514e763`,
`eb6008f24`). The cluster passed its scope/isolation/preemption tests, the
`openai_ws_v2` package, `go vet`, and diff checks. A rollback tag was created
before the cluster at `checkpoint/pre-ws-execution-scope-20260922`.

## Current inventory

The current `upstream-daily.mjs --no-fetch` report covers the watermark window
starting at `5097b3145` and contains 406 non-empty upstream commits:

| provisional route | count | meaning before review |
|---|---:|---|
| `MERGE` | 254 | backend/mirror or other changes; inspect ancestry and behavior |
| `VERBATIM` | 5 | candidate route says the product copy is untouched |
| `NEW` | 11 | candidate route says no product counterpart exists |
| `REBUILD` | 136 | candidate route says the product copy carries June changes |

These are routing hints, not completion claims. Each commit must receive a
final `TAKE`, `HAND-MERGE`, or `SKIP` disposition with affected paths,
rationale, and observed verification evidence.

The broader candidate-to-upstream graph still has 561 upstream commits that
are not ancestors of the candidate. That graph fact is not a reason to bulk
merge: the candidate contains intentional OAuth, GPT Live, sideband, billing,
and June-product divergence.

## Execution lanes

1. **Backend non-core:** schema/migrations, provider compatibility, and
   admin/ops/domain changes with explicit path ownership.
2. **Upstream mirror/contracts:** only `frontend/**`; this mirror is kept
   current and is not the June product.
3. **June product:** only `inferno-frontend/**`; existing converted files are
   hand-merged and genuinely new counterparts are built in June idiom.
4. **Serial core/integration:** gateway/WS, OAuth replay, failover, billing,
   account continuation, merges, and final verification.

No two lanes may write the same file. `design-system/tokens` and
`design-system/components` remain protected unless a reviewed requirement
proves otherwise.

## Checkpoint protocol

Every integrated phase must record:

- branch/worktree and resulting commit;
- upstream source commits and final disposition;
- affected paths and why they were selected;
- commands actually run and exit status;
- remaining unreviewed rows and blockers.

The upstream watermark is not advanced until the candidate's backend and
pristine mirror are current and every selected row has evidence. A session or
usage interruption resumes from the latest passing checkpoint rather than
starting a second writer.

## Baseline gate snapshot

These results were collected at commit `c50447e7e`, before the next
reconciliation batch. They are evidence about the starting point, not a
completion claim:

| command | result | observed state |
|---|---|---|
| `cd backend && go test ./...` | pass | exit 0 |
| `cd backend && go vet ./...` | pass | exit 0 |
| `bash inferno-frontend/scripts/check-divergence.sh` | fail | 660 differing files; 228 declared; the command listed inherited undeclared paths |
| `node inferno-frontend/scripts/conversion-status.mjs --json` | pass | 211/352 converted (59.9%); 141 remaining |
| `node inferno-frontend/scripts/june-lint.mjs` | fail | 1,401 violations across 313 converted files; inherited baseline debt |
| `vue-tsc --noEmit -p inferno-frontend/tsconfig.json` | not runnable | candidate has no installed `inferno-frontend/node_modules` |

The upstream inventory JSON used for this checkpoint was generated with
`upstream-daily.mjs --no-fetch` and had SHA-256
`797d33b6519dc4dfaed026c19924020d92a2f96e6df3ac972d284c1af44ff883`.

## Integrated / disposition ledger (current)

| upstream source | disposition | affected paths | evidence / rationale |
|---|---|---|---|
| `7b4de8b6a` | **TAKE** | `backend/internal/service/content_moderation.go`, `content_moderation_input.go`, `content_moderation_reminder_test.go` | Cherry-picked as `49720bb4`; focused reminder-keyword tests and full `cd backend && go test ./...` passed. This closes a moderation bypass where reminder tags could evade keyword checks. |
| `18d483c2a` | **TAKE (already present)** | `backend/internal/repository/custom_group_usage_rollup_repo.go`, `usage_log_repo_group_summary_test.go` | Candidate files are byte-identical to the upstream commit tree; a no-commit cherry-pick produced no changes. Existing rollup-tail behavior was verified by the focused repository test. |
| `b252821c5` | **TAKE (already present)** | `backend/internal/service/account_usage_service.go`, `account_usage_service_batch_test.go` | Candidate files are byte-identical to the upstream commit tree; a no-commit cherry-pick produced no changes. Existing batch usage behavior was verified by the focused service test. |
| `647714353` | **SKIP** | `backend/internal/repository/plugin_kv_store_test.go`, `backend/internal/service/plugin_host_services_test.go` | Test-only errcheck cleanup targets files deleted intentionally in the candidate. A scratch cherry-pick produced modify/delete conflicts; restoring those tests would reintroduce a removed surface without runtime value. |
| `e009ea303` | **HAND-MERGE / TAKE** | `backend/internal/repository/http_upstream.go`, `http_upstream_http2_keepalive_test.go`, `http_upstream_http2_ping_test.go` | Promoted as `1ed0ba3a`; preserved local OpenAI transport compatibility while adding upstream’s mode-specific 15s OpenAI versus 10s/5s long-stream keepalive behavior and ping-frame coverage. Focused HTTP/2 suite passed. |
| `8e34ca5e3` | **TAKE (already present)** | `backend/internal/repository/account_repo.go`, `account_repo_integration_test.go`, `account_repo_temp_unsched_test.go`, `backend/internal/service/token_refresh_service_candidates_test.go` | An isolated no-commit cherry-pick against the candidate produced no working-tree changes, confirming the behavior is already represented without a duplicate commit. Focused repository OAuth-refresh candidate tests and service refresh-candidate test passed (exit 0). |
| `18bfa4bf2` | **TAKE (already present)** | `backend/internal/service/openai_chat_roles.go`, `openai_chat_roles_test.go`, `openai_gateway_chat_completions_raw.go` | Candidate already contains the behavior via local commit `d6d7902bc`; an isolated no-commit cherry-pick produced no working-tree changes. Focused strict-developer-role tests passed with `go test -tags unit ./internal/service -run 'TestForwardAsChatCompletions_StrictDeveloperRole|TestNormalizeStrictChatDeveloperRoles' -count=1` (exit 0). |
| `1a32b91eb` | **TAKE** | `frontend/src/components/common/Pagination.vue`, `frontend/src/components/common/__tests__/Pagination.jump.spec.ts` | Promoted as `6c35936b`; isolated cherry-pick applied cleanly, `git diff --check` passed, and focused Vitest passed: 1 file / 4 tests. The temporary scratch worktree used the existing candidate dependency tree; no lockfiles changed. |
| `d03c42d79` | **TAKE** | `frontend/src/components/modelPlaza/PlazaModelPricingTable.vue` | Promoted as `11f50eb9`; isolated cherry-pick applied cleanly with one presentational `table-fixed` → `table-auto` change. Focused plaza tests passed: 2 files / 33 tests; Vue typecheck and `git diff --check` passed. ESLint was not runnable because the scratch dependency tree lacks the `vue-eslint-parser` symlink; no lockfiles or June files changed. |
| `1cab4d8c2` | **TAKE** | `backend/internal/service/pricing_service.go` | Promoted as `918b35b9`; isolated cherry-pick applied cleanly. `TestPricingHotReload_*` passed in the isolated and candidate worktrees; `gofmt -d` and `go vet ./...` passed. No schema, gateway, OAuth, sideband, or account-auth paths changed. |
| `c76db386c` | **TAKE** | `frontend/src/views/admin/SubscriptionsView.vue`, `frontend/src/views/admin/__tests__/SubscriptionsView.userUsageLink.spec.ts` | Promoted as `fad3efd4b`; isolated cherry-pick applied cleanly. Focused Vitest passed 4/4 and Vue typecheck passed; no June files or lockfiles changed. |
| `fe36f4a91` | **TAKE** | `frontend/src/views/auth/RegisterView.vue`, `frontend/src/views/auth/__tests__/RegisterView.spec.ts` | Promoted as `86c12166a`; isolated cherry-pick applied cleanly. Focused Vitest passed 11/11 and Vue typecheck passed; existing password-confirmation behavior was preserved. |
| `55a95d4c6` | **DEFER / HAND-MERGE** | `frontend/src/i18n/locales/{en,zh}/dashboard.ts`, `frontend/src/utils/keyGroupProviders.ts`, `frontend/src/views/user/KeysView.vue`, `frontend/src/views/user/__tests__/KeysView.spec.ts` | Isolated focused tests passed 18/18, but Vue typecheck fails because the source introduces `minimax` in `Record<GroupPlatform, ...>`. The candidate also lacks MiniMax/OpenCode in platform types, catalogs, icons/colors, and backend group validation; full support is a separate broad contract (`19382f275`, `31f550738`, `1e2120269`). UI-only promotion would expose impossible choices, so the lane remains isolated. |
| `aa556c372` | **SKIP (already represented)** | `frontend/src/components/layout/AppSidebar.vue` and its test | Candidate ancestor `7c9627beda` has the same patch-id; reapplying the upstream sidebar change fails at the same hunk. No new June or design-system changes are needed. |
| `983a3db3a` | **DEFER (mixed backend/frontend)** | GPT-6 Astra model metadata, billing, aliases, account UI, and localization | This commit introduces the `upstream_model_metadata_partial` contract and overlaps the candidate's separate GPT-6 aliases plus account-modal/localization customizations. It must be reconciled as a serial backend capability-sync lane before any frontend warning/UI pieces are promoted. |
| `3e60c3b86` | **TAKE** | `backend/internal/handler/admin/account_handler.go`, `backend/internal/handler/admin/account_handler_list_test.go`, `backend/internal/handler/dto/{mappers.go,types.go}` | Promoted as `ffcbfa28`; isolated and candidate admin handler suites passed, including compact DTO/ETag and redaction assertions. The compact response omits credentials and nested groups while the legacy full response remains available. |
| `994192ef4` | **TAKE (dependent)** | `frontend/src/api/admin/accounts.ts`, `frontend/src/types/index.ts` | Promoted as `05124f29` after the backend DTO. Added `AccountListItem = Omit<Account, 'groups'>` and narrowed list/listWithEtag response types while preserving local account/referral/eligibility additions. Vue typecheck and existing AccountsView tests passed. |
| `db15e0090` | **TAKE (dependent)** | `frontend/src/views/admin/AccountsView.vue`, `frontend/src/views/admin/__tests__/AccountsView.lite.spec.ts` | Promoted as `be020af3`. Compact list uses `lite=1` and hydrates full account details before edit/test/stats actions. New lite tests passed 5/5; existing AccountsView tests passed 35/35; Vue typecheck and diff check passed. |
| `1ee929e4b` | **TAKE (serial channel/cache lane)** | `backend/cmd/server/wire_gen.go`, `backend/go.sum`, `backend/internal/repository/{channel_cache.go,channel_cache_test.go,wire.go}`, `backend/internal/service/channel_service.go` plus affected service/handler test fixtures | Promoted as `1448e58e`; Redis channel-cache pub/sub invalidation is wired without touching OAuth, gateway, WS, sideband, or frontend paths. Channel-cache/repository/service focused tests, pricing SQL contract tests, reasoning-effort extraction tests, `gofmt`, `go vet ./...`, and diff checks passed. The isolated full `go test -tags=unit ./...` passed every package except four pre-existing OpenAI image-model tests, each reproduced on the candidate before this lane. |
| `c74a4f521` | **SKIP (already present/superseded)** | `backend/internal/service/billing_service.go`, `backend/internal/service/billing_service_test.go`, `frontend/src/composables/useModelWhitelist.ts` | The candidate already contains the GLM-5.3/GLM-5.3-Flash fallback prices, most-specific-first matching, regression cases, and whitelist entries through local `073910eae`/`9b1673a10`-era work. `TestGetFallbackPricing_FamilyMatching` passed; reapplying the upstream patch would overwrite newer billing/frontend changes. |
| `185951957` | **TAKE (independent deploy lane)** | `deploy/.env.example`, `deploy/APPLE_CONTAINER.md`, `deploy/apple-container.sh`, and Apple-container fixture tests | Promoted as `3435e6bf4`; the isolated fake-container lifecycle test passed through create, restart, stop/delete, and persistent-volume scenarios. This lane has no backend OAuth, gateway, WebSocket, sideband, billing, or frontend overlap. |
| `f5e4da5` | **TAKE (formatting-only)** | `backend/internal/service/http_upstream_profile.go` | Promoted as `1bcdd1475`; gofmt, go vet for the service package, focused transport checks, and diff check passed. No runtime or authentication behavior changed. |
| `8ed57b000` | **TAKE (bounded hand-merge)** | Gemini 3.7 Flash model constants/catalog, account passthrough, billing fallback, and regression tests | Promoted as `6734e812d`; the candidate already had the broader Gemini 3.6/3.7/3.8 pricing alias implementation, so the upstream `pricing_service.go` hunk was intentionally not reapplied. Focused service/domain/Antigravity tests, `go vet ./...`, and diff check passed. |
| `a3a6a85b7` | **TAKE (test-only)** | `backend/internal/handler/admin/account_handler_list_test.go` | Promoted as `e5d57098e`; nested credential redaction and `has_access_token` assertions pass in the compact admin list test. No production behavior changed. |
| `163eb7ffa` | **TAKE (bounded ops/admin lane)** | Database-backed system-log retention/runtime bounds plus the admin ops table/API/locales | Promoted as `ca56e344e`; isolated backend config/ops tests, `gofmt`, `go vet`, frontend OpsSystemLogTable tests (4/4), `vue-tsc`, and diff checks passed. No OAuth, GPT Live, sideband, WebSocket, billing, or provider-failover paths changed. |
| `983a3db3a` | **TAKE (serial Astra capability-sync lane)** | GPT-6 Astra model catalog, Codex descriptors, billing/aliases, per-model upstream capability snapshots, partial-metadata warnings, and account UI/locales | Promoted as `0d48568ed`; conflict resolution preserved the local bare `gpt-6` alias and added dated Astra IDs. Focused sync/Codex tests, `go vet ./...`, 21 frontend tests, `vue-tsc`, and diff checks passed. No June `inferno-frontend` or design-system files changed. |
| `c7343d2aa` | **TAKE** | `backend/internal/service/channel_monitor_checker.go`, `channel_monitor_checker_body_test.go` | Promoted as `dba38137`; focused Gemini monitor tests passed in the isolated lane, and candidate-side handler tests passed. Full service-suite image-model failures reproduce at the pre-backend checkpoint and are not touched by this change. |
| `38cfd7e2d` | **TAKE** | `backend/internal/handler/admin/{account_data.go,proxy_data.go,proxy_handler.go}`, `backend/internal/service/{admin_proxy.go,admin_service.go}` and focused tests | Promoted as `e5c2b50e`; focused proxy handler/service tests, `gofmt`, `go vet ./...`, and the isolated lane full backend suite passed. |
| `3fc08745a` | **TAKE** | `backend/internal/handler/model_plaza_handler.go`, `backend/internal/service/api_key_service.go` and focused visibility tests | Promoted as `8a71f145`; focused model-plaza/API-key visibility tests, `gofmt`, `go vet ./...`, and the isolated lane full backend suite passed. |
| `b8d52fad3` | **TAKE (bounded hand-merge)** | Backend reasoning-effort pricing contract, repository persistence, billing/account-stat calculations, channel/admin DTOs, and effort normalization | Promoted as `6987ec29`; the full upstream patch did not apply cleanly because its frontend pricing/admin and provider-forwarding portions cross local product surfaces. The bounded backend merge passed focused service/repository/handler tests, `go vet ./...`, and `git diff --check`; its dependent `e47255715` forwarding portion is recorded separately as `24305a78`. |
| `e47255715` | **TAKE (bounded hand-merge)** | Backend final-effort propagation through Gemini, native Anthropic, fallback, media, and usage-billing paths plus focused pricing tests | Promoted as `24305a78`; native Anthropic conflicts were resolved against the candidate's existing six-argument builder and returned sanitized body, preserving local sideband behavior. Focused native/fallback/Gemini/image tests and `go vet ./...` passed. The upstream OpenCode-Go session-header extension was intentionally not imported. |
| `1a32b91eb` | **TAKE (June surface)** | `inferno-frontend/src/components/common/Pagination.vue`, `Pagination.jump.spec.ts` | Promoted in June commit `de9df9d0e`; this is separate from the shared `frontend/**` pagination promotion above. June focused Vitest and typecheck passed in isolation. |
| `130ba634a` | **TAKE (June surface)** | `inferno-frontend/src/composables/{useClipboard.ts,__tests__/useClipboard.spec.ts}` | Promoted in `de9df9d0e`; June focused Vitest and typecheck passed in isolation. |
| `98321a054` | **TAKE (June surface)** | `inferno-frontend/src/components/payment/{AmountInput.vue,__tests__/AmountInput.spec.ts}` | Promoted in `de9df9d0e`; June focused Vitest and typecheck passed in isolation. |
| `f8e5a0d94` | **TAKE (June surface)** | `inferno-frontend/src/components/admin/channel/{ModelTagInput.vue,__tests__/ModelTagInput.keyboard.spec.ts}` | Promoted in `de9df9d0e`; June focused Vitest and typecheck passed in isolation. |
| `406be7c51` | **TAKE (June surface)** | `inferno-frontend/src/stores/{subscriptions.ts,__tests__/subscriptions.clear.spec.ts}` | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `6a4938bdf` | **TAKE (June surface)** | `inferno-frontend/src/stores/announcements.ts`, `announcements.markAll.spec.ts` | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `21532add4` | **TAKE (June surface)** | `inferno-frontend/src/views/admin/order/OrderList.vue` | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `b9d072868` | **TAKE (June surface)** | `inferno-frontend/src/views/model-plaza/ModelPlaza.vue` | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `d03c42d79` | **TAKE (June surface)** | `inferno-frontend/src/views/auth/RegisterView.vue` | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `6c8ad0bd4` | **TAKE (June surface)** | `inferno-frontend/src/views/auth/RegisterView.vue`, related registration visibility test | Promoted in `e67948d3`; June focused Vitest and typecheck passed in isolation. |
| `a16070ccf` | **TAKE (June surface)** | `inferno-frontend/src/components/TurnstileWidget.vue`, `TurnstileWidget.spec.ts` | Promoted in `32a9c0dd`; June focused Vitest, ESLint, Vue typecheck, and build passed in isolation. |

The full 40-character backend source/candidate mapping, including the quota-
403 skip and the reasoning/billing hand-merge dependency, is recorded in
`docs/superpowers/analysis/BACKEND-LOW-RISK-DISPOSITIONS-20260922.tsv`.

### Frontend mirror lane

The isolated `lane/frontend-mirror-audit-20260922` was promoted as the
candidate range `7c9627beda..60e0a5e3e`. It contains 36 upstream frontend-only
commits; the source-to-candidate mapping and one-row disposition for every
commit are recorded in
`docs/superpowers/analysis/FRONTEND-MIRROR-DISPOSITIONS-20260922.tsv`.

The lane changed only `frontend/**`: no backend, `inferno-frontend/**`,
design-system token/component, lockfile, or generated output paths were
changed. The changed-test set passed 32 files / 223 tests, ESLint passed,
Vue typecheck passed, `git diff --check` passed, and a conflict-marker review
was clean. The production build remains a serial gate because both frontend
surfaces emit the shared embedded `backend/internal/web/dist` output.

The pagination source was handled separately above. The batch-image
fix/revert pair was deferred because the candidate already has the fixed
runtime behavior, and mixed/customized frontend commits remain reserved for
the June product lane rather than being copied into the mirror.

After that mirror batch, three bounded shared-frontend fixes were promoted:
deleted-user filtering in the admin subscription assignment search
(`c76db386c` → `fad3efd4b`), registration promo-code flash prevention
(`fe36f4a91` → `86c12166a`), and the plaza pricing table width fix above.
The provider-filtered API-key groups change (`55a95d4c6`) remains isolated:
its runtime tests pass, but the candidate's declared `GroupPlatform` union
does not yet include the new `minimax` entry required by the source mapping.
That contract mismatch must be reconciled before any promotion.

The independent backend pricing hot-reload fix (`1cab4d8c2` → `918b35b9`)
was also promoted after isolated and candidate `TestPricingHotReload_*`,
`gofmt`, and `go vet ./...` checks passed. It does not touch the gateway,
OAuth, sideband, schema, or account-auth surfaces.

### June product lane

The isolated `lane/june-inferno-frontend-20260922` was promoted in three
bounded commits: `de9df9d0e`, `e67948d39`, and `32a9c0ddb`. The exact
source-to-candidate mapping is recorded in
`docs/superpowers/analysis/JUNE-INFERNO-FRONTEND-DISPOSITIONS-20260922.tsv`.
All changes are confined to `inferno-frontend/**`; no shared API/types,
router/i18n, design-system, backend, lockfile, or generated paths changed.

The isolated lane passed 34 focused tests across 7 files, ESLint on touched
files, Vue typecheck, `vue-tsc -b && vite build`, conversion-status, behavior
parity, and `git diff --check`. June lint still reports the inherited 1,401
violations across 313 converted files; its category totals did not change.
Higher-risk account-selection/admin and larger payment/sidebar rewrites remain
deferred for serial review because they overlap OAuth, GPT-Live, or customized
product surfaces.

The bounded backend reasoning-effort contract from `b8d52fad3` is promoted as
`6987ec29`, and its final-effort propagation dependency `e47255715` is promoted
as `24305a78`. These bounded merges deliberately exclude the upstream frontend
pricing/admin patch and OpenCode-Go session-header extension; those surfaces
still require serial review against the local OAuth, sideband, and June-product
behavior. The isolated probe of `db8692d67` remains skipped because it conflicts
in `backend/internal/service/ratelimit_cn_providers.go` and assumes the
upstream OpenCodeGo `applyCNProviderReactive429` predecessor behavior. Neither
that quota-403 behavior nor the OpenCode-Go-only pieces changed the candidate.

### Gateway/WS cyber-policy lane

The serial cyber-policy recording fix (`2da31290a1` → `b090e2aff`) is promoted
after an isolated cherry-pick onto the current WS execution-scope candidate.
It centralizes cyber-policy marking for both `error` and `response.failed`
event shapes across ingress, WS v2, the HTTP bridge, and passthrough paths,
and preserves the per-logical-turn recorded guard while account failover is
still in progress. Focused handler/service Cyber/OpenAIWS/HTTPBridge/Passthrough
tests passed, as did `go vet ./internal/handler ./internal/service` and
`git diff --check`. The full source/path/evidence row is in
`docs/superpowers/analysis/GATEWAY-WS-DISPOSITIONS-20260922.tsv`.

### June/custom GroupsView lane

The Codex-manifest edit-state fix (`b1ce821c4` → `5a51306a1`) is promoted as
a hand-merge because `frontend/src/views/admin/GroupsView.vue` is locally
customized. Direct `v-model` was replaced with explicit `model-value` and
`Object.assign`, preserving the reactive object identity across consecutive
child updates. The upstream regression test was adapted with the candidate's
existing group-allowlist API mock (`2162e6c43`); the focused GroupsView,
CodexManifestAccountsField, and duplicate suites passed 10/10, and
`vue-tsc --noEmit` passed. The merge wrapper `65246e69d` is recorded as a
skip because it adds no behavior beyond `b1ce821c4`.

### Account expiry preset lane

The independent account-expiry preset change (`00eabe8ab` → `5bcb2f796`) is
promoted after an isolated cherry-pick. It adds one-month/one-year calendar
presets to the shared create/edit account modals, clamps month-end dates, and
keeps manual editing and clearing semantics intact. Create/edit/helper tests
passed 90/90, `vue-tsc --noEmit` passed, and `git diff --check` passed. No
backend, OAuth, WebSocket, June `inferno-frontend`, design-system, or lockfile
paths changed.

### Codex ultrafast service-tier lane

The mixed ultrafast capability (`126ac24c8` → `1cbbddc04`) is promoted as a
bounded lane. It adds `service_tier=ultrafast` validation, GPT-5.6 Sol catalog
and routing hints, the matching 2x billing multiplier, policy/settings support,
and shared usage/localization display. Backend service/admin/handler tests and
the four touched frontend suites (36 tests) passed; `vue-tsc --noEmit` and
`git diff --check` passed. It does not touch OAuth, GPT Live, sideband,
WebSocket execution scope, the June `inferno-frontend`, schema, or lockfiles.
The merge wrapper `c42d78e2` is recorded as a skip because it contributes no
independent patch.

The broader GroupsView/toggle refactor (`f3bbb9531`) remains deferred for
hand-merge. It removes 471 lines and crosses locally customized model-list,
model-allowlist, and asynchronous `allow_live` behavior; applying it wholesale
fails at a customized GroupsView hunk. Its exact paths and rationale are
recorded in the frontend disposition manifest; only the narrow reactive
binding fix from `b1ce821c4` is currently promoted.

### Pinned-manifest/search wrapper audit

The pinned-account merge wrapper (`c4e6dcfd8`) is skipped: its non-migration
payload is byte-for-byte represented by candidate `c0d48976e`, while migration
`234_group_codex_models_manifest_config.sql` is already present from
`f185d110c`. Replaying the wrapper would overwrite later local/Astra/Ultrafast
changes in the generated Ent, Codex service, and customized GroupsView files.
The search-capability wrapper (`eba6ea563`) is likewise already represented;
its service logic and metadata regression test are present, with the test blob
identical and focused Codex/search tests passing.

### Upstream request-ID feature deferred

The broad usage-log/upstream-header feature (`de27905e8`) and its follow-up
(`708b85a6a`) remain a deliberate serial hand-merge item. The candidate has the
232/233 migration files but not the upstream `upstream_request_id.go` module or
the feature's 62-file wiring; its current gateway/usage request-ID handling is
custom. An isolated cherry-pick of `708b85a6a` conflicts on the missing module.
No partial schema/UI/backend feature was promoted. The full lane needs one
design pass over usage-log inserts, account validation, all gateway response
paths, migration/runtime verification, and the admin account/usage UI before
it can be safely accepted.

### Grok settings clarification lane

The narrow settings clarification (`32bf3d0f3`) is promoted as `d4d38f0eb`.
The backend default was already `true`; the patch corrects the misleading
comment and updates the English/Chinese admin hints to say that cross-client
mapping is enabled by default. Service tests, vet, locale compilation/key
collision tests (8/8), Vue typecheck, and diff checks passed.

The adjacent frontend audit found no additional code to promote for the
Turnstile loading placeholder (`a16070ccf`), registration password
confirmation (`017cc62e4`), onboarding shadow removal (`3aeab296d`), or Gemini
monitor user role (`c7343d2aa`): equivalent candidate patches are already
present. The pinned model-list locale/backend bundle (`cb3103397`) remains a
hand-merge item after backend contract parity.

### Payment/redeem safety lane

`7a70de401` was kept separate from the settings/frontend lanes because it
changes the admin redeem handler and payment fulfillment/rate-limit path. It is
now promoted only after its serial payment-safety tests passed.

The payment lane is now promoted as `a5cd2f2ac` plus the candidate-specific
test adaptation `230e770e9`. Trusted payment/admin fulfillment bypasses only
the public redeem failure counter, while code/type/amount/status/user checks
fail closed before crediting. Focused service/admin tests and vet passed. The
candidate's existing `redeemMaxFailedAttempts=30` policy was preserved; only
the upstream tests' stale constant name was adapted.

The OpenCode session-forwarding merge wrapper (`620eb3fd0`) is skipped because
candidate `b2906435e` already contains the exact new helper/test blobs and
forwarding calls. No duplicate replay is needed.

### Backup/migration serialization lane

The backup lock fix (`95023e7d4` → `fe7743709`) is promoted. The dumper now
holds the migrations advisory lock for the complete streaming `pg_dump`/`psql`
lifecycle and discards ambiguous SQL sessions on lock/unlock failure. Focused
repository tests, repository/server vet, and diff checks passed; no schema,
data, inference, OAuth, or container state was changed.
