# Inferno frontend rewrite — build log

The June redesign of `inferno-frontend/`, from `design_handoff_inferno_v2/`.
This file is the resume point. If it goes stale, `diff -rq frontend inferno-frontend`
reconstructs the truth.

## How this repo is arranged

| | |
|---|---|
| `frontend/` | Pristine upstream. **Never edited.** Upstream syncs land here conflict-free, forever, because nothing here is ever touched. It is the live reference: when a sync brings something new, diff it and decide what to rewrite. |
| `inferno-frontend/` | The product. Redesigned in place — same URLs, same router paths, components swapped as they are converted. |

**The conversion ledger is a command, not a document:**

```sh
diff -rq frontend inferno-frontend -x node_modules -x dist    # differs = converted
```

No `/v2` route tree and no feature flag. The handoff specified both to protect a
shipping v1; there isn't one (production is `oc-router`, untouched), so they were
cost with no benefit — `/v2/*` would have become permanent customer-facing URLs.

## Status

### Phase 0 — foundation — DONE

- **Tokens + fonts.** Nine June token sheets copied byte-identical to
  `src/design-system/tokens/`, plus `components/june-components.css`. Imported
  globally via `src/design-system/june.css`. The design bundle's internal
  `assets/fonts/` layout is preserved so `fonts.css` needs no edit — those files
  are spec and get re-synced; a local edit turns every future sync into a merge.
- **Icons.** Hugeicons stroke rounded **self-hosted** (`hgi-stroke-rounded.woff2`,
  860KB, 5,503 glyphs) with a generated `hugeicons.css`. Not the CDN: it sends no
  CORS header, so rasterized captures render tofu, and self-hosting also makes the
  console work offline.
- **Inferno-local tokens** in `src/design-system/inferno.css`: the `--s2a-*`
  family (control heights 28/32/36/40, attention colour, cell-series banding) and
  the shared `s2a-spin` keyframe with its reduced-motion guard.
- **Tailwind.** `darkMode` accepts both `.dark` and `[data-theme="dark"]`;
  `main.ts` mirrors one onto the other via MutationObserver. Deleted the whole
  `backgroundImage` block, `shadow-glow`/`glow-lg`/`inner-glow`, and the
  `glow`/`shimmer`/`pulse-slow` animations and keyframes.
- **`scripts/june-lint.mjs`** — mechanical checks for ground rules 1, 2, 3, 4, 6,
  7, 8, 10. Scopes itself to converted files by diffing against `frontend/`, so
  the scope widens automatically and there is no list to forget to update.

Verified live: `@property --brand` and `@property --fs-md` survive minification
(so theme changes interpolate rather than snap), all five `data-brand` presets
present, body resolves to June's `--background` at 13px in ABC Diatype, icons
resolve from the self-hosted face.

### Phase 1 — primitives — IN PROGRESS

**Part 02 Controls** — 12 of 15 sections.

| # | Section | Status | File |
|---|---|---|---|
| 01 | Button | done | `components/common/Button.vue` (new) |
| 02 | Icon button | done | `components/common/IconButton.vue` (new) |
| 03 | Input | done | `components/common/Input.vue` (rewritten, API unchanged) |
| 04 | Text area | done | `components/common/TextArea.vue` (rewritten, `variant` added) |
| 05 | Select | done | `components/common/Select.vue` (rewritten, API unchanged) |
| 06-08 | Toggle, checkbox, radio | done | `Toggle.vue` (rewritten), `Checkbox.vue`, `Radio.vue` (new) |
| 09 | Segmented, tabs | done | `components/common/Segmented.vue` (new) |
| 10 | Search input | done | `components/common/SearchInput.vue` (rewritten, `resultText?` added) |
| 12 | Amount input | done | `components/payment/AmountInput.vue` (rewritten, API unchanged) |
| 13 | Model tag input | done | `components/admin/channel/ModelTagInput.vue` (rewritten, API unchanged) |

Gate after wave 1: lint clean across 15 converted files, `vue-tsc` **0 errors**,
full suite **220/220 files, 1518/1518 tests**. No regressions.

Remaining in part 02, deferred because they compose Select:
11 Date range (`common/DateRangePicker.vue`), 14 Group selector
(`common/GroupSelector.vue`), 15 Proxy selector.

### Carried debt from wave 1 — read before wave 2

1. **`Checkbox.vue` and `Radio.vue` have zero consumers.** Roughly 40 files still
   use raw `<input type="checkbox">` / `"radio"`. The components are correct and
   tested; nothing imports them yet. Migrating those call sites is its own task
   and is not part of any prototype section. Until it happens these two are
   future code, not converted code.
2. **`ModelTagInput.platform` is inert** — still in the API, no longer drives
   anything, and two call sites pass it. Resolution depends on the platform-dot
   decision below.
3. **Hardcoded English strings**, because workers may not touch shared locale
   files: `"N models"` (ModelTagInput), `"Or a custom amount"` (AmountInput).
   Every future component will add more. Fix: the orchestrator owns `i18n/` and
   collects keys from each worker's report.

### VERIFICATION STATUS — pass 1 done, partial

All 17 components now render on `/dev/specimen` (17 sections). Measured in a
real browser, not self-reported:

**Passing (12 declared probes + 4 internals queried by class):**
Button 28/32/36/40 · IconButton 28/32/36 · Select trigger 36 · Toggle 34x20 ·
Checkbox 15 · Segmented tray 28 · Segmented tabs track 32 · DateRangePicker
trigger 32 · SearchInput field 32 (confirms the correction from 36) ·
ModelTagInput tag 24 and platform dot 5x5 (confirms the dot was restored).

Visual pass at 1440x900: one clay accent throughout, no gradient, glow, glass or
teal anywhere, sentence case everywhere, dense chrome. Reads as one system.

**Still unmeasured — nested or behind interaction, needs a second pass:**

| Component | Numbers not yet checked | Why |
|---|---|---|
| Select | option row 30, list cap 184, group header 22 | inside a teleported, `v-if`-gated popover |
| DateRangePicker | preset 26, calendar day 24, footer buttons 28 | same |
| BaseDialog | header 44, close 28, footer 52, widths 420/520/720/960 | dialog must be opened; widths ARE reachable via the component's own `.dlg-panel[data-width]` |
| GroupSelector / ProxySelector | rows 38, direct row 34, radio 15, test button 26 | `v-for` rows inside a monolithic root |
| AmountInput | chip 32, custom field 40 | nested descendants |
| Toggle / Segmented | knob 16, thumb 24 | nested spans |

**How to finish it:** drive the UI with Playwright — click each trigger to open
the popover or dialog, then query the internal class directly
(`.dlg-panel[data-width="wide"]`, the option rows, etc.) and compare against the
numbers above. No further `data-expect-*` attributes are needed; those internals
are reachable by class once visible.

Two findings from building the harness, worth keeping:
- `data-expect-*` cannot be bound onto `Toggle` or `Checkbox` — both set
  `inheritAttrs: false` and forward `$attrs` to their hidden `<input>`, so a
  bound attribute measures 1x1px. They are wrapped in a probe div instead.
- `Radio`'s root computes to ~17px rather than 15 because its grid uses
  `align-items: start` with a 2px top margin on the dot box and no matching
  bottom margin. Worth confirming against the prototype whether that is intended
  before treating it as correct.

| Verified in a browser | Compiles + lints + tests only |
|---|---|
| Button, IconButton, Input, TextArea | Select, Toggle, Checkbox, Radio, Segmented, SearchInput, DateRangePicker, GroupSelector, ProxySelector, AmountInput, ModelTagInput, BaseDialog, ConfirmDialog |

What passing lint, `vue-tsc` and 1518 tests actually proves: the code compiles,
obeys the mechanical ground rules, and does not regress existing behaviour. It
does **not** prove any of it matches its prototype. For the 13 on the right, the
only evidence of fidelity is the building agent's own report of the numbers it
implemented.

`SWARM-REGISTRY.md` defines the gate as three steps — lint, typecheck, and
"mounted in SpecimenView.vue with computed styles asserted in a real browser
against the measurements the prototype prints". Steps 1 and 2 ran on every wave.
Step 3 ran on nothing after the first four components. The checks that did run
were real command output, which is precisely what made the gap easy to miss.

**The verification pass, in order:**

1. Every component gets a section in `views/dev/SpecimenView.vue`, showing its
   states side by side, with `data-expect-height` / `data-expect-width` on the
   elements whose measurements the prototype prints.
2. Screenshot `/dev/specimen` against the matching prototype section at
   1440x900, light and dark. Serve prototypes with
   `python3 -m http.server 4599 --directory <handoff>/prototypes`.
3. Assert in-browser: `document.querySelectorAll('[data-expect-height]')`,
   compare `getBoundingClientRect().height` to the attribute. Any mismatch is a
   defect, not a rounding artefact — these are token-driven fixed heights.
4. Fix, re-assert, then continue to part 03.

Do not build part 03 or part 07 on top of 13 unverified primitives.

### PENDING i18n KEYS — add these before using GroupSelector / ProxySelector

The orchestrator owns `src/i18n/`; workers report keys instead of writing them
(CONVENTIONS rule 6). These 8 were reported by the selectors worker and are
**not yet added**. Until they are, both components render raw key paths on
screen. Add to `en` and `zh` both.

| Key | English |
|---|---|
| `common.searchGroupsPlaceholder` | Search groups |
| `common.groupSelectorMixedSchedulingHint` | Gemini and Anthropic groups are hidden because mixed scheduling is off. Turn it on to see them. |
| `common.groupSelectorPlatformHint` | Groups for other platforms are hidden. |
| `common.groupCapacityEmpty` | No accounts yet |
| `common.groupCapacityRateLimited` | {limited} of {total} rate limited |
| `common.groupCapacityTotal` | {total} accounts |
| `admin.proxies.testAll` | Test all |
| `admin.proxies.directOption` | Direct, no proxy |

Note on the last two: the worker deliberately did **not** reuse
`admin.proxies.batchTest` ("Test All Proxies") because it is Title Case and
violates ground rule 1. The old key is still referenced elsewhere and is part of
the wider i18n casing pass, not this component's problem.

Already added by the orchestrator: `admin.channels.form.modelTagCount`
(`{count} model(s)` / `{count} 个模型`), plus removal of four emoji and one em
dash that entered lint scope with it.

### OPEN REGRESSION — fix this first in the next session

`src/components/common/__tests__/DateRangePicker.spec.ts` →
"emits range updates with last24Hours preset when applied" **fails**.
Everything else is green: 219/220 files, 1517/1518 tests, `vue-tsc` 0 errors,
lint clean across 21 files.

**Root cause, confirmed — not a flaky test.** The old DateRangePicker rendered
its popover inline (zero `Teleport` in the pristine copy). The rewrite teleports
it to `<body>`, following the pattern `Select.vue` already established. The test
does `wrapper.findAll('.date-picker-preset')`, which only searches the mounted
component's own tree, so teleported nodes are invisible to it and
`presetButton` is `undefined` at the `toBeDefined()` assertion on line 76.

The three literal classes the worker was asked to preserve
(`date-picker-trigger`, `date-picker-preset`, `date-picker-apply`) **are** all
present. They are simply no longer reachable through `wrapper`.

**Decide, do not just make it pass:**

- If teleport is right (it probably is — a popover in a filter bar gets clipped
  by `overflow` ancestors, which is exactly why Select teleports), then the test
  should query `document.body`, the way `Select.spec.ts` already does against
  `.select-dropdown-portal`. Update the test.
- If teleport is not wanted here, remove it from the component instead.

Do **not** weaken the assertion to make it green. The test is asserting real
behaviour; only its DOM lookup is now wrong.

Related, same worker, worth reviewing at the same time: it also replaced
implicit-commit-on-close with an explicit Apply/Cancel draft model (outside
click, Escape and re-clicking the trigger now all discard). No prop, emit or
slot changed, but the *behaviour* did, and the prototype does specify a Cancel
button. Confirm that is intended before migrating the 5 consumers.

### Undecided, blocking clean completion

- **The platform dot.** ModelTagInput's prototype specifies a per-tag dot
  coloured by platform; ground rule 5 says colour encodes state, never category.
  A genuine contradiction in the handoff. Currently dropped, which is what makes
  `platform` inert.
- **Checkbox/radio as components vs documented raw inputs** — the prototype's own
  `#calls` (line 862) leaves this open. Built as components, which is the
  reversible choice: new files only, no call site touched.

### Process rules learned the hard way in wave 1

- Derive every target path by grepping for the actual import. Assigning
  `common/AmountInput.vue` from memory produced orphaned dead code that linted
  and typechecked clean while the app kept using the old component.
- The prototype's printed measurement wins over cross-component "consistency".
  SearchInput is 32px (toolbar) not 36px (form field), and they sit in different
  contexts on purpose.
- Workers must not run the full test suite during a parallel wave. One reported
  17 failures caused by another worker being mid-write. Only the orchestrator's
  post-wave gate means anything.
| 06-08 | Toggle, checkbox, radio | todo | |
| 09 | Segmented, tabs | todo | |
| 10 | Search input | todo | |
| 11 | Date range | todo | replaces `common/DateRangePicker.vue` |
| 12 | Amount input | todo | |
| 13 | Model tag input | todo | |
| 14 | Group selector | todo | replaces `common/GroupSelector.vue` |
| 15 | Proxy selector | todo | |

Then part 03 Data display, part 05 Modals, part 07 App shell v2.

## Verification loop, per section

1. Read the part's `#calls` section first. Parts 04, 05, 06 have one; 02 and 03 do not.
2. Read the per-section Migration note — it names the exact Vue files that section replaces.
3. Build the component.
4. Add it to `views/dev/SpecimenView.vue` (route `/dev/specimen`, dev-only —
   `import.meta.env.DEV` is statically replaced so it is dropped from production builds).
5. **Assert the printed measurements.** Prototypes print numbers next to components
   ("28 / 32 / 36 / 40") and README says that number is the spec. Specimen elements
   carry `data-expect-height`; assert against `getBoundingClientRect` in the browser.
6. `node scripts/june-lint.mjs` and `npx vue-tsc --noEmit`.
7. Update this file.

Verified so far, by asserting computed styles in the browser:

- **01 Button** — heights 28/32/36/40 exact, radius `--r-md` (8px), solid at 600
  and secondary at 400 (only two weights exist), transition property resolves to
  `background` alone, never `border-color`.
- **02 Icon button** — 28/32/36, square, radius `--r-sm` (6px), not the `--r-xs`
  badge tier.
- **03 Input** — height exactly 36px (was `py-2.5`), radius 8px, prefix padding
  32px, error border distinct, and the two states the migration note calls for:
  `readonly` is now visually distinct from `disabled`, and `disabled` keeps its
  `--card` fill instead of repainting grey.
- **04 Text area** — Berkeley Mono at 21px leading (12 × 1.75) in the `mono`
  variant, `resize: vertical` only.

### Note on the lint

Three bugs were found in `june-lint.mjs` by running it, all of the same kind —
it was reporting things that were correct:

1. It scanned its own code comments, so a comment saying "delete every
   shadow-glow" tripped the no-glow rule.
2. It flagged icon `font-size` in px, which 01-TOKENS explicitly *requires*
   because glyphs must not scale with the text-size preference.
3. `june-lint-disable` markers were invisible, because comments were stripped
   before the marker was looked for.

All three are fixed. Worth knowing because the failure mode of a lint is not
false negatives, it is false positives — a lint that reports correct code gets
switched off, and then it catches nothing.

## Decisions taken, with reasons

- **`--s2a-` prefix kept**, not renamed to `--inferno-`. It is the prototypes'
  own naming across all 18 documents; renaming has to be one coordinated pass,
  not a drift that starts mid-build.
- **Button heights are Inferno-local.** June's `--control-*` is 22/26/28/32/36;
  part 02 needs 28/32/36/40. `--s2a-h-*` aliases the June token where they
  coincide so a June control and an Inferno button on one row still line up, and
  only the 40px sign-in size is genuinely new.
- **Icon buttons require `label`.** The prototype says an icon button always
  carries a tooltip; making it a required prop turns that into a type error
  rather than an accessibility bug found later.
- **`active:scale-[0.98]` is deleted, not ported.** June's press feedback is the
  solid darkening by mixing 8% foreground into `--primary`.

## Open calls raised so far

- **`hgi-gauge-01` does not exist** in the live font. All 18 prototypes reference
  82 unique glyphs; 81 exist. Used once, in part 18's `foldFrames` — the variant
  the design did not recommend. Substitute `hgi-dashboard-speed-01`.
- **`TODO(open-call-4)`** in `hugeicons.css` — self-hosting is a licensing
  question. Currently shipping the free stroke-rounded tier.
- **README says six woff2, the bundle ships five** (Diatype Regular + Medium,
  Berkeley Regular + Oblique, Martina Light). Possibly a missing Diatype italic.

## Known work not yet scoped

- **Icon subsetting.** 860KB woff2 + 344KB CSS for 82 glyphs in use. Wants a
  build step driven by a manifest, not a one-off trim — a subset built from
  today's usage fails silently the first time someone adds an icon.
- **The i18n pass.** Sentence case and dash removal across 18,239 lines of locale
  files. Ground rules 1 and 2 make it non-optional and it touches every string.
- **Charts (part 09).** Chart.js draws to canvas and cannot read CSS custom
  properties, so every token has to be resolved to a literal in JS and
  re-resolved on theme change. The one part where the token architecture does not
  just work.

## Running it

```sh
open -a Docker
docker compose -f deploy/docker-compose.local.yml up -d   # API on :8080
cd inferno-frontend && pnpm install && pnpm dev            # :3000, proxies to :8080
```

Specimen board: `http://localhost:3000/dev/specimen`
Prototypes: `python3 -m http.server 4599 --directory <handoff>/prototypes`

## Automation — designed, blocked on two one-time auth steps

Both are ready and both fail on the same class of thing: this machine's GitHub
authorisation.

**1. `.github/workflows/upstream-sync.yml`** — committed locally (a2218952), NOT
pushed. `git push` is rejected: the gh OAuth token lacks the `workflow` scope.
Fix, once:

    gh auth refresh -h github.com -s workflow
    git push origin inferno-redesign

Every 2h: fetch, rebase onto upstream/main, run the gate (june-lint, vue-tsc,
vitest) on the rebased tree, publish `sync/upstream` only if green, open one
tracking issue if not. Mechanical only — it answers "did it break".

**2. Daily reconciliation routine** — a scheduled Anthropic cloud agent, config
written, creation refused with:
"Connect your GitHub account before saving a routine that uses a GitHub
repository." Fix, once, at https://claude.ai/code/onboarding?magic=github-app-setup

Intended shape: cron `30 3 * * *` (09:00 Asia/Calcutta), model claude-sonnet-5,
source `github.com/Open-Computer-AI/inferno`. It answers "does it matter":
diff `frontend/src/{api,stores,composables,utils,types}` since the last reviewed
SHA, decide what affects converted components, port only that, run the gate,
open a PR against `inferno-redesign` — never push to it. It records what it skipped
and why, and advances the reviewed SHA, so "what have I not looked at" stays
answerable.

Deliberately excluded from its diff: `frontend/src/{components,views,features}`.
Those are being replaced by the June parts, so upstream churn there (~340
commits/month of the ~460 total) is noise.

**The split, and why:** CI does the deterministic rebase because it is free,
always on, and needs no judgement. The agent does reconciliation because
deciding whether an upstream API change affects `dashboardModel.ts` is a reading
task CI cannot do. The agent proposes and never merges: the gate proves code
compiles, not that a ported change is semantically right, and an agent that
auto-merges its own judgement recreates exactly the unreviewed drift that put
oc-router 1,825 commits behind.

## Upstream reconciliation log

### 2026-08-09 — first sync, run manually to derive the runbook

13 commits behind (38 backend files, 12 frontend, 1 .github, no migrations).

**Rebase: clean.** All 7 of our commits replayed with zero conflicts, which is
the thin-fork invariant paying off. Verified after: 0 files modified under
backend/, frontend/, deploy/ or docs/. The mirror now matches upstream/main
exactly, so `frontend/` is current again.

**Gate: `vue-tsc` 0 errors, 1518/1518 tests.** But `june-lint` jumped from clean
to **578 violations across 33 files**, and the cause is a flaw in the lint, not a
regression in our code.

#### Finding 1 — the lint's scope heuristic breaks after every sync

`june-lint.mjs` defines "converted" as "differs from ../frontend". That is true
before a sync and false after one: upstream changed 12 files in frontend/, so
our untouched copies of those files now differ from the mirror simply by being
older. The lint reads that as "converted" and holds them to June's rules,
surfacing upstream's own pre-existing violations (284 of the 578 are
`font-medium`/`font-bold` in stock upstream Vue).

**Fix:** ask git whether *we* changed a file, not whether it differs from the
mirror:

    git log --oneline <fork-point>..HEAD -- inferno-frontend/<path>

Non-empty means we touched it. That stays correct across any number of syncs.
Until this is fixed, `june-lint` is unreliable immediately after a sync.

#### Finding 2 — the same signal IS the reconciliation work list

The files the lint dragged into scope are exactly the files whose
inferno-frontend/ copies are now stale. Upstream changed these 12:

    src/api/admin/settings.ts        <- API contract, matters most
    src/types/index.ts               <- API contract
    src/i18n/locales/{en,zh}/admin/settings.ts
    src/i18n/locales/{en,zh}/common.ts
    src/views/admin/SettingsView.vue
    src/views/auth/{EmailVerify,Register}View.vue
    plus 3 spec files

Under the port policy, `src/api/admin/settings.ts` and `src/types/index.ts` need
a decision; the views do not (they are being replaced by the June parts). The
two `common.ts` locale files overlap with keys we added, so they need a manual
merge rather than a copy.

**Not yet ported.** Recorded rather than rushed.

**Last reviewed upstream SHA:** see `git rev-parse upstream/main` at the time of
the next run; this entry advanced the mirror but did not complete the port.

#### Runbook derived from this run

1. `git fetch upstream` and record `git rev-list --count HEAD..upstream/main`.
2. `git tag -f pre-sync-backup HEAD` — a recovery point costs nothing.
3. `git rebase upstream/main`. On conflict, read it: we edit almost no upstream
   file, so a conflict is unusual and the likely culprit is the root .gitignore.
4. Assert the invariant: 0 changed files under backend/, frontend/, deploy/,
   docs/. If this ever fails, stop — the whole model depends on it.
5. Run the gate: june-lint, vue-tsc, vitest. Expect lint noise until finding 1
   is fixed; typecheck and tests are the trustworthy signals today.
6. Diff the API contract since the last reviewed SHA -- both the TS clients
   (frontend/src/api, src/types) AND the Go response structs
   (backend/internal/handler), because upstream can change a JSON shape in Go
   without touching any TS file, and a stale client does not fail loudly.
7. Port only what converted components depend on. Record ports AND skips here,
   then advance the last reviewed SHA.

## Modal archetype map (part 05 sections `archetypes` and `map`)

Documentation, not components. 44 modals resolve to six shapes; once a modal is
classified it inherits everything. Recorded here because the classification is
the remaining work, and it is a decision per modal rather than a sweep.

| Archetype | Count | Width | Status |
|---|---|---|---|
| A Confirm | 4 | narrow 420 | built: `ConfirmDialog.vue` |
| B Short form | 7 | normal 520 | primitive is `BaseDialog`; not migrated |
| C Sectioned form | 7 | wide 720 | built: `SectionedDialog.vue` |
| D Picker / list editor | 9 | wide 720 | not built |
| E Read-only detail | 8 | extra-wide 960 | not built |
| F Process / step | 9 | narrow 420 | not built |

Notes that change behaviour, not just looks:

- **12 modals get narrower.** "Wide" had been absorbing four different real
  widths by drift. This is the single biggest visual change in part 05, and it
  lands all at once when the classification is applied.
- **Archetype D is the largest behavioural change in the part**: 3 of its 9
  modals currently open a second dialog to edit one row. Removing that nesting
  is the "a modal may not open a modal" call, and it is also why
  `BaseDialog.zIndex` still exists (6 consumers stack dialogs).
- **Archetype E is the only one where `closeOnClickOutside` should default true**
  — nothing to submit.
- **B is often a one-word fix**: 4 of its 7 are currently `wide` for 4-5 fields,
  which is why they read as empty.
- A few modals get WIDER: `UserCreateModal` and `UserEditModal` move normal ->
  wide because they are actually archetype C, not B.
- `AccountTestModal` transitions F -> E once its streamed run finishes. That is a
  state change within one dialog, not a second dialog.
- **`CreateAccountModal` (6,338 lines) and `EditAccountModal` (4,799) are
  excluded** from the C migration. Part 05's own `#calls` calls them "a project
  rather than a pass" — they need a credential component per platform behind one
  interface before the archetype pays off.
