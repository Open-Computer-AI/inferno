# Swarm registry — Inferno June rewrite

Durable index of parallel agent runs. Purpose: if this session ends, a fresh one
resumes from here instead of re-running the swarm.

Distilled results land in `INFERNO-BUILD.md`. The authoritative record of what is
actually converted is always:

```sh
cd /Users/saksham/OpenComputerV2/inferno
diff -rq frontend inferno-frontend -x node_modules -x dist
```

That reads the filesystem, not a log, so it cannot go stale or lie.

## Contract every worker follows

`inferno-frontend/CONVENTIONS.md` — tokens, the ten ground rules, house style,
file-ownership rules, and the definition of done.

## Wave 1 — part 02 Controls, sections 05-13

Spawned 2026-08-09. Model: Sonnet (execution tier). Orchestrator: Opus, validating.

| Agent | Part 02 section | Owns (sole writer) | Status |
|---|---|---|---|
| select | 05 Select | `common/Select.vue` | **done, validated** |
| binary | 06-08 Toggle, checkbox, radio | `common/{Toggle,Checkbox,Radio}.vue` | **done, validated** |
| segmented | 09 Segmented, tabs | `common/Segmented.vue` | **done, validated** |
| search | 10 Search input | `common/SearchInput.vue` | **done, height corrected by orchestrator** |
| amount | 12 Amount input | `payment/AmountInput.vue` | **done, relocated by orchestrator** |
| tags | 13 Model tag input | `admin/channel/ModelTagInput.vue` | **done, validated** |

**Wave 1 gate (orchestrator-run, not self-reported):** lint clean across 15
converted files · `vue-tsc` 0 errors · full suite 220/220 files, 1518/1518 tests
· 0 upstream files modified.

### What the orchestrator had to correct

Both were bad instructions in the assignment, not worker error:

1. **AmountInput** was assigned `common/AmountInput.vue`; the app imports
   `payment/AmountInput.vue`. The worker followed the path literally and
   produced orphaned dead code that linted and typechecked. Relocated after
   verifying prop parity. **Derive every target path by grepping for the import,
   never from memory.**
2. **SearchInput** was told to reuse Input.vue's 36px height "for consistency".
   The prototype specifies 32px and is right: a form field sits in a column of
   fields, a search box sits in a toolbar beside 32px buttons. Reverted.

## Wave 2 — spawned after the product owner's two rulings

Rulings received: **keep the platform dot** (it is series-ramp identity colour,
a deliberate exception to ground rule 5, not chrome tint), and **the
orchestrator owns `src/i18n/`** — workers call `t('key')` and report the key and
English value; the orchestrator writes the locale files in one pass so there are
no write collisions. `CONVENTIONS.md` rule 6 now encodes this.

Every path below was derived by grepping for the real import, with the consumer
count recorded — the wave 1 lesson.

| Agent | Scope | Owns (sole writer) | Consumers | Status |
|---|---|---|---|---|
| dot-fix | Restore the platform dot; make `platform` live again | `admin/channel/ModelTagInput.vue` | 2 | running |
| daterange | 02 §11 Date range | `common/DateRangePicker.vue` | 5 | running |
| selectors | 02 §14 Group selector, §15 Proxy selector | `common/GroupSelector.vue`, `common/ProxySelector.vue` | 4 + 5 | running |
| modals | Part 05 primitives | `common/BaseDialog.vue`, `common/ConfirmDialog.vue` | **75 + 30** | running |

`BaseDialog` at 75 consumers is the highest-blast-radius rewrite in the project.
That worker was briefed explicitly: preserve props, emits, scoped-slot payloads
and `defineExpose` exactly, grep before renaming any CSS class, and report rather
than make an API change if the prototype seems to demand one.

Workers were also told **not** to run the full vitest suite during the wave —
that produced a false 17-failure report in wave 1.

Still queued: part 03 Data display (9 sections), part 07 App shell v2.

### Worker judgment worth keeping

- **Select** preserved the literal classes `select-trigger` and
  `select-dropdown-portal` because `composables/useOnboardingTour.ts` does
  `querySelector('.select-trigger')` and a spec asserts on the portal class.
  Neither appears in any props table. Preserving only the *documented* API would
  have silently broken the product tour.
- **ModelTagInput** overrode its assigned path, found the real file, and said so.
- **Segmented** hit the one `linear-gradient` in all 18 prototypes (the tab
  overflow fade, Controls line 531), which ground rule 7 bans. It substituted an
  inset `box-shadow` with the same affordance and documented the deviation.
- **A "17 failing tests" report from Select was a race, not a regression** — it
  ran the suite while another worker was mid-write on `Toggle.vue`. Re-run:
  23/23 pass. Workers must not run the full suite during a parallel wave; only
  the orchestrator's post-wave gate is meaningful.

**Deferred to wave 2 on purpose:** 11 Date range, 14 Group selector, 15 Proxy
selector. They compose Select, so they wait for it rather than racing it.

**Files no worker may touch** (orchestrator owns them, and they are where a
parallel run would otherwise corrupt itself): `SpecimenView.vue`,
`INFERNO-BUILD.md`, `SWARM-REGISTRY.md`, `router/index.ts`, `main.ts`,
`tailwind.config.js`, `scripts/june-lint.mjs`, `src/design-system/**`,
and `../frontend/**`.

## Validation gate

A worker's self-report is not acceptance. Every component is independently
checked by the orchestrator before its row is marked done:

1. `node scripts/june-lint.mjs` clean
2. `npx vue-tsc --noEmit` zero errors
3. Mounted in `SpecimenView.vue` and its computed styles asserted in a real
   browser against the measurements the prototype prints beside it

## Recovery

- Results arrive as task notifications in the orchestrating session.
- Nothing is committed, so `git status` plus the `diff -rq` above fully describe
  the state at any moment.
- If a worker dies, its section simply stays `todo` in `INFERNO-BUILD.md`; no
  partial state is shared between workers because no two workers share a file.

---

# Swarm registry — OC Portal: OAuth authorization server (2026-08-17)

Separate effort from the June rewrite above. Subagent-driven execution of the
OAuth AS plan, running in its OWN worktree so it never touches the redesign branch.

| field | value |
|---|---|
| worktree | `/Users/saksham/OpenComputerV2/inferno-oauth-as` |
| branch | `feat/oauth-authorization-server` (cut from `inferno-redesign` @ `1c91c378`) |
| spec | `docs/superpowers/specs/2026-08-17-inferno-oauth-authorization-server-design.md` |
| plan | `docs/superpowers/plans/2026-08-17-inferno-oauth-authorization-server.md` |
| ledger | `.superpowers/sdd/2026-08-17-inferno-oauth-authorization-server/progress.md` |
| purpose | make Inferno the OC Portal — replace Nous Portal for hermes-agent + Hermes Desktop |
| status | RUNNING (started Task 1 of 8) |

## Recover / resume

The ledger is git-IGNORED scratch — `git clean -fdx` destroys it. Git history is
the durable record. To resume in a fresh session:

```sh
cd /Users/saksham/OpenComputerV2/inferno-oauth-as
cat .superpowers/sdd/2026-08-17-inferno-oauth-authorization-server/progress.md  # if it survives
git log --oneline 1c91c378..HEAD                                                # authoritative
```

Each task commits separately with a `feat(oauth):` subject. Tasks with a
`Task <N>: complete` ledger line are DONE — resume at the first task without one.
If the ledger is gone, `git log` tells you which tasks landed; re-read the plan's
task list and resume after the last committed one. Per-task briefs and reports live
beside the ledger (`task-N-brief.md`, `task-N-report.md`).

## Tasks

1. org tenancy + personal org on signup
2. ES256 signing key + JWKS endpoint
3. `oauth_client` registry + self-hosted client registration
4. RFC 8628 device authorization request
5. token endpoint (device_code + refresh_token grants)
6. scope enforcement middleware + `/api/oauth/account`
7. device approval screen (`inferno-frontend`)
8. end-to-end conformance against the real hermes CLI
