# GOAL — finish the June conversion

Single entrypoint. Read this first, work top-down, stop when a gate fails.
Build history and design rationale live in `INFERNO-BUILD.md`; this file is the
backlog and the definition of done.

Baseline taken 2026-08-13, immediately after the dashboard chart conversion:

```
conversion: 118/326 files (36.2%), 14674 legacy utilities across 113384 lines
224 test files / 1588 tests green · june-lint clean across 129 files
```

---

## The one number

```bash
cd inferno-frontend && node scripts/conversion-status.mjs
```

`filesRemaining` and `legacyUtilitiesRemaining` must **only ever go down**.
`--json` for machine use. This reports; it does not gate. The gate is below.

---

## GATES — every one must pass before any commit

Run from `inferno-frontend/`. A red gate means stop and fix, never commit past it.

| # | command | pass condition |
|---|---------|----------------|
| 1 | `npm run typecheck` | zero output after the banner |
| 2 | `node scripts/june-lint.mjs` | `clean across N files` |
| 3 | `npm run test:run` | all green, **and the test count never drops** |
| 4 | `npm run build` | `built in …`, no new errors |
| 5 | `cd .. && git diff --name-only upstream/main..HEAD -- backend frontend deploy docs` | **empty** — thin-fork discipline, backend is read-only |
| 6 | `node scripts/conversion-status.mjs` | remaining count strictly lower than the line above |

Gate 5 is the one that ends the project if broken. Check it every time.

### Known-good exceptions, do not "fix" these
- `npm run lint:check` reports one pre-existing eslint error in
  `scripts/june-lint.mjs` (`no-misleading-character-class`, the emoji range).
  It reproduces on a clean tree. Leave it or fix it deliberately, but it is not
  a regression you caused.
- `/admin/audit-logs` has one nested scrollbar: `DataTable`'s `.table-wrapper`
  inside `TablePageLayout`, which scrolls internally **by design**.

---

## Verification that is not a gate, but is required

Type-checking proves nothing about whether a screen looks right. For any visual
change:

1. **Render it.** Dev server on `:5173`, log in, navigate to the actual route.
2. **Measure, do not eyeball.** `getBoundingClientRect`, `getComputedStyle`,
   `scrollHeight` vs `clientHeight`. Numbers in the commit message.
3. **Both themes.** `localStorage.setItem('theme','dark')` then reload.
4. **Two widths minimum**, one of them below 1024 where the rail goes off-canvas.
5. **Seed first if the screen is empty.** A component cannot be verified against
   no data — see "Seeds" below.

### The shell audit — rerun after any layout change
Walks every in-shell route asserting: document must not scroll · card frame must
not drift while scrolled · outer gaps 7/7/7 · no reserved scrollbar gutter · no
nested scrollbar · no new console errors. Last run: **48/49 clean**, the one
being the by-design table-wrapper above. The script is in the session log for
`384cd7d0`; rebuild it from that commit message if needed.

---

## BACKLOG — ordered. Do them in this order.

### 1. `TablePageLayout` height coupling — SMALL, DO FIRST
`src/components/layout/TablePageLayout.vue` hardcodes
`height: calc(100vh - 46px / 62px / 78px)` at three breakpoints. Its own comment
explains why: *"shell__card is a plain block … so this element cannot inherit a
height through it"*. **That stopped being true in `31a75e07`** — the shell is a
flex chain pinned to `100dvh` now.

- Replace the three `calc()` rules with `height: 100%` (or `flex: 1; min-height: 0`).
- **Done when:** the three `calc(100vh` occurrences are gone AND all 15 table
  views still render with `.tpl` height == the card's client height, measured.
- **Risk:** touches every admin table page. Measure `.tpl` on at least
  `/admin/users`, `/admin/accounts`, `/admin/audit-logs` before and after.

### 2. Finish ops — the modal-only surfaces — ✅ COMPLETE — every mounted file under `src/views/admin/ops/` is converted.

| file | legacy utils | commit |
|------|--------------|--------|
| `OpsSettingsDialog.vue` | 93 | `4e716ec0` |
| `OpsErrorDetailModal.vue` | 95 | `2bff989f` |
| `OpsRequestDetailsModal.vue` | 87 | `755828bc` |
| `OpsAlertRulesCard.vue` | 86 | `6fcea767` |
| `OpsErrorLogTable.vue` | 66 | `263ff0b5` |
| `OpsOpenAITokenStatsCard.vue` | 50 | `11d9fc7f` |
| `OpsErrorDetailsModal.vue` | 21 | `11d9fc7f` |
| `OpsDashboard.vue` | 6 | `11d9fc7f` |

`conversion-status.mjs` now reports only the two dead cards under
`src/views/admin/ops/`. Every one of these was opened in a browser against
seeded data and measured, not just typechecked.

**To see the OpenAI token stats card at all:** it is hidden by default. Ops
page → Settings → Advanced settings → "Display OpenAI token request stats".
The flag lives in `getAdvancedSettings`, not in the `settings` table, so there
is no row to flip directly.

#### ⚠️ OWNER DECISION NEEDED — two ops cards are dead code

`OpsRuntimeSettingsCard.vue` (118 utils, 537L) and
`OpsEmailNotificationCard.vue` (121 utils, 442L) have **zero references
anywhere in `src/`** — no import, no `defineAsyncComponent`, no `<component
:is>`. They are also unmounted in `upstream/main`; they arrived dead in
`d464c0f0` when the upstream frontend was vendored wholesale.

They are superseded by `OpsSettingsDialog.vue`, which calls the same two API
pairs (`get/updateAlertRuntimeSettings`, `get/updateEmailNotificationConfig`)
and renders the same fields.

**Not converted deliberately.** 239 utilities of markup that nothing renders
would move the metric without moving the product, and would then read as
"converted" forever. **Not deleted either** — reducing scope is the owner's
call (rule 3), and there is one live consequence to weigh first:

> `OpsRuntimeSettingsCard` was the only UI in the product for **alert
> silencing** (`silencing.global_until_rfc3339`, per-rule silence entries) and
> for the **distributed lock** settings. `OpsSettingsDialog` does not render
> either — `grep -c 'silencing\|distributed_lock' OpsSettingsDialog.vue` is 0.
> Those fields are not lost on save (the dialog PUTs back the whole runtime
> object it loaded, so they round-trip untouched), but they are **only
> editable via the API**, not the dashboard.

Three ways forward, owner picks:
1. **Delete both** — accept that silencing stays API-only. One command:
   `git rm src/views/admin/ops/components/OpsRuntimeSettingsCard.vue src/views/admin/ops/components/OpsEmailNotificationCard.vue`
2. **Port silencing into `OpsSettingsDialog`** as a new section, then delete
   both. Silencing becomes reachable for the first time.
3. **Mount `OpsRuntimeSettingsCard`** somewhere and convert it — most work,
   and re-introduces two editors for the same settings.

Recommendation: **2**, then 1. Silencing an alert during a known incident is a
real operator need, and it is currently a curl away rather than a click.

### 3. `SettingsView` — the elephant, 12,923 lines / 1,742 utilities
### ⚠️ DEFERRED — needs an owner decision before it can start. Item 4 went first.

**Correction:** this file previously said "seven routes". `INFERNO-BUILD.md`
line 1434 says **nine**. That line is the source of truth.

**Why it is blocked and not merely large.** The same line states: *"part 14
wants a redirect from the old URL and a release note, since admin bookmarks
break."* Splitting `/admin/settings` into nine routes is a user-facing
navigation change that invalidates every bookmark and deep link an operator
has. That is a product decision, not a styling one, and it is not something to
land silently overnight.

Owner picks one:
1. **Nine real routes** (`/admin/settings/general`, `/security`, …) with a
   redirect from `/admin/settings` to the first tab, plus a release note.
   Matches archetype C. Bookmarks to the bare URL survive via the redirect;
   nothing else does, because nothing else exists today.
2. **Nine components, one route**, selected by a tab or a `?tab=` query. Zero
   URL breakage, the 12,923-line file still becomes nine reviewable files, and
   the review problem is solved. Loses the clean per-section URL.
3. **Restyle in place** without splitting. Cheapest, but a 12,923-line SFC
   cannot be meaningfully reviewed, which is the reason the split was proposed.

Recommendation: **2**. It captures the entire reviewability win, which is the
actual blocker, at zero cost to anyone's bookmarks, and 1 stays available later
as a pure routing change once the components already exist.

Once unblocked, either way:
- Split **before** restyling. One section per commit, all six gates each.
- It is on the june-lint `TOUCHED_NOT_CONVERTED` waiver — **remove that entry**
  when the last section lands, and confirm lint still passes with it gone.

### 4. Remaining Chart.js — 4 consumers left
`grep -rln "chart\.js" src/` should reach zero, then drop the dependency.

- `src/components/admin/payment/DailyRevenueChart.vue` — needs `payment_enabled`
  and order data to verify; seed before starting.
- `src/features/channel-monitor-v2/MonitorTrendChart.vue`
- `src/components/account/AccountStatsModal.vue` **and**
  `src/components/admin/account/AccountStatsModal.vue` — ⚠️ these are two copies
  of the same component (749L and 713L, different hashes, both import Chart.js).
  **Reconcile them into one before converting**, or the work is done twice into
  two files that keep diverging.
- **Done when:** `grep -rln "chart\.js" src/` is empty and `chart.js` +
  `vue-chartjs` are out of `package.json`.

### 5. The rest, by weight
`node scripts/conversion-status.mjs --top 30`. Largest first, except that the
three account modals (`CreateAccountModal` 965, `EditAccountModal` 620,
`BulkEditAccountModal` 293) are one job, not three — they share structure and
are all on the waiver list.

---

## Seeds — a screen with no data cannot be verified

All seeds are `now()`-relative, so re-running re-anchors them. The DB clock
drifts ahead of the seed; if a page reads Idle or "No data", reseed before
concluding anything is broken.

```bash
cd inferno-frontend/scripts/seed
for f in seed-dashboard seed-year seed-models seed-ops seed-ops-dense seed-alert-events seed-users; do
  docker cp $f.sql sub2api-postgres:/tmp/
  docker exec sub2api-postgres psql -U sub2api -d sub2api -q -f /tmp/$f.sql
done
```

See `inferno-frontend/scripts/seed/README.md` for what each one covers.

Seeded so far: a year of dashboard daily/hourly, per-model usage, dense ops
traffic + error logs + hourly metrics, 24 alert events across the full P0–P3
ramp and all three statuses, and 11 users with 30 days of usage.

Local dev login: `admin@sub2api.local` / `36232e0cd5be929e4004e4ca025b100e`.

---

## Rules that are not negotiable

1. **Gate 5.** The backend is read-only. If `upstream/main..HEAD` shows anything
   under `backend/`, `frontend/`, `deploy/` or `docs/`, revert it.
2. **Never weaken a test to make it green.** If a spec fails, decide whether the
   test or the code is wrong and fix that one. `INFERNO-BUILD.md` has a worked
   example of this going right.
3. **Never delete a row, column or control to make a layout tidy.** Reducing
   scope is the owner's call. If something must go, say so explicitly.
4. **Measure before claiming.** "Looks fine" is not evidence. Every completion
   claim in the log so far carries numbers; keep that.
5. **A BEM block may not be named after a bare Tailwind utility** while Tailwind
   is still in the build. `june-lint` enforces this now — it was found the hard
   way when `.ring` inherited Tailwind's blue focus ring for three commits.

---

## Status log — append one line per landed commit

| date | commit | what | files remaining |
|------|--------|------|-----------------|
| 2026-08-13 | `0ae68c16` | ops skeleton matched to the real page | 119 |
| 2026-08-13 | `31a75e07` | shell: card scrolls, not the document | 119 |
| 2026-08-13 | `384cd7d0` | card scrollbar hidden, padding squared | 119 |
| 2026-08-13 | `35aab418` | dashboard user-trend chart off Chart.js | 118 |
| 2026-08-13 | `caeb742f` | TablePageLayout derives its height (item 1) | 118 |
| 2026-08-13 | `4e716ec0` | ops settings dialog + NumberField primitive | 120 |
| 2026-08-13 | `2bff989f` | ops error detail modal; 12 cards to a `<dl>` | 121 |
| 2026-08-13 | `755828bc` | ops request details; colour off the default state | 122 |
| 2026-08-13 | `6fcea767` | ops alert rules; list stops showing enum names | 123 |
| 2026-08-13 | `263ff0b5` | ops error log table; 6 hues for a category to 0 | 124 |
| 2026-08-13 | `11d9fc7f` | last 3 ops surfaces — **item 2 complete** | 127 |
