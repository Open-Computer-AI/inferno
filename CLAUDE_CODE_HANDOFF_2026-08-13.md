# Claude Code → Codex handoff

## Source session

- Claude title: **Rename OC router to Inferno with upstream sync**
- Claude session ID: `3555e339-7e70-40ff-8a71-1c207a51ba41`
- Transcript: `~/.claude/projects/-Users-saksham/3555e339-7e70-40ff-8a71-1c207a51ba41.jsonl`
- Latest Claude working directory: `/Users/saksham/OpenComputerV2/inferno/inferno-frontend`
- Session ended on 2026-08-13 after the weekly Claude usage limit was reached.

This document is a curated handoff, not a raw transcript. It intentionally excludes
credentials, pasted secrets, tool payloads, and repetitive UI noise.

## Executive summary

The work is an incremental visual/design-system conversion of the vendored
`inferno-frontend` inside the Inferno fork. The backend and upstream-fork boundary
must remain untouched. The conversion replaces legacy Tailwind/Chart.js presentation
with the repository's tokenized shell and Dither chart primitives, while preserving
the actual API data, interactions, empty states, zoom behavior, and responsive layout.

Claude completed and committed the shell, dashboard, ops, and payment milestones.
Codex resumed the session and committed the final Chart.js cleanup as `6db5fef1`.
The current working tree may contain only intentional follow-up documentation
changes; do not reset or discard unrelated user work.

## Repository coordinates

```text
Repository: /Users/saksham/OpenComputerV2/inferno
Branch:     inferno-redesign
Remote:     origin/inferno-redesign
HEAD:       b594a5dd feat(payment): revenue chart off Chart.js; reconcile the duplicate stats modal
Upstream:   upstream/main
Fork state: branch is 48 commits ahead of origin/inferno-redesign
Frontend:   inferno-frontend/
```

The parent repository instructions are in `/Users/saksham/OpenComputerV2/AGENTS.md`.
The key constraints are: keep the backend/frontend fork thin, use focused commits,
run all gates, measure visual changes, and never weaken tests merely to make them
green.

## What is complete and committed

The recent commit sequence is the authoritative implementation history:

- `caeb742f` — `TablePageLayout` now derives its height from the flex shell.
- `35aab418` — dashboard user-trend Chart.js conversion, conversion counters, seed data,
  and the June-conversion goal file.
- `31a75e07`, `384cd7d0`, and related shell commits — card scrolling, padding,
  scrollbar behavior, and shell layout fixes.
- `4e716ec0` through `11d9fc7f` — ops settings, error/request modals, alert rules,
  error logs, OpenAI token stats, and the ops dashboard converted to the tokenized
  design system.
- `c785b917` — `GOAL.md` records ops as complete and defers `SettingsView` pending
  an owner decision about its URL/navigation split.
- `b594a5dd` — payment revenue chart moved off Chart.js, the dead duplicate
  `components/account/AccountStatsModal.vue` was removed, payment seed data was added,
  and payment labels were corrected.

Important implementation decisions already made:

- Different units do not share a stack or dual axis. Revenue currencies and order
  count use independent Dither strips; QPS/TPS and TTFT follow the same principle.
- Gaps remain gaps. Do not interpolate a day with no orders or turn an unknown TTFT
  value into zero.
- Payment seed rows must use the backend's uppercase statuses and put currency in
  `provider_snapshot.currency`; currency is not a `payment_orders` column.
- The OpenAI token stats card is hidden by default. Enable it through Ops → Settings
  → Advanced settings → “Display OpenAI token request stats” before visual testing.
- The existing `inferno-frontend` is the frontend. Do not create a second replacement
  frontend or modify backend/upstream-fork files for this conversion.

## Resumed work committed by Codex

The identified Claude session left the following Chart.js conversion changes
uncommitted. Codex reviewed and committed them as `6db5fef1`:

```text
inferno-frontend/package.json
inferno-frontend/pnpm-lock.yaml
inferno-frontend/scripts/june-lint.mjs
inferno-frontend/src/components/admin/account/AccountStatsModal.vue
inferno-frontend/src/components/charts/__tests__/TokenUsageTrend.spec.ts
inferno-frontend/src/features/channel-monitor-v2/MonitorTrendChart.vue
inferno-frontend/src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts
inferno-frontend/src/views/admin/ops/components/__tests__/OpsErrorScopeCharts.spec.ts
```

### Chart.js removal

- `MonitorTrendChart.vue` now uses `DitherArea` while retaining data-array zooming,
  the wheel interaction, null/gap behavior, and the existing three-series semantics:
  error rate, cache rate, and TTFT.
- The live admin `AccountStatsModal.vue` usage trend now uses Dither strips. The
  rest of that large modal remains legacy and is explicitly listed in the
  `TOUCHED_NOT_CONVERTED` waiver; remove the waiver only when the modal's full
  conversion is actually done.
- `chart.js` and `vue-chartjs` were removed from `package.json` and `pnpm-lock.yaml`.
- Vestigial chart mocks were removed from the affected specs.
- The design-system structure test was updated from assertions on the old Tailwind
  class strings to assertions on the current tokenized shell and Dither primitive.

No runtime imports of `chart.js` or `vue-chartjs` remain. Remaining textual matches
are comments or tests describing the migration.

## Verification performed after Claude stopped

These commands were run against the current working tree:

| Check | Result |
|---|---|
| `npm run typecheck` | Pass, exit 0 |
| `node scripts/june-lint.mjs` | Pass: clean across 142 converted files |
| `npm run test:run` | Pass: 224 test files, 1,588 tests |
| `npm run build` | Pass: built in 22.09s; existing large-chunk warnings only |
| thin-fork gate | Pass: no `backend`, `frontend`, `deploy`, or `docs` diff against `upstream/main` |
| conversion status | 129/326 files, 39.6%; 13,967 legacy utilities remaining |

The test suite emits expected stderr from tests that deliberately exercise error
paths, plus existing Vue/i18n warnings. They did not fail the run.

The final Chart.js changes are verified by the gates and focused code review. The
local `/monitor` route currently has no configured channels, so a real-data browser
render of `MonitorTrendChart` remains outstanding; do not claim that visual proof
until channel data is seeded or a configured monitor is available.

## Remaining backlog and owner decisions

### 1. Dead ops cards

`OpsRuntimeSettingsCard.vue` and `OpsEmailNotificationCard.vue` have zero references
under `src/` and are also unmounted upstream. They are not converted or deleted.

The runtime settings card contains the only UI for alert silencing and distributed
lock settings. The live `OpsSettingsDialog` round-trips those fields but does not
make them editable. The recommended path is:

1. Port silencing controls into `OpsSettingsDialog`.
2. Delete the two dead cards.
3. Run all gates and record the decision in `GOAL.md`.

Do not delete them silently; this is an owner/product decision.

### 2. `SettingsView.vue`

`src/views/admin/SettingsView.vue` is 12,923 lines / 1,742 legacy utilities.
`INFERNO-BUILD.md` says the intended split is **nine** sections and warns that
real routes require redirects and a release note because bookmarks break.

Recommended owner decision: split into nine components behind the existing single
route, optionally using a tab/query selector. This preserves bookmarks while making
the work reviewable. Do not start the split until the routing choice is explicit.

### 3. Remaining conversion work

After the two decisions above, continue with:

```bash
cd /Users/saksham/OpenComputerV2/inferno/inferno-frontend
node scripts/conversion-status.mjs --top 30
```

The largest remaining items include `SettingsView.vue`, `GroupsView.vue`, the three
account modals (`CreateAccountModal`, `EditAccountModal`, `BulkEditAccountModal`),
`RiskControlView.vue`, and other files listed by the status command.

## Exact next-session procedure

1. Read `/Users/saksham/OpenComputerV2/AGENTS.md` and this handoff.
2. Confirm `6db5fef1` and the `GOAL.md` status update are present.
3. If visual proof is required, seed/configure monitor data and render the monitor
   chart at both themes and at least two widths.
4. Before any further work, rerun all six gates from `GOAL.md`.
5. Resolve the dead-ops-card and `SettingsView` owner decisions before starting
   either backlog item.

Useful commands:

```bash
cd /Users/saksham/OpenComputerV2/inferno
git diff --check
git diff --stat
git status --short

cd inferno-frontend
npm run typecheck
node scripts/june-lint.mjs
npm run test:run
npm run build

cd ..
git diff --name-only upstream/main..HEAD -- backend frontend deploy docs
```

## Safety notes

- Do not use `git reset --hard`, `git checkout --`, or broad cleanup commands.
- Do not commit or copy credentials from the Claude transcript. The raw transcript
  contains pasted secrets from earlier prompts; this handoff does not.
- Do not touch backend/upstream-fork files; gate 5 must stay empty.
- Do not remove UI rows, controls, or data fields merely to make a layout easier.
- The Chart.js conversion is committed in `6db5fef1`; preserve that commit and the
  documented visual-verification caveat.
