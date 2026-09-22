# Selective upstream reconciliation — 2026-09-22

This is the durable execution record for the reconciliation goal. It is kept in
the candidate worktree only; the protected baseline is never edited.

## Source snapshot

| role | ref | value |
|---|---|---|
| candidate worktree | `port/inferno-selective-upstream-20260922` | `49720bb4181d3ed386ef0fa37c2cafcc3c4b28c0` |
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

The next serial lane is the OpenCode/reasoning-effort/billing dependency group.
The isolated probe of `e47255715` was not promoted: it conflicts in the native
Anthropic forwarding files and depends on the upstream OpenCode builder and
account changes. It remains unintegrated until that dependency group is
reviewed as a unit.
