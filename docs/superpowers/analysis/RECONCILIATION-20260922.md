# Selective upstream reconciliation — 2026-09-22

This is the durable execution record for the reconciliation goal. It is kept in
the candidate worktree only; the protected baseline is never edited.

## Source snapshot

| role | ref | value |
|---|---|---|
| candidate worktree | `port/inferno-selective-upstream-20260922` | `6c35936bf23bfbbd5d1218c9a8e57f8542382b36` |
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

The next serial lane is the OpenCode/reasoning-effort/billing dependency group.
The isolated probe of `e47255715` was not promoted: it conflicts in the native
Anthropic forwarding files and depends on the upstream OpenCode builder and
account changes. The isolated probe of `db8692d67` likewise conflicts in
`backend/internal/service/ratelimit_cn_providers.go` because it assumes the
upstream OpenCodeGo `applyCNProviderReactive429` predecessor behavior. Neither
probe changed the candidate; both remain unintegrated until the dependency
group is reviewed as a unit.
