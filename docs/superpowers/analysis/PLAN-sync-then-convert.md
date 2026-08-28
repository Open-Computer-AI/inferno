# Plan: sync to upstream parity first, convert to June second

## The decision, and why it is the cheap order

**Sync everything to upstream's current state BEFORE converting anything else to
June.** The reason is that a file's cost *flips* the moment you convert it:

| | sync cost | design |
|---|---|---|
| **unconverted file** | free — wholesale copy, we never touched it | upstream's |
| **converted file** | expensive — hand-merge, forever, on every reconcile | June |

Convert first and you pay the conversion once, then a hand-merge on every future
upstream change to that file. Sync first and you pay the copy (free), convert
once, and only then start paying hand-merge. Same end state, strictly less work.

It also means **every conversion is a permanent tax**, exactly like a divergence
ledger row. Convert deliberately, in priority order — not because a file happens
to be next alphabetically.

## What the three gaps actually need

Measured 2026-08-28, not estimated.

| gap | size | needs |
|---|---|---|
| **A. never built** | 17 files | build |
| **B. stale** | 52 files | **hand-merge** — ours diverged deliberately |
| **C. never converted** | 94 `.vue` | **conversion only** |

**C needs no sync at all.** All 94 are already byte-identical to upstream's
current tip — upstream has not touched them since we vendored on 2026-08-09. That
was the surprise: the biggest gap is also the one with zero sync work in it.

So Phase 1 is A + B only. C is entirely Phase 2.

---

## Phase 0 — guardrails first

Do not start until these exist. Every one of them is a lesson already paid for.

- [ ] **The classifier.** One script that answers, per file: copy or hand-merge?
      The rule is mechanical — if it appears in
      `git diff $VB..HEAD -- inferno-frontend`, hand-merge; if not, copy is safe.
      This is the single check that would have prevented the 2026-08-11 incident.
- [ ] **The count guardrail.** Record `june-lint`'s **converted file count**
      (287 as of 2026-08-28) before and after every batch. If it *drops*, a copy
      overwrote converted work. A health signal that improves when work is
      destroyed is the one to distrust.
- [ ] **Never copy a locale file.** i18n is always hand-merge, no exceptions. The
      June-only key inventory is in `2026-08-28-upstream-frontend-port-list.md`.
- [ ] **Baseline the gate.** `check-divergence.sh` green, `vue-tsc` clean, vitest
      count recorded. A test count that drops is the same tell as the lint count.

---

## Phase 1 — SYNC to upstream parity

Goal: **every feature upstream has, works in our app.** Design is explicitly not
the goal here; a new screen may land in upstream's Tailwind and be converted in
Phase 2. That is acceptable and consistent — the app already carries 94 such
screens today.

### 1.1 Types and API first (unblocks everything else)
- [ ] widen `AccountPlatform` / `GroupPlatform` with `kimi | zhipu | deepseek`
- [ ] new `constants/platforms.ts`; replace the 8 hardcoded platform lists
- [ ] `api/admin/channels.ts` — multipliers, `time_pricing`, interval multipliers
- [ ] `api/admin/channelMonitor.ts` — `check_mode`, `latest_quota`, `account_id`
- [ ] `api/admin/accounts.ts` — `metadata`, `warnings` on sync-upstream
- [ ] `api/modelPlaza.ts` — `time_pricing`, `long_context_basis`, `intervals`
- [ ] new `api/admin/cnProviders.ts`, `api/codex.ts`, `api/admin/plugins.ts`

**Exit:** `vue-tsc` clean. Nothing renders differently yet — this phase only
makes the shapes correct.

### 1.2 The 52 stale files — hand-merge, in dependency order
Work them in this order; later ones depend on earlier ones.

- [ ] `utils/`, `constants/`, `composables/` (4 files) — pure logic, no design
- [ ] `components/admin/channel/types.ts` — form helpers everything else needs
- [ ] account modals + `credentialsBuilder.ts` + `ModelWhitelistSelector.vue`
- [ ] monitor components (4 cells + form dialog)
- [ ] `views/admin/ChannelsView.vue`, `GroupsView.vue` — the big two
- [ ] ops components
- [ ] remaining leaf components

**Rule for every one:** keep upstream's structure, re-apply our June changes on
top. Never take our whole file — that resolves the diff and silently deletes
whatever upstream fixed in lines we did not care about.

### 1.3 The 17 missing files
Land them as upstream ships them (unconverted). They are new surface, so there is
no June work to preserve and nothing to hand-merge later until we convert them.

- [ ] `constants/platforms.ts`, `utils/codexCatalogConfig.ts`, `components/account/longContextBilling.ts`
- [ ] CN cells (3), `MonitorQuotaView.vue`, `TimePricingSection.vue`
- [ ] `views/admin/PluginsView.vue` — **must ship with `684d9efb1` + `391d69e08`**;
      the `sandbox="allow-scripts"` attribute and the `event.origin !== "null"`
      guard ARE the security boundary for third-party plugin code
- [ ] `OpsEmailNotificationCard.vue`, `OpsRuntimeSettingsCard.vue`, `AccountStatsModal.vue`
- [ ] routes, store flags, feature flags, sidebar entries

### 1.4 i18n — hand-merge only, last
- [ ] `openaiFastPolicy` 8-key rename **atomically** (live consumers today)
- [ ] `admin.groups.platforms.{kimi,zhipu,deepseek}` (visibly broken today)
- [ ] `createFirstAnnouncement` + fix `AnnouncementsView.vue:151`
- [ ] the feature key clusters, per feature, as each lands
- [ ] rewrite every en/em dash out — `june-lint` bans both in `i18n/**.ts`
- [ ] do NOT delete `concurrencyMin`, `soraClient`, `soraS3` just because upstream did

### 1.5 The loose fixes
concurrency `0`, IPv6 regex, composite live/dispatch, `sortByContext`, Grok `$0`
chips, post-reset refresh suppression, cache tokens in the dashboard breakdown,
`Promise.allSettled`, priority column default.

### Phase 1 exit criteria
- [ ] `diff -rq frontend/src inferno-frontend/src` differs **only** on files in
      `git diff $VB..HEAD -- inferno-frontend` — i.e. zero stale files remain
- [ ] gate green; `vue-tsc` clean; vitest count ≥ baseline
- [ ] june-lint converted **count** ≥ 287 (it will rise as files gain June work)
- [ ] every P1 item in `PORT-TRACKER.md` closed

---

## Phase 2 — CONVERT to June

Now every file is current, so each conversion happens once against a stable
target. 94 `.vue` files: 73 components, 15 views, 6 features.

**Convert in this order, because each conversion starts the hand-merge tax:**

1. [ ] **`admin/UsersView`** and **`ModelPlazaView`** — core screens, high traffic
2. [ ] **`user/ChannelStatusView`** — user-facing
3. [ ] the 73 components, bottom-up: leaf primitives before the screens that use
       them, so a screen is converted against already-June children
4. [ ] `admin/PromoCodesView`, `user/PaymentQRCodeView`, `BatchImageGuideView`
5. [ ] the 4 `admin/affiliates` views — a whole untouched section
6. [ ] `EmailTemplateEditor`, `OpenAIFastPolicyUserSelector`, `NotFoundView`,
       `StripePopupView`, `ChannelStatusV1View` — low traffic, defer freely

**Per file:** June tokens, no raw Tailwind, no `dark:`, no hardcoded English —
`t()` and report the key. Run `june-lint` after each; the count must rise.

**A real option worth keeping open:** a rarely-seen screen can stay unconverted
indefinitely. Unconverted costs nothing to maintain; converted costs a hand-merge
forever. `ChannelStatusV1View` (a legacy view) is a fair candidate to never
convert.

---

## Phase 3 — steady state

Once Phases 1 and 2 land, the daily routine holds the line:

- backend: merges itself (Runbook v2)
- frontend: upstream's delta lands as a hand-merge on converted files, a copy on
  unconverted ones — the classifier decides
- new upstream screens: land unconverted, convert when they earn it

**The failure this whole plan exists to prevent:** the backend half of a feature
arriving automatically while the frontend half waits on a human, silently, for
weeks. That is what happened between 2026-08-16 and 2026-08-28.

---

## Risks

| risk | mitigation |
|---|---|
| A copy overwrites converted June work | The classifier, plus the june-lint count guardrail. This has already happened once. |
| Phase 1 lands 17 unconverted screens and the app looks inconsistent | Accepted, and temporary. 94 such screens already ship today. |
| A stale hand-merge silently drops an upstream fix | Never take our whole file. Keep upstream's structure, re-apply ours. |
| Plugin view ships without its origin guard | Treat the two security commits as part of the file, not as follow-ups. |
| Upstream moves during the work | It will — ~40 commits/day. Re-run the stale detection at the start of each batch; do not trust a list from last week. |
