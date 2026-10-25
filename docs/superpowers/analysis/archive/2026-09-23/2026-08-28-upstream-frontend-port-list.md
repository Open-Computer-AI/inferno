# Upstream frontend port audit — 2026-08-28

Six read-only agents. Baseline: vendor point `47b1130cb`, upstream base
`baeac1f3d`, upstream tip `e866ff6ec`. Five produced claims; a sixth was told to
**refute** them and to default to "refuted" when it could not confirm.

**It refuted half.** That is the headline. Four of eight high-severity claims did
not survive, and two of the four survivors were materially reframed. Everything
below is post-verification.

## The structural finding

Not "we are behind upstream". **Our own Go backend already ships capabilities our
June frontend cannot reach.** The backend half of an upstream feature arrives
automatically by `git merge`; the frontend half arrives by manual port under a
rule that skips `components/` and `views/`. The two halves of one feature take
different paths and one silently does not arrive.

Gate 5 cannot see this. Both halves are correctly declared divergence.

## Confirmed — worth doing

### 1. Composite groups cannot enable Live or Messages dispatch
The only confirmed item whose harmful precondition is fully reachable in our
product.

- `composite` is a first-class option in our own platform selector
  (`views/admin/GroupsView.vue:4618,4628`).
- Our backend explicitly permits both flags for it: `service/admin_group.go:513`
  and `:893` clear `AllowLive` only when the platform is neither `openai` nor
  `composite`; `service/openai_messages_dispatch.go:115` likewise.
- Our UI gates both on `platform === 'openai'` (`GroupsView.vue:1552, 1585,
  3255, 3288`) and its watchers (`:6509, :6557, :6605`) actively reset them to
  `false`. Vue's default `flush: 'pre'` defers the callback past
  `openEditModal`, so the reset lands *after* the form is populated and the
  payload submits `false`.

So the controls are hidden AND any value set out-of-band is destroyed on the next
edit. Upstream models it as `supportsLivePlatform` /
`supportsMessagesDispatchPlatform`. Fix: port those two helpers, swap the four
gates, split the watchers.

### 2. Channel pricing UI is missing — reframed, NOT a live billing leak
The first pass called this silent billing corruption. The verifier could not show
the precondition is reachable, and neither could I.

- Real: `views/admin/ChannelsView.vue:1115-1127` emits no `fast_multiplier`,
  `flex_multiplier` or `time_pricing`, and our `ChannelModelPricing`
  (`api/admin/channels.ts:24`) does not declare them. Zero hits tree-wide.
  Our `PricingInterval` also omits the four interval multipliers.
- Real: the backend replace is destructive —
  `repository/channel_repo_pricing.go:305` DELETEs then re-inserts, and
  `handler/admin/channel_handler.go:371` maps a missing `time_pricing` to nil.
- **But** nothing in our product can populate those columns. Our Dockerfile
  builds only `inferno-frontend`; the upstream `frontend/` is never served. On a
  greenfield Inferno DB every column is NULL, so there is nothing to erase.

Treat as **"restore missing pricing configuration UI"**, not "stop a billing
leak". The erasure needs an inherited upstream DB or direct admin-API writes.

### 3. Concurrency 0 rejected in the per-user modal — cosmetic
`components/admin/user/UserEditModal.vue:120` rejects `< 1`; backend treats
`<= 0` as unlimited (`service/concurrency_service.go:343, :382`). But
`BulkEditUserModal.vue:119` already accepts and sends `0`, so the capability is
not lost — it is an inconsistency between two modals, and it fails loudly with a
toast.

### 4. IPv6 proxies rejected on batch import — minor
`views/admin/ProxiesView.vue:1290`'s host class `[^:]+` cannot match
`[2001:db8::1]`. Backend supports it (`service/proxy.go:42`, `net.JoinHostPort`).
Not silent — counted as `invalid` and rendered (`:557`). The single-add form
accepts a bare IPv6 host today, so there is a working path.

### 5. `UseKeyModal` writes `model_reasoning_effort = "xhigh"` unconditionally
`components/keys/UseKeyModal.vue:721, 943, 984`, no guard. Upstream extracted
`utils/codexCatalogConfig.ts` to emit it only when the selected Codex model's
catalog entry supports a non-`none` effort.

### 6. Data-only ports, no styling
New Kimi models (`kimi-for-coding`, `kimi-k2`), the `kimi` platform alias, and
Grok's `grok-imagine-image-2.0` in `composables/useModelWhitelist.ts`. Note our
`utils/platformColors.ts` is **not** June-tokenised — it is still literal
Tailwind, so a verbatim merge is correct there. The reflex to "convert to June
tokens" would be wrong for that file.

## Refuted — do not re-raise

| claim | why it failed |
|---|---|
| `UserAllowedGroupsModal` revokes public groups | `restrict_public_groups` has **zero** references in `inferno-frontend/`, the column defaults to `false`, and neither our create nor update path sends the key. The flag can never be true in our build, so `CanBindGroup` short-circuits before consulting `AllowedGroups`. Latent only if a UI toggle ships later. |
| Stale balance on `PaymentResultView` | That page renders no balance — order id, amounts, credited only. `PaymentView.vue:492` and `:993` already call `refreshUser()` before redirecting, four other balance surfaces refresh on mount, and `stores/auth.ts:150` polls every 60s. |
| False "SLA critical" on an empty window | `OpsDashboardHeader.vue:454` early-returns a "system idle" item *before* the SLA diagnostics at `:563`. Guarded by a different route than the claim looked for. Only a cosmetic `0.000%` label survives in `OpsTrafficSplit.vue:65`. |
| Plaza renders wrong price in time-pricing windows | `time_pricing` does not exist anywhere in our tree, so there are no windows to render wrongly. Only an unsorted-tier nit survives (`PlazaModelPricingTable.vue:325`), which shows correct numbers in the wrong sequence. |


## i18n — 22 locale files

Delivered on a second pass; the first returned nothing. It then **corrected three
of its own sub-agent's claims** — `autoResetCredit.*`,
`openaiQuotaReset.autoStatus.*` and `syncUpstreamModelsMetadataIncomplete` were
each reported as having live consumers and have none. Same lesson as the main
verification pass: "a key exists and a plausible component exists" is not "that
component references that key".

### The one visibly broken today

`admin.groups.platforms.{kimi,zhipu,deepseek}` — our code calls
`t('admin.groups.platforms.' + platform, platform)` with the raw id as fallback,
so any group tagged with a CN platform **renders `kimi` / `zhipu` / `deepseek`
literally instead of a label, right now**. Every other missing key is inert
because the UI that would render it does not exist yet.

### MUST SURVIVE — June-only keys a `cp` would destroy

Large and load-bearing. Highlights: `common.ts` `shell.*` (36 keys),
`auth.{goBack,hidePassword,legalPrefix,...}`, `groupCapacity.*`,
`sectionedDialog.*`; `admin/overview.ts`'s whole `dashboard.*` analytics
namespace (`tile*`, `verdict*`, `mix.*`, `modelMix.*`, `period.*`);
`admin/ops.ts` `alertEvents.*`, `resources.*`, `systemLogs.*`, `traffic.*`;
`admin/settings.ts` `pageDescriptions.*`, `appearance.*`, `groups.*`;
`admin/accounts.ts` `usageWindow.*`, `capacity.dimension.*`.

This is exactly what the 2026-08-11 wholesale `cp` destroyed. Hand-merge only.

### Do NOT delete what upstream deleted

`admin.users.concurrencyMin` was removed upstream but is **actively used** by our
`UserEditModal.vue:121`. Removing it because upstream did would break our
validation. Same for the orphaned `soraClient` / `soraS3` / `soraStorageQuota`
blocks — dead in our tree, but that is a separate dead-code decision, not part of
an i18n port.

### Atomic units, not piecemeal

`admin.settings.openaiFastPolicy.*` is an 8-key "whitelist → target models"
terminology rename. Merge all eight or none, or the UI shows mixed vocabulary.
Beware the unrelated `betaPolicy.modelWhitelist` block in the same file.

### Style violations if copied verbatim — 3 em dashes, EN only

`admin.accounts.cnProviders.apiProtocol.anthropicDesc`, `.responsesDesc`, and
`dashboard.modelPlaza.table.timePricingRowHintPeak`. The ZH counterparts are
clean, so the risk is specific to the EN files. No emoji anywhere.

### Two live i18n defects — do these first

**1. Our settings page renders stale terminology today.**
`admin.settings.openaiFastPolicy.*` — 8 keys whose values upstream rewrote in a
"whitelist to target models" rename. Ours still hold the OLD strings, and unlike
every other key in this audit they have **live consumers**:
`components/admin/settings/AdminGatewaySettingsPage.vue`,
`views/admin/SettingsView.vue`, `views/admin/settings/OpenAIFastPolicyUserSelector.vue`.
Merge all eight atomically or the page shows mixed vocabulary. Beware the
unrelated `betaPolicy.modelWhitelist` block in the same file. Port
`__tests__/openaiFastPolicyLocales.spec.ts` in the same change — it asserts the
new strings and that the hints no longer contain "whitelist"/"白名单".

**2. Empty announcements list shows a "failed to load" error.**
`views/admin/AnnouncementsView.vue:151` passes
`t('admin.announcements.failedToLoad')` as the description of its **empty-state**
slot. Upstream uses `createFirstAnnouncement` there. So an admin with no
announcements is told loading failed. Found incidentally by the locale audit, not
by the component pass. Needs the new key AND the `.vue` reference changed.

### Corrections the EN pass made to its own parent

`dashboard.ts` was reported as having zero June-only keys; it has **25**, all
under `profile.settings.*` / `profile.avatar.*`. Those would have been destroyed
by a `cp` that the parent's report said was safe.

### An EN/ZH parity gap, unrelated to upstream

Our own `en/admin/settings.ts` has `payment.field_keyId`, `field_keySecret`,
`providerRazorpay` and `razorpayWebhookHint`; our `zh/` file does not. Likewise
25 `profile.*` keys exist in our EN and not our ZH. That is our own untranslated
surface, independent of this reconcile.

### Style: six dash violations, not three

`june-lint` ground rule 2 bans **both** en and em dashes in `i18n/**.ts`. The
added upstream strings carry three em dashes and three en dashes (including
"0.1–100" and "Mon–Fri", which read as innocent numeric ranges and are not).
Rewrite each on merge. Zero emoji anywhere.

## Zero security regressions

Both classifiers grepped specifically for `sanitize|escapeHtml|encodeURI|innerHTML|
v-html|window.open|target=_blank|rel=` across all 58 diffs. No hits. The one
security-relevant upstream commit hardens the plugin iframe bridge, inside a view
we do not have. **The skip rule hid bugs, not holes.**

## The rule change both classifiers proposed independently

Replace *"ignore `frontend/src/{components,views}`"* with:

> Ignore `<template>` and `<style>` diffs. **Always review `<script setup>` and
> `.ts` hunks.**

Only 6 of 58 files in this window were genuinely pure presentation. The narrowed
rule catches every defect above for the cost of reviewing 4 extra files.

## Method note

Half of a confident, well-evidenced finding set did not survive an agent whose
only instruction was to attack it. Reading the code and confirming its shape is
not the same as confirming the defect is **reachable** — three of the four
refutations turned on reachability, not on the code being different. Any future
audit of this kind needs the adversarial pass, not just the finding pass.
