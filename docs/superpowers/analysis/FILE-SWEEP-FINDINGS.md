# File-level sweep — what the June rewrite broke

Date: 2026-08-30. Complement to `COMMIT-MANIFEST.md`, which asks a different
question and is still correct.

| Question | Instrument | Verdict |
|---|---|---|
| did every upstream commit land? | `COMMIT-MANIFEST.md` | yes — 103 rows, 0 open |
| does every file BEHAVE like upstream? | this sweep | no — 20 defects |

**Nothing here is a bad port.** Every defect was introduced by the June restyle
of a file that already worked. That is why the commit-level audit could not have
found any of it, and why reporting "Set B complete" was true but not sufficient.

## The surface

```
identical to upstream : 467   nothing to check
differ                : 285   the sweep target, 60452 diff lines
only in upstream      :   9   checked by hand; relocated or unreferenced
only in ours          :  89   no upstream twin
```

Of the 285, **243 have script-region changes** — so "June only touched template
and style" is false, and the risk is not confined to markup.

## Method

Six read-only agents read all 285 files against upstream and classified each
STYLE / OURS-AHEAD / SUSPECT. Every SUSPECT was then re-checked by hand before
any edit. Two agent claims were rejected on measurement:

- "546 broken `font-[var(--fw-medium)]` across 47 files" — ran Tailwind on a
  fixture; it emits `font-weight: var(--fw-medium)` correctly. Not a bug.
- "`ProxiesView` renders `badge-danger : badge-danger`" — identical upstream.
  Not ours to change.

The same probe DID confirm `bg-[var(--brand-tint)/60]` emits
`background-color: var(--brand-tint)/60`, which browsers drop. 135 of those.

## Defects fixed

**Reachability**
- `AppSidebar` — `/monitor`, `/purchase`, `/orders` had no entry point anywhere.
  The reduction planned to fold them elsewhere; no fold was built.
- `AppSidebar` — `/admin/prompt-audit` ungated while its route carries
  `requiresRiskControl`, so the row bounced the admin to `/admin/settings`.
- `GroupSelector` — rows advertised a full-width click target that did nothing.

**Predicate drift**
- `UserConcurrencyCell`, `GroupCapacityBadge` — `used >= max` became `pct > 100`,
  making the at-capacity tone unreachable under an enforced cap.
- `OpsErrorTrendChart` — 429/529 dropped from `totalUpstreamErrors`, disabling
  the drill-down during a pure rate-limit outage.
- `OpsResourceMeters` — DB pool saturation counted active connections only; idle
  connections hold slots, so a checked-out pool read as near-empty.
- `RiskControlView` — tone dropped `mode === 'off'`; "Disabled" beside a green tick.
- `Toggle` — emitted `event.target.checked`, making the DOM the source of truth;
  a parent that refused the change left the control dead.
- `BaseDialog` — Escape gated on focus containment, so a scrim click killed it.
  Now gated on being topmost, which also fixes closing a whole stack at once.
- `opsFormatters` — empty timestamps rendered blank instead of `-`.

**Wrong or missing strings**
- 22 i18n keys referenced and never declared; 4 pointed at the wrong path.
- `AmountInput` — hardcoded English and a hardcoded `$`; default currency is CNY
  and the Razorpay flow is INR.
- `UserDashboardCharts` — the rolled-up tail was labelled "No Group", which in
  this product means an unassigned key.
- `AuthLayout` — operator `site_subtitle` stopped rendering on auth pages. The
  first fix was partial (slot fallback) and still hid it on the four views that
  set a slot; the tagline now renders as its own line.

**Collapsed state colours**
- Export progress bar, and two for-sale toggles, whose on/off branches had become
  the same token.

**Robustness**
- `UserDashboardStats` — upstream's per-field `|| 0` guards were dropped; one
  absent numeric field threw inside a template and blanked the dashboard.

## Gate failures this exposed

`scripts/i18n-keycheck.mjs` reported **"all t() keys resolve" while scanning zero
files** — it only checked paths passed as arguments, and matched only the leaf of
a dotted path, so `dates.from` passed because an unrelated block declared a
`from:`. Proven by breaking a key deliberately: the old script passed it.

Replaced by `src/i18n/__tests__/keyResolution.spec.ts` — resolves full paths
against the real locale object, discovers its own inputs, and fails that same
mutation.

## The standing detector

`inferno-frontend/scripts/logic-drift.mjs` lists every line carrying a condition
that exists upstream and has no counterpart in ours:

```
774 vanished predicate line(s) across 85 file(s)
node scripts/logic-drift.mjs <path>    # the lines for one file
```

It is a **worklist, not an oracle** — a line legitimately rewritten shows up too.
It independently ranks the same files the agents found by reading, and it bounds
the surface from 60k diff lines to a few hundred. Run it before claiming a June
conversion is faithful.

## Not yet closed

Ranked by the detector, still to review: `OpsDashboardHeader` (89 lines — SLA and
error-rate thresholds are configured but never read for display; several metrics
no longer rendered), `AccountCapacityCell` (33), `DashboardView` (15 — `tpm`,
`today_account_cost`, `total_account_cost`, `total_cost` no longer rendered),
`UsageView` (`endpointDistributionSource` pinned to `inbound`, upstream/path
endpoint stats unreachable), `ProxySelector` (rows drop `proxy.name` and
`account_count` while name is still the search key), `AccountUsageCell`
(`GrokQuotaProbeCell` has no call site, so an xAI quota probe cannot be
triggered), `DataTable` (`isScrollable` machinery has no consumer),
`OpsErrorDistributionChart` (colour assigned by post-filter index, so a slice
changes colour between refreshes).

Deliberate reductions needing sign-off, not defects: `ModelDistributionChart` and
`EndpointDistributionChart` `TOP_N=6` truncation removes drill-down past rank 6.

Coverage regressions (behaviour still live, its only test deleted):
`AccountUsageCell.spec` (Grok tier resolution), `EditAccountModal.spec`
(auto-reset visibility gating), `OllamaCloudUsageCell.spec`, `UsageView.spec`
(request-id column migration), `GroupsView.columnSettings.spec`,
`PlatformTypeBadge.grok.spec`, `useModelWhitelist.spec`.
