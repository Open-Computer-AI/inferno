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
| status | **COMPLETE** — 8/8 tasks, final review + fix wave done. HEAD `ee9b3c12`. Awaiting the user's merge decision. |

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

---

# Swarm registry — step 5: authorization_code + PKCE (2026-08-18)

Continues on the SAME branch and worktree as the OAuth AS run above. Step 5 was
deferred out of that plan and is now the blocker: the device flow works and the real
CLI logs in, but **Hermes Desktop cannot authenticate at all** without the Portal half
of its gateway-brokered RFC 8252 flow.

| field | value |
|---|---|
| worktree | `/Users/saksham/OpenComputerV2/inferno-oauth-as` |
| branch | `feat/oauth-authorization-server` (BASE for this run: `49457d7e`) |
| spec | `docs/superpowers/specs/2026-08-18-authorization-code-pkce-design.md` |
| plan | `docs/superpowers/plans/2026-08-18-authorization-code-pkce.md` |
| ledger | `.superpowers/sdd/2026-08-18-authorization-code-pkce/progress.md` |
| status | RUNNING (Task 1 of 6) |

## Recover / resume

```sh
cd /Users/saksham/OpenComputerV2/inferno-oauth-as
cat .superpowers/sdd/2026-08-18-authorization-code-pkce/progress.md   # if it survives
git log --oneline 49457d7e..HEAD                                      # authoritative
```

## Tasks

1. RS256 signing key + kid-dispatched verification (**first: it changes SigningKey.Private's
   type, so the compiler enumerates every caller**)
2. redirect_uri validation at registration and authorize
3. authorization codes — issue and redeem, PKCE-bound and single-use
4. `GET /oauth/authorize` + consent screen + org auto-approve
5. the 60-second refresh reuse grace the Portal contract documents
6. conformance against the real `plugins/dashboard_auth/nous` provider

# Swarm registry — upstream frontend port audit (2026-08-28)

Answering: after the 473-commit reconcile, what has upstream changed in the
frontend that we should bring into `inferno-frontend/`? Specifically whether the
standing "IGNORE components/views/features" rule has been hiding real defects.

| field | value |
|---|---|
| run dir | `/Users/saksham/OpenComputerV2/inferno` (branch `inferno-redesign`) |
| transcripts | `~/.claude/projects/-Users-saksham/3555e339-7e70-40ff-8a71-1c207a51ba41/subagents/` |
| distilled output | `docs/sync-analysis-2026-08-28/` (written when the verifier lands) |
| baseline | vendor point `47b1130cb`; upstream base `baeac1f3d`; upstream tip `e866ff6ec` |
| status | 5 of 6 COMPLETE; adversarial verifier RUNNING; i18n agent resumed after returning no result |

## Agents

| # | model | scope | status |
|---|---|---|---|
| 1 | Opus | API contract (`src/api`, `src/types`) + Go handler JSON shapes | COMPLETE — 3 MUST-FIX |
| 2 | Sonnet | stores / composables / utils / constants / router | COMPLETE — 2 MUST-PORT |
| 3 | Sonnet | i18n (22 locale files) | RESUMED — first return was empty |
| 4 | Opus | components (40 source files) | COMPLETE — 36 of 40 are NOT pure styling |
| 5 | Opus | views (18 source files) | COMPLETE — 16 of 18 are NOT pure styling |
| 6 | Opus | adversarial verifier — refutes 1-5's claims | RUNNING |

## Recover

Cross-session, always works: the transcripts above are the durable record. Each
agent's final message carries its full table. Do NOT tail them into a main-thread
context — they are 400-800KB each; extract the last assistant message only.

## Headline, pending verification

The recurring finding is not "we are behind upstream". It is that **our own Go
backend already ships these capabilities and our June frontend cannot reach
them**: `restrict_public_groups` (migration 231), composite live/dispatch,
`fast_multiplier`/`time_pricing`, Kimi/Zhipu/DeepSeek routing, concurrency 0 =
unlimited. The backend half arrives automatically by merge; the frontend half
arrives by manual port under a rule that skips components and views. So the two
halves of one upstream feature take different paths and one silently does not
arrive. The gap is between our own two halves, not against upstream.

Zero security regressions found — both classifiers grepped for them specifically.

Both classifier agents independently proposed the same replacement rule: ignore
`<template>` and `<style>` diffs, always review `<script setup>` and `.ts` hunks.
On this window that catches all 15 bug fixes for the cost of 4 extra files.

# Swarm registry — commit-by-commit catch-up, set A (2026-08-29)

Replaces file-diffing with commit-walking. Four rounds of diffing each found a
category the last missed, because a diff cannot separate "we redesigned this"
from "we never had this". A commit can: it carries the intent.

| field | value |
|---|---|
| manifest | `docs/superpowers/analysis/COMMIT-MANIFEST.md` — one row per commit, the completion record |
| scope | set A: 21 commits with content, 2026-08-07 → 08-15, missed at vendor time |
| remaining | set B: 78 non-merge commits since vendoring |
| transcripts | `~/.claude/projects/-Users-saksham/3555e339-*/subagents/` |
| status | 4 agents RUNNING (read-only analysis); nothing applied yet |

## Agents

| batch | commits | period |
|---|---|---|
| 1 | 9b54b46b0, 4999231d6, 563a72ca7, bbc8b6e90 | 08-07 → 08-09 |
| 2 | 33351c7bc, 5350b3d98, 9096492b5, b689e5b40, 0d7b6ae64, 943f09d35 | 08-10 → 08-11 |
| 3 | 670b03f7e, c0ab3a00e, 0ae151a23, 363cc4994, 69648476d, a04ce4901 | 08-12 → 08-13 |
| 4 | b830bc14d, e215c98c2, f3d949107, cb7b03795, fce41e318 | 08-13 → 08-15 |

Each agent reads the full commit (message + diff, translating Chinese), maps
every touched file to our equivalent, judges whether the SPECIFIC change is
present in our rewritten copy, and classifies it LOGIC / UI / BOTH. UI changes
cannot be pasted — they get described so they can be rebuilt in June.

## Recover

Agents are read-only and produce plans, not edits. If this session dies before
they are applied, their final messages are in the transcripts above; extract the
last assistant message only (the files are large). The manifest survives in git
and shows exactly which commits still say TODO.

# Swarm registry — set B catch-up (2026-08-29)

Set A is closed (22/22, validated end to end). Set B is the 81 non-merge
upstream commits touching `frontend/` since the vendor point, 08-16 to 08-29.

| field | value |
|---|---|
| manifest | `docs/superpowers/analysis/COMMIT-MANIFEST.md`, Set B table |
| baselines | vue-tsc 0 · vitest 1665/1665 across 234 files · june-lint 984 across **288** converted · divergence all-declared |
| status | 5 agents RUNNING on tier 1 (27 commits); tier 2 (54) not yet started |

## Triage

Tier 1 (27) touches api/types/stores/utils/composables -- contract and logic,
where a wrong value renders confidently rather than failing. Tier 2 (54) touches
only components/views/i18n. Tier 2 is NOT skippable: in set A, 15 of 40 changed
component files carried bug fixes, and the standing "ignore components" rule is
what let them accumulate.

## Agent batches (tier 1, chronological, sequences kept whole)

| batch | commits | note |
|---|---|---|
| 1 | 901a0439f 8d82bb069 9f24a5530 a20e1c00c c9effc456 2c250bfd7 | CN providers, monitor quota mode |
| 2 | 26be82cc8 1f2a87adb 39485f2e2 6c3edc095 e62ec2c42 3445485eb | contains a FEATURE AND ITS REVERT (429 cooldown) |
| 3 | 22e1b8144 e39fce270 77e0409f7 40ea3aeba 377d1230f 83d4eb6a4 b07d85c49 | two sequences; plugin iframe security must ship WITH the feature |
| 4 | 6f972145b 3f1581b2d 11ada80d5 5705f4a4a 195b21970 2abce6503 | contains a PAIR pulling opposite ways (expose then hide reasoning effort) |
| 5 | b56c61ecc b5827cfd5 | restrict_public_groups -- shipping the toggle may MAKE the refuted save-filter bug reachable; and DeepSeek peak/off-peak billing |

## The guardrail that matters

june-lint's CONVERTED FILE COUNT (288) must not DROP. Violations rising is
expected and fine -- it means Tailwind added to files that are not June yet,
which the standing rule asks for. A falling converted count means a copy
overwrote converted work, which is the failure this whole process exists to
prevent.

## Recover

Agents are read-only and produce plans, not edits. Their final messages are in
`~/.claude/projects/-Users-saksham/3555e339-*/subagents/`; extract the last
assistant message only, the transcripts are large. The manifest survives in git
and shows which rows still say TODO.

---

## Swarm: full file-level fidelity sweep (2026-08-30)

Not commit-by-commit. This one walks the *file* surface: every file that exists
in BOTH `frontend/src/` (pristine upstream) and `inferno-frontend/src/` (ours)
AND differs -- 285 files. Each is classified STYLE / OURS-AHEAD / SUSPECT.

Census that defined the surface:

```
identical to upstream : 467
differ                : 285   <- the sweep target
only in upstream      :   9   (all checked, benign: relocated or unreferenced)
only in ours          :  89
```

| Batch | File list | Files | Agent purpose |
|---|---|---|---|
| 0 | `/tmp/fbatch0.txt` | 47 | classify diffs, flag dropped upstream logic |
| 1 | `/tmp/fbatch1.txt` | 48 | same |
| 2 | `/tmp/fbatch2.txt` | 48 | same |
| 3 | `/tmp/fbatch3.txt` | 48 | same |
| 4 | `/tmp/fbatch4.txt` | 47 | same |
| 5 | `/tmp/fbatch5.txt` | 47 | same |

Files were ranked by diff size and dealt round-robin, so every batch carries a
share of the large files (`SettingsView.vue` 17644 diff lines,
`OpsDashboardHeader.vue` 1773, `AppSidebar.vue` 1756, `AccountUsageCell.vue` 1447).

**Agents are read-only.** They report; the main thread fixes and re-runs gates.

### Recover

The `/tmp/fbatch*.txt` lists are the durable input -- regenerate a batch by
re-dispatching the same prompt against its list. If `/tmp` was cleared, rebuild
the census with:

```bash
cd inferno && for f in $(cd inferno-frontend/src && find . -type f | sed 's|^\./||'); do
  [ -f "frontend/src/$f" ] || continue
  cmp -s "frontend/src/$f" "inferno-frontend/src/$f" || echo "$f"
done
```

Distilled findings land in `docs/superpowers/analysis/FILE-SWEEP-FINDINGS.md`.

---

## 2026-09-04 — landing-page removal + inference deploy skill (2 agents, parallel)

Two independent agents, no shared files. Spawned from the main thread; results
are reviewed and committed by the main thread, never self-merged.

| # | Purpose | Owns | Status | Distilled output |
|---|---------|------|--------|------------------|
| 1 | Remove the public landing page so `/` opens on login, mirroring `oc-router` | `inferno-frontend/src/router/`, `views/KeyUsageView.vue`, `views/public/LegalDocumentView.vue` | see below | working tree diff (uncommitted) |
| 2 | Capture the `inference.tryopencomputer.com` deploy + redeploy runbook as a skill | `skills/inferno-inference-deploy/` (new) | see below | `skills/inferno-inference-deploy/SKILL.md` + `scripts/` |

Agent 2 is explicitly **read-only against production** — it inspects
`i-0066a065c11a7b94d` and writes files locally; it deploys nothing and never
touches `i-0e4fe42fc3fadf277` (oc-router).

### Recover

Neither agent commits. If the session dies before review, the work survives as:

- agent 1 — uncommitted changes in `inferno-frontend/src`; `git diff` shows the
  whole of it. Re-derive the scope with
  `grep -rn "/home" inferno-frontend/src` (8 sites across 4 files) and the
  reference implementation at
  `../oc-router/frontend/src/router/index.ts` (same change, already made).
- agent 2 — files under `skills/inferno-inference-deploy/`. Source material is
  `deploy/inference/README.md` and `deploy/inference/bootstrap.sh`.

Live facts both depend on, so they need not be rediscovered:
AWS `133277694446` / us-east-1, creds only on `oc-internal`
(`ssh root@oc-internal "su - architsakri -c '<aws ...>'"`);
`i-0066a065c11a7b94d` = oc-inference (3.82.43.139),
`i-0e4fe42fc3fadf277` = oc-router (35.175.193.193, off-limits);
ECR repo `oc-platform/inferno`, linux/amd64; hermes profiles on oc-internal at
`/Users/architsakri/.hermes/profiles/<name>/skills/`.
