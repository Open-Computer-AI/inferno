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
| 5 | `./scripts/check-divergence.sh` | every changed file appears in the divergence ledger below |
| 6 | `node scripts/conversion-status.mjs` | remaining count strictly lower than the line above |

Gate 5 changed meaning on 2026-08-15. It used to require the diff be **empty**.
That was right while Inferno was a pure restyle and wrong the moment it started
becoming its own product: upstream is not going to grow the features we want,
so backend divergence is the goal, not a defect. Requiring zero forbade the
product from existing.

It now requires that divergence be **declared**. The failure it still catches is
the one that actually happened: `84a3c4ac`, a commit whose subject said
`feat(ui):`, quietly regenerated ent and added a migration. Accidental drift is
still fatal. Deliberate drift is legal, listed, and survives reconciles.

**Measure against the base HEAD sits on, never against a freshly fetched
`upstream/main`.** Against a stale ref it under-reports; against a fresh one it
counts upstream's own movement as our drift. On 2026-08-15 the same tree read
19 files against the base and 246 against fresh upstream, of which 227 were
upstream moving on. `check-divergence.sh` uses `git merge-base`, which is the
only ref that answers "what have *we* changed". The daily reconcile gets this
right by rebasing (step 3) *before* asserting (step 4).

### The divergence ledger

Anything under `backend/`, `frontend/`, `deploy/` or `docs/` must appear here
**and** in `DECLARED` inside `scripts/check-divergence.sh`. A file that differs
and is not listed is a bug, not a feature.

| # | area | files | why | re-apply after rebase |
|---|------|-------|-----|-----------------------|
| D1 | `avatar_seed` on `users` | `ent/schema/user.go` (3 lines) + `ent/` regeneration (8 files, 286 lines) + `dto/{types,mappers}.go`, `user_handler.go`, `api_key_repo.go`, `user_repo.go`, `service/user.go`, `user_service.go` (15 lines) + `migrations/900_add_user_avatar_seed.sql` | Persists a regenerated identicon server-side so it survives reload and syncs across devices. Upstream stores avatars in a separate table and has no seed concept. | **Do not hand-merge the `ent/` files.** Re-run `go generate ./ent` and let codegen rebuild from the 3-line schema. Only `ent/mutation.go` realistically conflicts — it is shared across every entity, so any upstream schema change touches it. |
| D2 | English legal-document defaults | `service/setting_public.go` (4 strings) + `server/api_contract_test.go` (golden fixture) | Chinese defaults on the legal documents of a rebranded English product. Applies only when no `login_agreement_documents` settings row exists — i.e. a fresh install, which is exactly when it matters. | Trivial. Zero upstream commits to this file in the 124 we were behind. |
| D3 | `.gitignore` | appended negations | Lets `inferno-frontend/` and `docs/superpowers/` be tracked (upstream allowlists `docs/*`). Outside the four gated dirs, so the script does not see it; listed for completeness. | Trivial. |
| D4 | OpenComputer portal design specs | `docs/superpowers/specs/*.md` | Design docs for making Inferno the OC Portal (OAuth authorization server, agent registry, billing contract) — replacing Nous Portal for the hermes-agent client. Docs only, no code. Upstream has no equivalent and never will. | Trivial — additive files, cannot conflict. |
| D5 | Org tenancy (`orgs` + `org_members`) | `ent/schema/{org,org_member}.go` + `ent/` regeneration (20 files, incl. incidental `ent/group.go` comment-only drift picked up by the same `go generate ./ent` run) + `migrations/901_org_and_members.sql` + `migrations/904_org_personal_user_id.sql` + `service/{org_service,org_service_test}.go` + `service/{auth_service,wire}.go` + `cmd/server/wire_gen.go` + `cmd/jwtgen/main.go` + 16 test files threading the new `NewAuthService` `orgService` param | OAuth authorization server plan Task 1: minimal org tenancy so every later task can reference `org_id`. Every user gets an idempotent personal org on signup (email or OAuth), created best-effort — failure logs a warning and does not block signup, since `EnsurePersonalOrg` repairs on next login. Migration numbering: 900 already taken (D1), so this is 901, first of the fork's 9xx series; 902/903 are reserved by this same plan's later tasks (oauth_client, oauth_device_authorization), so the concurrency-safety follow-up below is 904. Ledger numbering: D4 already taken (design specs, committed same day), so this is D5. Org-role constants are named `OrgRoleOwner`/`OrgRoleAdmin`/`OrgRoleMember` (not `RoleOwner`/`RoleAdmin`/`RoleMember`) because `service.RoleAdmin` already exists as the platform-wide user role (`domain.RoleAdmin == "admin"`); the wire-format string values themselves (`"OWNER"`/`"ADMIN"`/`"MEMBER"`, read verbatim by the desktop client) are unchanged. `migrations/904_org_personal_user_id.sql` adds `orgs.personal_user_id` (nullable, unique where non-null) — a Task 1 review found `EnsurePersonalOrg`'s original read-then-create had no DB-level invariant, so two near-simultaneous calls for a new user (double-fired OAuth callback, two tabs) could each create a distinct personal org; `EnsurePersonalOrg` now looks up and inserts by `personal_user_id` and treats a unique-constraint violation on create as "lost the race, re-read the winner's row" rather than an error. Upstream has no tenancy concept. | **Do not hand-merge the `ent/` files.** Re-run `go generate ./ent` and let codegen rebuild from the two schema files; then re-run `cd backend/cmd/server && go generate ./...` to regenerate `wire_gen.go` from `wire.go`. Only `ent/mutation.go` and `ent/runtime/runtime.go` realistically conflict, for the same reason as D1. |

**Better mechanism for D2 when someone has time:** seed the four titles as a
`login_agreement_documents` settings row instead. Admin-editable, and it touches
no Go at all, which retires this ledger entry.

**Before adding an entry:** prefer frontend-only, then an existing backend field,
then upstreaming it as a PR to `Wei-Shaw/sub2api`, and only then a new entry
here. Each entry is a permanent tax on every future reconcile. `avatar_seed` was
checked against riding on the existing `avatar_url` field first — it cannot,
because `SetAvatar` runs `normalizeUserAvatarInput`, which rejects a bare seed.

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

`conversion-status.mjs` now reports 129/324 converted files; the two dead ops
cards below have been removed rather than counted as converted. Every mounted
ops surface listed above was opened in a browser against seeded data and
measured, not just typechecked.

**To see the OpenAI token stats card at all:** it is hidden by default. Ops
page → Settings → Advanced settings → "Display OpenAI token request stats".
The flag lives in `getAdvancedSettings`, not in the `settings` table, so there
is no row to flip directly.

#### ✅ Two dead ops cards removed; controls consolidated

`OpsRuntimeSettingsCard.vue` (118 utils, 537L) and
`OpsEmailNotificationCard.vue` (121 utils, 442L) have **zero references
anywhere in `src/`** — no import, no `defineAsyncComponent`, no `<component
:is>`. They are also unmounted in `upstream/main`; they arrived dead in
`d464c0f0` when the upstream frontend was vendored wholesale.

They are superseded by `OpsSettingsDialog.vue`, which calls the same two API
pairs (`get/updateAlertRuntimeSettings`, `get/updateEmailNotificationConfig`)
and renders the same fields.

**Resolution:** the alert-silencing and distributed-lock controls from the dead
runtime card are now editable in `OpsSettingsDialog.vue`, and both dead cards
are deleted. The dialog still sends the complete runtime object to the existing
single PUT endpoint, so fields outside the form continue to round-trip intact.

Silencing is validated before save using the existing RFC3339, positive rule ID,
`P0..P3` severity, and lock-key/TTL rules. Focused coverage lives in
`OpsSettingsDialog.spec.ts`.

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

### 4. Remaining Chart.js — ✅ COMPLETE

Commit `6db5fef1` removes the final runtime Chart.js consumers:

- `DailyRevenueChart.vue` uses independent Dither strips per currency and for
  order count; it was completed in `b594a5dd` with payment seed data.
- `MonitorTrendChart.vue` uses three Dither strips while preserving zoom,
  localization, gap handling, and the existing metric semantics.
- The live admin `AccountStatsModal.vue` usage trend uses Dither strips. The
  duplicate unmounted `components/account/AccountStatsModal.vue` was reconciled
  and removed in `b594a5dd` before conversion.
- `chart.js` and `vue-chartjs` are removed from `package.json` and the lockfile.

The remaining textual references are migration comments/tests only; no runtime
imports remain. The account modal stays on the `TOUCHED_NOT_CONVERTED` waiver
because its other legacy sections still need the later account-modal pass.

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

| date | commit | what | files converted |
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
| 2026-08-13 | `b594a5dd` | payment revenue chart off Chart.js; duplicate account modal reconciled | 127 |
| 2026-08-13 | `6db5fef1` | remaining Chart.js consumers removed; item 4 complete | 129 |
| 2026-08-15 | `bdba323b` | landed PR #3's upstream reconcile by cherry-pick; sync PR queue emptied | 208 |
| 2026-08-15 | `8bc560dc` | restored the site logo + its sanitiser, dropped by the sidebar rewrite | 208 |
| 2026-08-15 | `3e6888ab` | confirm-dialog helper retargeted at the Button primitive | 208 |

### Open, needs an owner decision — read before the next sync run

1. **Gate 5 is breached locally.** `84a3c4ac` put 19 files under `backend/`: an
   `avatar_seed` user field with full ent regeneration, migration
   `161_add_user_avatar_seed.sql` (a duplicate of the existing 161), and CN→EN
   default legal-doc titles in `setting_public.go`. Not pushed, which is the
   only reason the reconcile routine has not tripped. Rule 1 says revert; the
   uncommitted profile-avatar work depends on the field, so the two decisions
   are one decision.
2. **`inferno-redesign` on the remote is 61 commits behind local.** Every
   reconcile the routine has produced was computed against that stale base,
   which is why all three PRs read as massively conflicting. Pushing fixes the
   routine's view and trips Gate 5 in the same move — see 1.
3. **Local is 124 commits behind `upstream/main`.** Gate 5 only means what it
   claims *after* a rebase; against a stale ref it under-reports (19 files),
   against a fresh one it over-reports (246, of which 227 are just upstream
   moving on). Run it post-rebase or not at all.
4. **june-lint: 845 violations across 273 files**, none from the commits above.
   The denominator grew when the restore commits pulled ~140 half-migrated
   files into scope; a file counts as converted the moment it has a scoped
   `<style>` block. `dead-teal-palette` 476 and `ground-rule-3-two-weights` 271
   are the bulk, concentrated in `UsageTable.vue` / `UsageFilters.vue`.
5. **`AppSidebar.vue:114`** binds `user.avatar_url` to `:src` through a bare
   `.trim()` with no sanitiser — same hole as the site logo, different field.
6. Two gaps PR #3 flagged and left open: `backup.ts` multi-part downloads will
   404 until `BackupView.vue` converts, and `accountUsageRefresh.ts` is ported
   but unwired, so Grok quota cells lack the refresh OpenAI/Codex cells have.
