#!/usr/bin/env node
/**
 * debt-ledger — what the June rewrite still owes, with status MEASURED from the
 * code rather than asserted by a checkbox.
 *
 * WHY THIS EXISTS: the June conversion ran as seven batch commits in one day
 * (2026-08-09), and each deliberately deferred cross-file work, writing the debt
 * into its own commit message and into INFERNO-BUILD.md. Part 08 said it
 * outright: "Three things are owed and cannot be built from inside these files",
 * and "19 i18n keys pending; both cells render raw paths until they land."
 *
 * That checklist ended at 17 unchecked boxes and 0 checked. When the boxes were
 * finally probed, three were simply WRONG: buildGrokUsageRefreshKey had been
 * wired weeks earlier, and /legal/terms does not 404 (it renders, empty, until
 * an operator fills the content in Settings). A box records what someone
 * believed once. The upstream ports never rotted this way, because their ledger
 * required a row and its port in the SAME commit and every row could be checked
 * against the tree.
 *
 * So every row here carries a `probe` that prints evidence the item is still
 * open. Silence means closed. A row cannot be closed by editing this file --
 * only by changing the code until its probe goes quiet. Same principle that
 * killed the old i18n-keycheck: status you assert is worthless, status you
 * measure is not.
 *
 *   node scripts/debt-ledger.mjs           status of every row
 *   node scripts/debt-ledger.mjs --open    only what is still open
 *   node scripts/debt-ledger.mjs --check   exit 1 if a CLOSED row has reopened
 */
import { execSync } from 'node:child_process'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const SRC = resolve(ROOT, 'src')

// probe : prints evidence the item is STILL OPEN. Silence means closed.
// expect: what we believe today. --check fails only when a 'closed' row reopens,
//         so this gates against regression without blocking on known debt.
// DELIBERATE, NOT DEBT -- do not re-add as a row:
//
// The user usage page has no endpoint-source picker, and must not get one. The
// backend nils UpstreamEndpoints and EndpointPaths on the USER stats endpoint
// (usage_handler.go Stats) -- upstream routing detail is admin-scope by design
// -- so a three-way selector there would offer two options that can never hold
// data. The ADMIN usage page already has the real one, on
// EndpointDistributionChart with show-source-toggle, where the data exists.
//
// This row was wrong when I wrote it: it claimed the stats were "fetched and
// unreachable" on the user view. They are not fetched at all. I read the
// consumer and never checked the producer -- the same one-sided mistake as
// dashboard-stat-fields. Verified by building the selector, seeing two
// permanently empty options in the browser, and reverting it.
//
// AccountUsageCell.pinnedWindowKey has no producer, and that is fine. Upstream
// has no such prop and no column-header window picker at all -- it was a June
// idea from part 08, and only the header UI was left unbuilt. The cell side is
// implemented and correct (primaryWindow honours a pin, falls back to
// closest-to-limit), so nothing is lost and nothing regressed against upstream.
// Building the picker now would be inventing scope nobody asked for. The hook
// stays so the work is already done if it is ever wanted.
//
// admin DashboardView does not show tpm, total_cost, today_account_cost or
// total_account_cost. The file sweep flagged all four; Saksham confirmed the
// omission is a product decision about what the dashboard puts in front of an
// operator. Two of them could not have been rendered anyway -- the admin stats
// endpoint returns 33 keys and neither account-cost field is among them, so
// upstream's own markup paints a coerced $0 for money it never measured.
//
// Kept here rather than deleted because a future file-level sweep WILL flag
// these again: they look exactly like dropped upstream logic.
const LEDGER = [
  {
    id: 'behaviour-parity',
    what: 'Upstream specs with no equivalent in ours, or holding fewer cases — behaviour they pin that we do not',
    origin: 'scripts/behaviour-parity.mjs 2026-08-31; 4 missing files + 3 shortfalls remain, each needing a reason or a test',
    expect: 'closed',
    probe: `cd ${ROOT} && n=$(node scripts/behaviour-parity.mjs --gaps 2>/dev/null | grep -cE '^  (-|[0-9a-f]{7})'); [ "$n" != "0" ] && echo "$n upstream-pinned behaviours unmatched in ours"`
  },
  {
    id: 'port-coverage-clean',
    what: 'No ported upstream commit has silently failed to land — coverage explained for every manifest row',
    origin: 'five ports partially applied and were recorded done; scripts/port-coverage.mjs 2026-08-31',
    expect: 'closed',
    /*
     * Pins the SET of low-coverage commits, not the COUNT. A count passes if
     * one explained row rises above 40% while an unexplained one drops below
     * it -- which is exactly the silent partial-port this row exists to catch.
     *
     * Every sha below scores low for a reason that has been checked:
     *
     *   b689e5b40  response-model billing hardening — our June billing cells
     *              were rebuilt; upstream's literal lines are correctly absent
     *   a6d868f27  cache tokens in the token card — same, UsageStatsCards
     *   0d5e3ca9b  neutral SLA card — ops header rebuilt in June tokens
     *   e8ff2017c  ops error distribution legend — chart rebuilt off Chart.js
     *   6c3edc095  429 cooldown strategies — settings UI extracted into
     *              AdminGatewaySettingsPage, so none of its markup matches
     *   0aef702b6  hidden cache tooltip — REBUILD. Our HelpTooltip already
     *              fixed the bug via v-show + Teleport, so the 4 unmatched
     *              lines are upstream's class assertions on a span we do not
     *              have. Added 2026-09-02 with the catch-up's Set C.
     */
    /*
     * Pins the recorded SET of unaccounted lines, not a count of anything.
     *
     * Both earlier shapes were measurably too weak. A count of rows below 40%
     * passed when one explained row rose and an unexplained one fell. A count
     * of FILES with gaps passed when a new defect landed inside an
     * already-explained file -- measured: flipping SettingsView's TTFT default
     * took that file from 19% to 13% and the headline never moved.
     *
     * port-coverage --check-baseline diffs the current unaccounted set against
     * docs/superpowers/analysis/port-coverage-missing.txt and exits 1 naming
     * every line that changed state. Verified against four injected defects:
     * a reverted hunk, a changed locale value, a flipped default and an
     * inverted predicate. All four caught; the flipped default had previously
     * escaped tsc, 2049 tests, june-lint, behaviour-parity and old coverage.
     *
     * Re-baseline only after reviewing the delta -- a June conversion
     * legitimately removes upstream lines and will move this.
     */
    probe: `cd ${ROOT} && node scripts/port-coverage.mjs --check-baseline >/dev/null 2>&1 || echo "the unaccounted-line set changed — run: node scripts/port-coverage.mjs --check-baseline"`
  },
  {
    id: 'grok-probe-column',
    what: 'Opt-in Grok probe column — the probe has no call site, so an xAI quota probe cannot be triggered anywhere in the app',
    origin: 'part 08 departures (3737762c9) unhooked the embedded button and owed a replacement column',
    expect: 'open',
    probe: `grep -rl "import.*GrokQuotaProbeCell" ${SRC} | grep -v "__tests__" | head -1 | grep -q . || echo "GrokQuotaProbeCell has no importer"`
  },
  {
    id: 'ops-thresholds-unread',
    what: 'Ops SLA and request-error-rate thresholds are configurable and saved, but nothing reads them for display',
    origin: 'file sweep 2026-08-30 — OpsDashboardHeader lost getSLAThresholdLevel and getRequestErrorRateThresholdLevel',
    expect: 'open',
    probe: `grep -rl "sla_percent_min" ${SRC} | grep -v "api/\\|OpsSettingsDialog\\|__tests__" | head -1 | grep -q . || echo "sla_percent_min is written but never read for display"`
  },
  {
    id: 'proxy-selector-name',
    what: 'ProxySelector rows omit proxy.name and account_count, while name is still what the search filters on',
    origin: 'file sweep 2026-08-30',
    expect: 'open',
    probe: `test "$(grep -c "proxy\\.name" ${SRC}/components/common/ProxySelector.vue)" = "1" && echo "proxy.name appears only in the search predicate, never rendered"`
  },
  {
    id: 'june-checkbox',
    what: 'Every checkbox is the design system Checkbox, not the browser native control',
    origin: 'INFERNO-BUILD.md owed cross-file work -- 23 files, 111 instances, converted 2026-08-30',
    expect: 'closed',
    probe: `n=$(grep -rl 'type="checkbox"' ${SRC} | grep -v "__tests__\\|Checkbox.vue\\|Toggle.vue" | wc -l | tr -d " "); [ "$n" != "0" ] && echo "$n files still use a raw checkbox"`
  },
  {
    id: 'grok-refresh-key',
    what: 'AccountsView wires buildGrokUsageRefreshKey so Grok cells refresh reactively like OpenAI does',
    origin: 'INFERNO-BUILD.md listed this as owed — the box was STALE, it was wired weeks earlier',
    expect: 'closed',
    probe: `grep -q "buildGrokUsageRefreshKey(current)" ${SRC}/views/admin/AccountsView.vue || echo "not wired in AccountsView"`
  },
  {
    id: 'i18n-keys-resolve',
    what: 'Every static t() key resolves — part 08 shipped cells that "render raw paths until they land"',
    origin: 'part 08 departures: "19 i18n keys pending"',
    expect: 'closed',
    probe: `cd ${ROOT} && npx vitest run src/i18n/__tests__/keyResolution.spec.ts --silent >/dev/null 2>&1 || echo "unresolved t() keys remain"`
  },
  {
    id: 'sidebar-reachability',
    what: 'Every route the shell rewrite dropped is reachable again (/monitor, /purchase, /orders)',
    origin: 'part 07 app shell v2 — folds that were planned in the commit message and never built',
    expect: 'closed',
    probe: `for p in /monitor /purchase /orders; do grep -q "path: '$p'" ${SRC}/components/layout/AppSidebar.vue || echo "no nav entry for $p"; done`
  },
  {
    id: 'distribution-tail',
    what: 'The distribution charts keep the tail past TOP_N reachable rather than collapsing it into a dead row',
    origin: 'part 09 charts — TOP_N=6 made every model and endpoint past rank 6 undrillable',
    expect: 'closed',
    probe: `for f in ModelDistributionChart EndpointDistributionChart; do grep -q "othersExpanded" ${SRC}/components/charts/$f.vue || echo "$f has no expandable tail"; done`
  }
]

// Probes run through a shell because they ARE shell -- grep pipelines and small
// loops. That is safe here and must stay safe: every probe is a literal in the
// LEDGER above, and nothing from argv or the environment is ever interpolated
// into one. argv is read only for flags. If you ever need a probe to take a
// value from outside this file, switch it to execFile with an argument array
// rather than building a string.
const run = (cmd) => {
  try {
    return execSync(cmd, { shell: '/bin/sh', encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }).trim()
  } catch (e) {
    return (e.stdout || '').trim()
  }
}

const onlyOpen = process.argv.includes('--open')
const check = process.argv.includes('--check')

let reopened = 0
const rows = LEDGER.map((row) => {
  const evidence = run(row.probe)
  const status = evidence ? 'OPEN' : 'CLOSED'
  if (row.expect === 'closed' && status === 'OPEN') reopened++
  return { ...row, status, evidence }
})

for (const r of rows) {
  if (onlyOpen && r.status === 'CLOSED') continue
  console.log(`${r.status === 'OPEN' ? 'o' : '*'} ${r.status.padEnd(6)} ${r.id}`)
  console.log(`           ${r.what}`)
  if (r.evidence) for (const line of r.evidence.split('\n')) console.log(`           -> ${line}`)
  console.log(`           origin: ${r.origin}`)
  console.log()
}

const open = rows.filter((r) => r.status === 'OPEN').length
console.log(`${open} open, ${rows.length - open} closed, of ${rows.length} tracked.`)

if (check) {
  if (reopened) {
    console.log(`\nFAIL: ${reopened} row(s) expected closed are open again.`)
    process.exit(1)
  }
  console.log('No closed row has reopened.')
}
