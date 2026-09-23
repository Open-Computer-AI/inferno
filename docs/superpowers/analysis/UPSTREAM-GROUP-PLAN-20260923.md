# Current upstream-to-June frontend reconciliation

Snapshot: `upstream/main` = `5e244e7382bbb100ab2a4579b1078a79e6124c40`.
Candidate: `/Users/saksham/OpenComputerV2/inferno-port-20260922`, branch
`port/inferno-selective-upstream-20260922`. Protected baseline remains
`/Users/saksham/OpenComputerV2/inferno-local` at
`7d3a6099bfa5d14d95253de7ac1864a9b330040e`.

## What “195 rows” means

The 195 figure was the prior upstream snapshot, not 195 features or 195 pages.
At the refreshed tip there are 203 upstream commits that touch `frontend/`,
with 794 commit-to-file touches across 340 unique standard-frontend paths.
The untouched standard `frontend/` mirror in the candidate is byte-for-byte
equal to `upstream/main`; the manual queue is deciding whether/how those source
behaviors are already satisfied or need adaptation in `inferno-frontend/`.

The current router classifies those 203 rows as:

| Route | Rows | Meaning |
|---|---:|---|
| VERBATIM | 7 | June counterpart is still the upstream copy; copy is potentially safe after lineage/checks |
| NEW | 11 | June has no file counterpart; create/adapt the feature |
| REBUILD | 185 | June changed the source file; inspect behavior and hand-adapt or record existing equivalent |

Across the wider 540-commit source range, 337 rows are MERGE-only (backend or
other files, no June frontend task). The route classifier is conservative; a
REBUILD is not proof of a missing feature. An equivalent June behavior should
be recorded as such rather than reimplemented.

## Fast batching map

This is a mutually exclusive keyword grouping for scheduling the review, not a
claim that feature ownership never overlaps. Source diffs and actual June
consumers still decide the disposition.

| Behavior batch | Rows | Rebuild | New | Verbatim | Risk/order |
|---|---:|---:|---:|---:|---|
| Accounts, OAuth, selection | 71 | 68 | 2 | 1 | First-class, serial; preserve Inferno OAuth, account routing, sideband, continuation |
| Users, API keys, groups | 53 | 44 | 6 | 3 | High; group related forms, API contracts, and stale-response races |
| Billing, pricing, channels | 27 | 25 | 0 | 2 | High; verify backend/UI field round-trip and exact price semantics |
| Admin, ops, settings, backups, affiliates | 28 | 26 | 1 | 1 | Split by endpoint: backups and ledger operations need focused safety tests |
| Payments | 4 | 3 | 1 | 0 | Keep payment callbacks/fulfillment atomic with their tests |
| Shared UX, async, i18n | 11 | 11 | 0 | 0 | Batch common controls and race fixes; keep locale changes with consumers |
| Other | 9 | 8 | 1 | 0 | Inspect after the named risk groups |
| **Total** | **203** | **185** | **11** | **7** | |

## Implemented in the current June worktree

- `b18e4ce45` — scheduler OAuth rate fallback: June admin settings accept blank/null as per-account fallback, preserve explicit zero, and explain it in EN/ZH.
- `bdb9a91db` — video pricing: June model plaza distinguishes per-second video pricing and shows the base rate alongside tiers.
- `57b7dbdc8` + `c19204289` — affiliate offline withdrawal: user/quota selection, 8-decimal validation, ledger action labeling, and persisted idempotency keys for uncertain retries; nullable historical rebate/order rows render safely.
- `a9ff66338` + `8f8358cc6` — monthly backup archives: selected days/month-end, finite or forever retention, explicit archived-copy delete confirmation, and zero retention values survive load/save.
- `b8d52fad3` — per-reasoning-effort billing multipliers adapted across channel pricing, account-stat overrides, group pricing, model-plaza disclosure, and the shared June pricing editor. The map round-trips through the existing backend contract; empty values clear, while unknown keys and non-positive/non-finite values block submission.
- `63c079d87` — OAuth reauthorization preservation is already equivalent in the candidate: existing non-auth credentials are merged before stored credentials are sanitized, with `model_mapping` and `account_id` preserved while auth material is refreshed. Confirmed by `TestApplyOAuthCredentialsPreservesExistingNonAuthCredentials`; no duplicate patch was needed.
- `7f18f3e9a` — adapted the user API-key modal's stale-request protection to June. It reloads when the selected user changes, clears previous rows immediately, and only lets the latest request update rows/loading/error state. Four regression cases cover failed switch, late response, stale `finally`, and switching while open.
- `f3a2dcabb` and `b86849e44` — balance-history and user-error-detail stale-request guards were already present in June, with request-version checks around result/error/loading updates and focused race suites; no duplicate port was needed.
- `50f79e11f` — group replacement already surfaces extracted API failure text and keeps the dialog open for retry; June behavior matches upstream.
- `3b0bb60ea` and `cd2a4357c` — group RPM inputs already reject fractional/negative values while preserving zero RPM, and group rate multipliers already reject non-finite/non-positive values; June has focused validation tests for both.
- `cb2bb6084` — adapted the allowed-groups editor's load-readiness guard. Save remains disabled until group configuration loads successfully and the handler repeats the guard; tests cover pending load, load failure, failed reopen, and successful payload preservation.
- `6b09c74e3` and `26c09b7de` — numeric user attributes already stay strings through the June values API, and optional attribute descriptions/placeholders already send explicit empty strings so a cleared value persists. Both June forms have focused regression tests.

Integration verification at the 2026-09-23 checkpoint before the API-key-modal
slice: the full June suite passed (2,227 tests across 314 files), the production
frontend build passed, and the billing slice passed focused tests, June
typecheck, changed-file ESLint, static i18n-key check, and `git diff --check`.
After the API-key-modal slice, the full June suite passed (2,232 tests across
315 files), June typecheck passed, and its focused four-test suite, changed-file
ESLint, static i18n-key check, and `git diff --check` passed. After the grouped
user/group batch, the full June suite passed (2,236 tests across 316 files),
typecheck, production build, 17 focused tests, changed-file ESLint, static
i18n-key checks, and `git diff --check` passed. The build reports existing
dynamic/static import and large-chunk warnings. The full lint command previously
reported 11 errors in unrelated legacy files; no unrelated files were changed
to silence them.

## One-hour execution discipline

1. Keep one candidate branch and one integrated checkout. Do not spawn more
   worktrees for overlapping June surfaces.
2. Finish one behavior batch at a time. Within a batch, classify each source
   row as equivalent/present, hand-adapt, take verbatim, or skip with evidence;
   merge backend/shared-mirror/June UI facts for the same behavior before edits.
3. Prioritize account/OAuth/routing and billing contract changes over cosmetic
   copy. Keep OAuth, sideband, continuation, quota/failover and payment ledger
   changes serial and review them against the actual caller.
4. Add focused regression tests per batch, run typecheck and changed-file lint
   continuously, then full tests/build/review once at integration checkpoint.
5. Do not call all 203 complete because the standard mirror is current: 185 are
   conservative June rebuild decisions and need a tested equivalent or a real
   port. A one-hour block can finish a small number of coherent, tested feature
   groups; it cannot responsibly disposition every rebuild row. Never trade
   that proof for a percentage claim.

No production, VM, Docker, database, credential, push, or deployment operation
is part of this work.
