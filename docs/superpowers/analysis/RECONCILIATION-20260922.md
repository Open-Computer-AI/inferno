# Selective upstream reconciliation — 2026-09-22

This is the durable execution record for the reconciliation goal. It is kept in
the candidate worktree only; the protected baseline is never edited.

## Source snapshot

| role | ref | value |
|---|---|---|
| candidate worktree | `port/inferno-selective-upstream-20260922` | `24305a78a3b9b6da8be8e9edcb6c279e65da1238` |
| protected baseline | `baseline/gpt-live-working-20260922` | `7d3a6099bfa5d14d95253de7ac1864a9b330040e` |
| upstream | `upstream/main` | `1c0a69c0ceddb2fd21581c17ab09f6c500b89ba1` |
| rollback tag | `checkpoint/pre-reconciliation-8aa1e5de0` | candidate HEAD before this run |

The candidate was clean before this checkpoint. The protected baseline has
pre-existing untracked runtime artifacts; they are intentionally left alone.

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
| `c7343d2aa` | **TAKE** | `backend/internal/service/channel_monitor_checker.go`, `channel_monitor_checker_body_test.go` | Promoted as `dba38137`; focused Gemini monitor tests passed in the isolated lane, and candidate-side handler tests passed. Full service-suite image-model failures reproduce at the pre-backend checkpoint and are not touched by this change. |
| `38cfd7e2d` | **TAKE** | `backend/internal/handler/admin/{account_data.go,proxy_data.go,proxy_handler.go}`, `backend/internal/service/{admin_proxy.go,admin_service.go}` and focused tests | Promoted as `e5c2b50e`; focused proxy handler/service tests, `gofmt`, `go vet ./...`, and the isolated lane full backend suite passed. |
| `3fc08745a` | **TAKE** | `backend/internal/handler/model_plaza_handler.go`, `backend/internal/service/api_key_service.go` and focused visibility tests | Promoted as `8a71f145`; focused model-plaza/API-key visibility tests, `gofmt`, `go vet ./...`, and the isolated lane full backend suite passed. |
| `b8d52fad3` | **TAKE (bounded hand-merge)** | Backend reasoning-effort pricing contract, repository persistence, billing/account-stat calculations, channel/admin DTOs, and effort normalization | Promoted as `6987ec29`; the full upstream patch did not apply cleanly because its frontend pricing/admin and provider-forwarding portions cross local product surfaces. The bounded backend merge passed focused service/repository/handler tests, `go vet ./...`, and `git diff --check`; `e47255715` remains the serial provider-forwarding dependency. |
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
