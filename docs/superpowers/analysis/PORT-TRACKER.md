# June port tracker — upstream features not yet built in `inferno-frontend`

**The model.** `frontend/` is the pristine upstream mirror and it is the SPEC.
`inferno-frontend/` is our implementation of that spec in the June design system.
Upstream ships a feature; we build our June version of it. This is a continuous
pipeline, not a one-time conversion — the whole app was built this way.

The old rule ("ignore upstream changes under components/views") was never a risk
rule. It quietly **paused this pipeline**, and it has been paused since
2026-08-16. That is why the backend kept arriving and the frontend did not: the
backend needs no adaptation and merges itself, the frontend needs rewriting into
June before it counts as arrived. Only the automatic half had a process.

**Backend status:** synced through `e866ff6ec` (2026-08-28). Verified by feature
probe, not by trusting the merge. Nearly every item below is a UI gap against
endpoints our own backend already serves.

**Rules for every item:** hand-merge, never `cp` (see the 2026-08-11 incident in
INFERNO-BUILD.md). June tokens — except where our copy of a file is still literal
Tailwind, e.g. `utils/platformColors.ts`, where a verbatim merge is correct.
Rewrite en and em dashes out of any copied i18n string; `june-lint` bans both.

Status legend: `TODO` · `WIP` · `DONE` · `BLOCKED`

---

## P1 — visible to users today

### 1. CN providers (Kimi / Zhipu / DeepSeek) — TODO
Backend routes them first-class (`gateway_handler.go`, `openai_gateway_handler.go`,
`routes/gateway.go`). Our UI has no concept of them, so **a group tagged `kimi`
renders the literal string `kimi`** — `t('admin.groups.platforms.' + platform,
platform)` falls back to the raw id.

- [ ] widen `AccountPlatform` / `GroupPlatform` (`types/index.ts:529,886`)
- [ ] new `constants/platforms.ts` — `CONCRETE_PLATFORM_OPTIONS`, `GROUP_PLATFORM_OPTIONS`
- [ ] new `api/admin/cnProviders.ts` — `queryQuota`, `queryBalance`
- [ ] new `components/account/CNProviderQuotaCell.vue`, `CNProviderBalanceCell.vue`, `CnBaseUrlPresets.vue`
- [ ] `utils/platformColors.ts` — add 3 platforms to every map (**verbatim, this file is not June-tokenised**)
- [ ] `common/PlatformTypeBadge.vue` — labels, else they fall through to "Gemini"
- [ ] `composables/useModelWhitelist.ts` — `kimi-for-coding`, `kimi-k2`, `kimi` alias
- [ ] replace hardcoded platform lists: `GroupsView.vue:4612,4621,4631`, `SubscriptionsView.vue:997`, `OpsDashboardHeader.vue:149`, `ChannelsView.vue:766,811`, `ErrorPassthroughRulesModal.vue`, `AccountTableFilters.vue`
- [ ] i18n: `admin.groups.platforms.*` **first** (the visible one), then `accounts.platforms.*`, `accounts.cnProviders.*` (~46 keys)

### 2. `openaiFastPolicy` terminology rename — TODO
8 keys whose values upstream rewrote ("whitelist" to "target models"). Ours hold
the OLD strings and they have **live consumers**, so our settings page renders
stale vocabulary right now.

- [ ] 8 keys atomically in `en` + `zh` `admin/settings.ts` — mixed vocabulary if partial
- [ ] add `summaryTargetModels`, `summaryAllModels`, `summaryOtherModels`, `summaryAction.*`
- [ ] do NOT touch the unrelated `betaPolicy.modelWhitelist` block in the same file
- [ ] port `__tests__/openaiFastPolicyLocales.spec.ts` in the same change

### 3. Empty announcements shows "failed to load" — TODO
`views/admin/AnnouncementsView.vue:151` passes `t('admin.announcements.failedToLoad')`
as the **empty-state** description.

- [ ] add `admin.announcements.createFirstAnnouncement`
- [ ] fix the `.vue` reference

### 4. Composite groups cannot enable Live / Messages dispatch — TODO
Backend permits both for `composite` (`admin_group.go:513,893`,
`openai_messages_dispatch.go:115`). Our UI hides the controls AND its watchers
reset them to `false` on edit, destroying values set elsewhere.

- [ ] `supportsLivePlatform` / `supportsMessagesDispatchPlatform` in `groupsMessagesDispatch.ts`
- [ ] swap gates at `GroupsView.vue:1552,1585,3255,3288`
- [ ] split watchers at `:6509,:6557,:6605` — live and dispatch reset independently
- [ ] keep `messages_dispatch_model_config` OpenAI-only, as upstream does

---

## P2 — backend ready, UI absent

### 5. Time-of-day pricing + Fast/Flex multipliers — TODO
Backend persists them; our form round-trip drops them, and the repo replace is
destructive. Not a live leak (nothing in our product can set them today) but the
configuration surface is missing entirely.

- [ ] `api/admin/channels.ts` — `fast_multiplier`, `flex_multiplier`, `time_pricing`, `ChannelTimePricing`, the 4 interval multipliers
- [ ] `components/admin/channel/types.ts` — `TimePricingFormEntry`, `createDefaultTimePricingForm`, `apiTimePricingToForm`, `formTimePricingToAPI`, `validateTimePricing`, `isValidPositiveMultiplier`
- [ ] new `components/admin/channel/TimePricingSection.vue`
- [ ] `IntervalRow.vue` multiplier inputs; `PricingEntryCard.vue` wiring + clear periods on billing-mode switch
- [ ] `ChannelsView.vue` — carry all fields through `apiToForm` AND `formToAPI` (:1115, :1211), plus the submit guard
- [ ] i18n ~38 keys

### 6. Channel monitor quota mode — TODO
- [ ] `constants/channelMonitor.ts` — providers, `CHECK_MODE_*`, `QUOTA_ONLY_PROVIDERS`
- [ ] `api/admin/channelMonitor.ts` — `CheckMode`, `MonitorQuotaSnapshot`, `latest_quota`, `account_id`
- [ ] `composables/useChannelMonitorFormat.ts` — `formatMonitorModel`, `checkModeLabel`, badge classes
- [ ] new `components/common/MonitorQuotaView.vue`
- [ ] `MonitorFormDialog.vue` — check mode, linked account, `account_id=0` unbind, picker race guard; endpoint must stop being `required` in quota mode
- [ ] `common/Select.vue` — `remote`/`loading`/debounced search (prereq)
- [ ] `formatMonitorModel` across 4 cells, else `"quota"` renders as a model name
- [ ] `channel_monitor_show_quota` settings toggle + i18n

### 7. Plugin management — TODO (security-sensitive)
- [ ] `api/admin/plugins.ts` (8 endpoints), `admin/index.ts` barrel
- [ ] new `views/admin/PluginsView.vue` — **must port `684d9efb1` and `391d69e08` with it**: `sandbox="allow-scripts"` and the `event.origin !== "null"` postMessage guard are the security boundary for third-party plugin code
- [ ] route, `stores/app.ts` flag, `utils/featureFlags.ts`, sidebar nav
- [ ] i18n 46 keys + `nav.plugins` (note: our shell nav lives under `shell.*`, not `nav.*`)

### 8. Codex catalog / routed Codex — TODO
- [ ] new `api/codex.ts`, new `utils/codexCatalogConfig.ts`
- [ ] `UseKeyModal.vue:721,943,984` — stop emitting `model_reasoning_effort = "xhigh"` unconditionally; gate on the model's catalog entry
- [ ] i18n `keys.useKeyModal.{deepseek,composite,routedCodex,codexModelCatalog}.*`

### 9. Long-context billing gate — TODO
- [ ] new `components/account/longContextBilling.ts`
- [ ] gate the billing toggle in `CreateAccountModal.vue:2917` and `EditAccountModal.vue:1915`
- [ ] **the trap:** upstream first attached this `v-if` to the WS-mode div by mistake (`d4d2c746c`). Billing toggle only.

### 10. Auto-reset credit + upstream-sync warnings — TODO
- [ ] `api/admin/accounts.ts` — widen `SyncUpstreamModelsResult` with `metadata`, `warnings`
- [ ] surface `upstream_model_metadata_incomplete` — today we show a false success toast
- [ ] `ModelWhitelistSelector.vue` `upstream-synced` emit; `OpenAIQuotaResetCell.vue` badge
- [ ] i18n `autoResetCredit.*`, `openaiQuotaReset.autoStatus.*`

### 11. Ops back-to-list + diagnostics — TODO
- [ ] `resumeState` / `detailReturnTarget` across `OpsDashboard.vue` + both modals
- [ ] `upstream_status_code` tile in `OpsErrorDetailModal.vue` (the rest of that rewrite is a design difference, not a missing fix — ours solves it differently)
- [ ] new `OpsEmailNotificationCard.vue`, `OpsRuntimeSettingsCard.vue`
- [ ] i18n `errorDetail.*` + port `opsLocaleKeys.spec.ts` together

---

## P3 — small, independent

- [ ] `UserEditModal.vue:120` — accept concurrency `0` (backend: `<= 0` is unlimited). Needs `concurrencyNonNegative` key. `BulkEditUserModal` already allows it, so this is a two-modal inconsistency.
- [ ] `ProxiesView.vue:1290` — IPv6 regex + bracket strip
- [ ] `PlazaModelPricingTable.vue:325` — `sortByContext`; tiers can render out of order
- [ ] `AccountUsageCell.vue` — hide Grok prepaid / used-limit at `$0`; drop the post-reset refresh suppression (backend `CachePostResetSnapshot` confirmed present)
- [ ] `UserDashboardStats.vue` — include cache tokens in the breakdown
- [ ] `AccountsView.vue:637` show priority by default; `:2376` `Promise.allSettled`
- [ ] `UserAllowedGroupsModal.vue` — `restrict_public_groups` toggle. **Not urgent:** the flag is unreachable in our build today, but shipping the toggle without the save-filter fix would make it reachable AND broken. Do both together.
- [ ] `HomeView.vue` — Model Plaza entry for anonymous visitors

---

## Do NOT do

- Do not delete `admin.users.concurrencyMin` because upstream did — `UserEditModal.vue:121` still uses it.
- Do not delete `soraClient` / `soraS3` / `soraStorageQuota` as part of an i18n port. Dead in our tree, but that is a separate dead-code decision.
- Do not `cp` any locale file. The June-only key list (`shell.*` 34 keys, `dashboard.tile*`, `ops.resources.*`, `profile.settings.*` 25 keys, and more) is in the port list doc; a `cp` destroys all of it and june-lint's file count *improves* when it happens.

## Known parity gaps in our own tree (not upstream's fault)

- `en/admin/settings.ts` has `payment.field_keyId`, `field_keySecret`, `providerRazorpay`, `razorpayWebhookHint`; `zh/` does not.
- 25 `profile.settings.*` / `profile.avatar.*` keys exist in our EN and not our ZH.

---

# The full frontend gap — three separate things

The feature list above is only ONE of three. Measured 2026-08-28 with the
runbook's own stale detection, not estimated.

| gap | size | what it is |
|---|---|---|
| **A. Never built** | **17 files** | Upstream has them, we have no counterpart. This is the feature list above. |
| **B. Stale** | **52 files** | Upstream changed them since vendoring; our copy is upstream's OLD version, untouched. |
| **C. Never converted** | **94 `.vue` files** | Byte-identical to the mirror. Still literally upstream's code and design — the June rewrite never reached them. |

**A and B overlap heavily** — most stale files are the ones carrying the features
above. **C overlaps with neither**: zero of the 94 unconverted files are also
stale, which makes sense — nobody has touched them since vendoring, so upstream
hasn't diverged from them either.

## C is the one nobody was counting

`.vue` files: **94 of 292 are still byte-identical to the mirror** — 32% of the
UI is unconverted. By area: 73 components, 15 views, 6 features.

The 15 unconverted views:

```
NotFoundView              ModelPlazaView            admin/UsersView
admin/PromoCodesView      user/PaymentQRCodeView    user/ChannelStatusView
user/ChannelStatusV1View  user/StripePopupView      user/BatchImageGuideView
admin/settings/OpenAIFastPolicyUserSelector         admin/settings/EmailTemplateEditor
admin/affiliates/AdminAffiliateRebatesView          admin/affiliates/AdminAffiliateRecordsTable
admin/affiliates/AdminAffiliateTransfersView        admin/affiliates/AdminAffiliateInvitesView
```

`admin/UsersView` and `ModelPlazaView` are core screens. The whole affiliates
section is untouched.

**The 143 identical `.ts` files are NOT debt.** API clients, utils and types have
no design surface — identical is the correct state for them, and converting them
would be meaningless churn. Only the `.vue` count measures design debt.

## Why this matters for sequencing

A file in C is cheap to sync and expensive to convert. A file in B is the
opposite. Do not batch them:

- **C** — porting upstream's newest version is a `cp`, because we never touched
  ours. Then it needs the full June conversion, which is the real cost.
- **B** — never `cp`. Our copy diverged deliberately; a copy destroys that. Hand-merge.

The rule is the same as always: **if it appears in `git diff $VB..HEAD --
inferno-frontend`, hand-merge it; if it does not, a wholesale copy is safe.** That
single check separates B from C mechanically.

## Regenerate these numbers

```sh
VB=$(git log --format=%H -1 --grep='vendor upstream frontend as the redesign target')
diff -rq frontend/src inferno-frontend/src | grep '^Files' | sed 's|Files frontend/||; s| and .*||' | sort > /tmp/differ
git diff --name-only "$VB..HEAD" -- inferno-frontend | sed 's|^inferno-frontend/||' | sort -u > /tmp/ours
comm -23 /tmp/differ /tmp/ours          # B: stale
# C: identical .vue files
find frontend/src -name '*.vue' ! -path '*__tests__*' | while read f; do
  r="${f#frontend/}"; [ -f "inferno-frontend/$r" ] && cmp -s "$f" "inferno-frontend/$r" && echo "$r"
done
```

---

# Gap D — born stale (measured 2026-08-29)

**The vendor point is not a clean baseline.** At commit `47b1130cb`, `frontend/`
and `inferno-frontend/` were NOT identical: **68 files already differed**. We
copied the mirror from an earlier upstream state than the mirror itself held.

```sh
VB=$(git log --format=%H -1 --grep='vendor upstream frontend as the redesign target')
git diff --name-only "${VB}:frontend/src" "${VB}:inferno-frontend/src"
```

(Quote `${VB}` with braces. Bare `$VB:frontend` is eaten by zsh's `:f` history
modifier and the command fails silently, reporting zero.)

## Why every prior count missed it

**51 of the 68 are invisible to the stale detector.** That detector asks: *differs
from the mirror AND not in our changed set?* These files ARE in our changed set —
we converted them to June — so they were classified as "ours, deliberately
changed" and assumed current.

**A converted file can also be missing upstream work from before its conversion.**
Converting it made it look like ours. Nobody asked whether it was current at the
moment we converted it. Conversion masks staleness permanently, because from then
on the file differs from the mirror for a legitimate reason.

## Confirmed on a real file, not inferred

`components/account/AccountUsageCell.vue` was born missing Grok plan-detection
(`grokPlanLabelIsFree`/`IsPaid`, the `showPrepaid` guard). It never arrived:
`showPrepaid` appears 5 times in the mirror and **0** times in ours. That is the
same Grok `$0` chip defect the component audit found — its real origin is the
vendoring moment, not a later upstream change.

## What is in the 68

Major converted files, which is the worrying part: `AppSidebar.vue`,
`ChannelsView.vue`, `GroupsView.vue`, `SettingsView.vue`, `AccountsView.vue`,
`EditAccountModal.vue`, `CreateAccountModal.vue`, `BulkEditAccountModal.vue`,
`AccountUsageCell.vue`, `UsageTable.vue`, `PricingEntryCard.vue`, 12 locale
files, `types/index.ts`, `router/index.ts`, `main.ts`, and most of
`components/common/` (Input, Select, Toggle, TextArea, BaseDialog,
ConfirmDialog, GroupSelector, ProxySelector, SearchInput, DateRangePicker).

3 no longer exist on either side. 9 have since caught up.

## How to work it

`cmp` is useless here — a converted file always differs, so "differs today"
proves nothing. For each of the 68, diff **our current version** against the
**mirror's version at the vendor commit** to isolate what we were born missing,
then check whether that specific content ever arrived:

```sh
git diff "${VB}:inferno-frontend/src/<path>" "${VB}:frontend/src/<path>"
```

Everything that diff adds is content we never had. Grep our current file for it.

Treat D as part of Phase 1: it is sync work, not conversion work, and it is the
category most likely to be carrying live defects, because these are the files we
use most.
