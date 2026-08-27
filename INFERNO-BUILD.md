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

Backend first, frontend second. The order matters: `vite.config.ts:53` registers an
`injectPublicSettings` plugin that fetches `${backendUrl}/api/v1/settings/public` while
serving, so a dev server started against a dead backend serves a page missing its
injected settings.

```sh
open -a Docker

# deploy/.env is gitignored (deploy/.gitignore:11). POSTGRES_PASSWORD has no default
# and compose refuses to start without it. BIND_HOST defaults to 0.0.0.0 -- pin it to
# loopback so a local dev instance is not exposed on the LAN.
cat > deploy/.env <<'EOF'
BIND_HOST=127.0.0.1
SERVER_PORT=8080
POSTGRES_PASSWORD=inferno-local-dev
EOF

docker compose -f deploy/docker-compose.local.yml up -d   # sub2api + postgres 18 + redis 8
docker compose -f deploy/docker-compose.local.yml ps      # wait for all three "healthy"

cd inferno-frontend && pnpm install && pnpm dev           # :3000, proxies to :8080
```

Check the backend is actually up before blaming the frontend:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/v1/settings/public  # 200
curl -s http://127.0.0.1:8080/setup/status   # {"needs_setup":false,"step":"completed"}
```

Overrides, both read in `vite.config.ts:83-84`: `VITE_DEV_PORT` (default `3000`) and
`VITE_DEV_PROXY_TARGET` (default `http://localhost:8080`). The dev server proxies
`/api`, `/v1` and `/setup`, so no frontend config change is needed for a local backend.

**Admin credentials.** First boot seeds `admin@sub2api.local` and prints a one-time
generated password to the container log. It is printed once and not recoverable later:

```sh
docker compose -f deploy/docker-compose.local.yml logs sub2api | grep -i -A2 password
```

Confirm you are looking at a genuinely fresh install rather than reusing old state by
checking the data dir's age -- a completed `/setup/status` on a stale volume looks
identical to a fresh one from the API alone.

**Trap: stale Vite dep-optimizer cache.** A first `pnpm dev` can 500 every request to
`/src/main.ts` with `Failed to resolve import "@/stores/app"` even though
`src/stores/app.ts` exists and exports correctly. It is a stale
`node_modules/.vite` cache, not a code defect. Fix, no source change needed:

```sh
rm -rf node_modules/.vite && pnpm dev
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

### 2026-08-15 — third sync, following the derived runbook

**115 commits behind.** Range `48eb3766d` (2026-08-09, the previous sync's
endpoint, found via `git merge-base pre-sync-backup upstream/main` since no
concrete SHA from a later run ever landed on this branch) .. `c204d33b0`
(2026-08-15, new last-reviewed SHA).

**Two prior automated sync PRs (#1 `sync/reconcile-2026-08-10`, #2
`sync/reconcile-2026-08-11`) were still open and unreviewed at the start of
this run, and both are now stale/conflicting against this branch.** Their
analysis was read for precedent (see the backup.ts and channel.ts decisions
below) but their code was not merged or reused — this run redid the diff
against current `HEAD` from scratch, since nothing they ported ever landed.
A human should close or rebase #1 and #2 rather than merge them as-is.

**Rebase: clean.** All 54 of our commits replayed with zero conflicts.
**Invariant asserted, twice** (immediately after rebase and again after
porting): 0 files changed under `backend/`, `frontend/`, `deploy/`, `docs/`.

## Ported (5 files wholesale, 1 hand-merged)

Wholesale copy (mirror differed, we had never touched these):

1. `inferno-frontend/src/constants/channel.ts` -- upstream added
   `BILLING_MODE_VIDEO` and `BILLING_MODEL_SOURCE_RESPONSE` to their
   respective unions (additive only). Matches a new
   `oneof=...response_model` value on `BillingModelSource` in
   `channel_handler.go`. Consumers include several unconverted components;
   none broke under `vue-tsc` since both additions are new union members,
   not removals.
2. `inferno-frontend/src/composables/useModelWhitelist.ts` (+ its spec) --
   upstream added the `grok-4.6` model to the whitelist/rename tables. Pure
   data addition, no shape change. Consumers
   (`CreateAccountModal.vue`, `EditAccountModal.vue`,
   `BulkEditAccountModal.vue`, `ModelWhitelistSelector.vue`) are all
   unconverted or partially-touched, but the file itself was never ours to
   diverge from, so upstream's version is simply correct.
3. `inferno-frontend/src/utils/accountUsageRefresh.ts` (+ its spec) --
   upstream added `buildGrokUsageRefreshKey` (a Grok analogue of the
   existing `buildOpenAIUsageRefreshKey`, used to detect when a Grok
   quota/billing snapshot changed and a row needs to re-render). Ported the
   utility; **did not wire it into `AccountsView.vue`** (that file is under
   the ignored `views/` bucket, and wiring it in is real feature work, not a
   port). Recorded under "owed cross-file work" below so this doesn't read
   as done -- Grok quota cells in the admin accounts table do not yet get
   the same reactive refresh that OpenAI/Codex cells already have.
4. `inferno-frontend/src/views/admin/groupsImagePricing.ts` (+ its spec) --
   lives under `views/admin/` by path but is inert exported config (a
   platform-name `Set`), not a Vue view -- same call as the 2026-08-10 log
   entry made for this exact file. Upstream added `"composite"` to
   `imagePricingPlatforms`, enabling image-pricing for Composite groups.
   Sole consumer is the unconverted `GroupsView.vue`.

Hand-merged (file is in our own touched history, so this was a targeted
insertion, not a copy):

5. `inferno-frontend/src/types/index.ts` -- upstream added two fields to the
   `Group`/`AdminGroup`/`CreateGroupRequest`/`UpdateGroupRequest` family:
   `long_context_pricing_enabled: boolean` and
   `model_pricing: ChannelModelPricing[]` (the latter required on
   `AdminGroup`, optional on the two request types). Verified against
   `backend/internal/handler/dto/types.go`, `dto/mappers.go`, and
   `group_handler.go` -- all three matched exactly, no surprises. This is a
   real, live backend contract (not inert): `dto/mappers.go` now populates
   `ModelPricing` on every `AdminGroup` response. Diffed the merged result
   against the mirror afterward -- byte-identical, confirming nothing else
   in the file had drifted.
   **One fallout, fixed:** `src/views/dev/SpecimenView.vue`'s `makeGroup()`
   fixture factory built an `AdminGroup` without the new required
   `model_pricing` field, which `vue-tsc` correctly rejected once the type
   became non-optional. Added `long_context_pricing_enabled: false,
   model_pricing: [],` to the factory's inert defaults. `SpecimenView.vue`
   is nominally orchestrator-only per `SWARM-REGISTRY.md`'s no-worker-touch
   list, but that rule guards against parallel workers colliding on it, not
   against fixing contract fallout during reconciliation -- flagging the
   edit here rather than doing it silently.

## Skipped, with reasons

1. **`inferno-frontend/src/api/admin/backup.ts`** -- upstream changed
   `getDownloadURL()`'s response from `{ url: string }` to
   `{ url?: string; parts?: BackupDownloadPart[] }` for large-file
   multi-part backup downloads (verified live in
   `backup_handler.go`: `response.Success(c, download)` now returns the
   full struct instead of `gin.H{"url": url}`). Sole consumer,
   `BackupView.vue`, is unconverted and unconditionally does
   `link.href = result.url`. Porting the type alone would make `.url`
   optional under `vue-tsc` without adding any handling for `.parts`, so it
   would compile clean and still break downloads of large backups. Left
   both files at their pre-sync shape. (The two abandoned PRs disagreed with
   each other here -- #1 ported `backup.ts` + `BackupView.vue` together as a
   matched-pair exception to the views-ignore rule, #2 skipped both. This
   run follows #2's read: the views-ignore rule exists precisely so a
   redesign candidate isn't invested in twice, and the type-only port helps
   nobody without the view change alongside it.)
2. **`inferno-frontend/src/api/admin/groups.ts`** (`getUsageSummary`) --
   upstream dropped the `timezone` query param (backend now computes "today"
   server-side via `service.GroupUsageTodayStart(time.Now())`, confirmed in
   `group_handler.go`) and added a `yesterday_cost` field to the response.
   Sole consumer is unconverted `GroupsView.vue`, which still passes a
   `timezone` arg -- harmless (the backend simply ignores unbound query
   params) but the extra field is invisible to it. Deferred to that view's
   conversion.
3. **`inferno-frontend/src/views/admin/ops/utils/opsFormatters.ts`** (+
   `OpsDashboardHeader.vue`, which is the only consumer) -- upstream added
   `formatMemorySizeMB()` for nicer memory-size display on the ops
   dashboard. Both files are under the ignored ops-views bucket and neither
   is touched by us; porting the formatter alone would add a function with
   zero callers in our tree, since wiring it in means editing
   `OpsDashboardHeader.vue`. Skipped as a matched pair.
4. **`BackupView.vue`, `ChannelsView.vue`, `GroupsView.vue`, `UsageView.vue`**
   and their component/spec siblings (`BulkEditAccountModal.vue`,
   `PricingEntryCard.vue`, `UsageTable.vue` + spec, `PlatformTypeBadge.vue` +
   spec, `AccountsView.sparkShadow.spec.ts`, `GroupsView.columnSettings.spec.ts`,
   `UsageView.spec.ts`) -- all under the explicit `components/`/`views/`
   ignore rule.
5. **New upstream test files not ported**: `admin.groups.usage-summary.spec.ts`,
   `BackupView.spec.ts`, `opsFormatters.spec.ts` -- each tests one of the
   three skips above; porting the test without the code it tests would just
   fail.
6. **`pnpm-lock.yaml`** differs only in a transitive `nanoid` patch version
   (3.3.17 vs 3.3.18); `package.json` is byte-identical to the mirror. Not a
   port target -- a fresh `pnpm install` against the same `package.json`
   naturally resolves to whichever patch is current: not a real drift.
7. **`components/common/ProxyAdBanner.vue`, `components/layout/AppHeader.vue`**
   (present in the mirror, absent from ours) -- both are standing,
   deliberate June deletions from before this run (monetisation banner and
   the old shell header), not new upstream churn. No action.
8. Backend-only changes verified but not ported (no client-visible contract
   change): `api_key_handler.go` (+47 lines, server-side validation only),
   `failover_loop.go`, `grok_audio.go`, `openai_gateway_handler.go`,
   `openai_x_search.go` (new file), `gateway_web_search.go`,
   `security_audit_helper.go` -- all gateway/proxy-path logic (the actual AI
   request path external clients hit), not admin/user SPA-facing endpoints,
   so nothing under `frontend/src/api` reads them.

## Gate output (real, not carried forward)

Before port: `june-lint` clean across 99 converted file(s) · `vue-tsc
--noEmit` 0 errors · `vitest run` 220/220 files, 1536/1536 tests (one
pre-existing, non-blocking `useRoute` mock unhandled-rejection warning in
`DashboardView.spec.ts`, unrelated to this sync, present before and after).

After port: `june-lint` clean across 98 converted file(s) (the drop by one
reflects `types/index.ts` -- see note below, not a scope regression) ·
`vue-tsc --noEmit` 0 errors (after the `SpecimenView.vue` fixture fix) ·
`vitest run` 220/220 files, 1542/1542 tests (the +6 came from the two ported
spec files) · `vite build` succeeded (pre-existing >500kB chunk-size
warnings only).

## Owed cross-file work, added to the PENDING checklist

- `AccountsView.vue` does not call the newly-ported
  `buildGrokUsageRefreshKey`, so Grok quota/billing cells in the admin
  accounts table lack the reactive refresh-on-snapshot-change that OpenAI
  cells already have. Wiring this in means editing `AccountsView.vue`
  itself, which is out of scope for a port.

## Unsure about / flagging for review

- The `backup.ts` skip (item 1 above) reverses the prior (abandoned) run's
  call. Flagging explicitly since it is a real product gap (multi-part
  backup downloads render a plain URL link, which will 404 or download a
  partial file, for any backup large enough to need splitting) that will
  stay open until either `BackupView.vue` converts or someone makes a
  deliberate call to invest in the unconverted view early.
- Two stale, unreviewed sync PRs (#1, #2) are sitting on this repo from
  2026-08-10 and 2026-08-11. Neither was merged, both are now marked
  "dirty" against the current branch tip. Recommend closing both rather than
  attempting to land them -- their content is superseded by this entry.

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

## BLOCKING: 32 i18n keys pending for the part 07 shell

The shell renders raw key paths until these are added under `shell.*` in en and
zh. The orchestrator owns src/i18n/; workers report keys (CONVENTIONS rule 6).

accessGroup Access · accountMenu Account menu · affiliateInvites Affiliate
invites · affiliateRebates Affiliate rebates · affiliateTransfers Affiliate
transfers · apiKeys API keys · auditLogs Audit logs · billing Billing ·
collapseSidebar Collapse sidebar · darkMode Dark mode · expandSidebar Expand
sidebar · language Language · lightMode Light mode · mainNavigation Main
navigation · modelPlaza Model plaza · monitor Monitor · myAccount My account ·
openMenu Open menu · overview Overview · paymentDashboard Payment dashboard ·
people People · preferences Preferences · promoCodes Promo codes · promptAudit
Prompt audit · redeemCodes Redeem codes · referrals Referrals · riskControl Risk
control · roleAdministrator Administrator · roleMember Member · sectionsListLabel
Sections · switchSection Switch section · system System · traffic Traffic

Note: the shell deliberately does NOT reuse the existing nav.lightMode,
nav.darkMode, nav.apiKeys, nav.channelManagement keys. Their values are Title
Case ("Light Mode", "API Keys"), which violates ground rule 1 even when merely
moved. The old keys stay for unconverted screens; these replace them as screens
convert, and the old ones die with their last consumer.

## AppHeader deletion: what lost its home

Eight functions had no destination in v2's four-item header inventory and were
dropped. Listed because a header deletion is exactly where a feature disappears
unnoticed:

| Dropped | Note |
|---|---|
| **Balance pill + hover panel** | **Customer-visible.** Customers could read their balance from any screen; now nowhere until part 15 Billing exists. The most significant of these. |
| SubscriptionProgressMini | Same class; belongs on Billing. |
| Docs link | No home specified. |
| GitHub link (admin) | No home. |
| Contact support | No home. |
| "Restart tour" button | The MECHANISM is intact (onboardingStore.replay, AppLayout's defineExpose). Only the UI trigger is gone, because the prototype fixes the avatar menu at three items. |

Rehomed rather than dropped: AnnouncementBell and LocaleSwitcher to the sidebar
footer, Model Plaza to a customer nav row, the user dropdown to the avatar menu.
A mobile hamburger was ADDED to AppLayout (not in the prototype, which previews
at 1280px) so mobile nav does not break under 1024px.

## Routes now unreachable from the nav

Intended: /profile, /monitor, /redeem, /orders.
**Unintended, flagged:** /batch-image and /purchase are absent from the
prototype's 7-row userNav array and were not among the five named deletions, so
building the nav exactly as drawn dropped them too. Routes still work; they just
have no sidebar entry. Needs a decision.

## OPEN: 3 failing tests after part 04, and one width decision

### The tests — 1525/1528, three red, all diagnosed

`src/components/common/__tests__/DataTable.spec.ts` (2) and one more in a second
file. Do NOT weaken the assertions to make these green; each needs a specific fix.

1. **"renders paired sort arrows and highlights the active direction"** —
   asserts two `<svg>` per header with Tailwind `text-primary-600` /
   `text-gray-300`. Part 04 explicitly REMOVES the permanent double chevron in
   favour of one caret on the sorted column only. The test encodes the old
   behaviour and needs rewriting to the new one, not patching.

2. **"emits controlled current-page selection while preserving off-page keys"** —
   was `.setValue(true)`; changed to `.trigger('click')` and it still does not
   emit. The mobile equivalent WAS fixed by the same change and now passes, so
   the mechanism is right and something else differs on the desktop path.
   Not diagnosed. Needs someone to check whether the `<thead>` select-all is
   reachable in that test's render mode.

**Systemic cause worth knowing:** June's `Checkbox.vue` listens on `click`, not
`change` — deliberately, because jsdom flips `.checked` during a programmatic
click's activation step but does not synthesise a following `change`. So
`wrapper.setValue()` never reaches it. EVERY existing test that drives a
checkbox with `.setValue()` will break as `Checkbox.vue` is adopted. That is a
migration cost of the component, not a bug in it, and it will recur.

### The width decision — affects all 15 admin table pages

`TablePageLayout` now caps content at `--content-max` (760px, stepping to 880 at
1440 and 1000 at 1920) and centres it, because `spec.pagelayout.tokens` says
"--content-max column".

But: 01-TOKENS describes `--content-max` under **"Reading columns"** — prose and
forms, where a narrow measure aids reading. And the prototype's own table demo
uses `max-width: 1080px`, not the token.

A ten-column admin table letterboxed at 760px is a real usability regression,
and it lands on 15 pages at once. Options: keep the token (trust the spec text),
use 1080px (trust the prototype's own demo), or exempt tables from the reading
column entirely. **Not decided — flagging rather than shipping it silently.**

### Also outstanding from part 04

- No page shows a title yet. `TablePageLayout` gained optional `title` and
  `description` props, but no consumer passes them. 15 view files need a pass.
- `stickyActionsColumn` default stays `true`, not the spec's `false`: the row
  actions menu that would replace it is not wired into any consumer yet.
- `Column.align` not added — `Column` lives in `types.ts`, which no agent owned.
- `EmptyState` has no `kind` prop, so part 04's four empty variants
  (new / filtered / search / error) are not available yet.

## Part 04's three open calls, researched against the real code

Read before deciding any of them. The spec's prose understates two and
mis-frames one.

### 1. Merge UserTokenRanking + TopUsersLeaderboard? -- a refactor, not a rename

They are less alike than the spec implies:

  admin/usage/UserTokenRanking.vue      a real <table>. Fetches its own data via
                                        getUserBreakdown(), owns sort state,
                                        ranks by tokens.
  admin/payment/TopUsersLeaderboard.vue NOT a table today. A card of flex rows
                                        grouped by currency, fed pre-grouped
                                        data as a prop, fetches nothing.

They share no data shape. One fetches by date range and filters; the other
receives Record<string, TopUserPaymentStats[]>. Merging means choosing one
data-flow pattern and migrating the other's caller.

Worth knowing: both already drifted. Gold/silver/bronze badges use bg-amber-100
in one and bg-yellow-100 in the other -- same intent, two implementations. That
drift is the actual argument for merging.

### 2. Usage-log tooltip -> row expansion -- ALREADY DESIGNED, flagged for blast radius

This is not an undecided design. The spec names the exact pattern to reuse:
UserBreakdownSubTable.vue (section 12), described as "the pattern for every row
expansion in the product, including the usage log tooltip replacement". It sits
in `calls` because it touches the busiest screen, not because anything is
unresolved. Read it as "confirm before I touch your most-viewed table".

Today UsageTable.vue teleports a floating panel on mouseenter/mouseleave showing
the full token breakdown -- input/output split by text vs image, cache creation
split by 5m/1h TTL. Real depth, visible only while the pointer holds still.

A row expansion instead: the numbers become selectable and copyable (a hover
tooltip vanishes the moment you move toward it), it works on touch where hover
does not exist, it cannot be clipped at a viewport edge, and it stays open while
you cross-reference another row or screenshot it.

### 3. visibleColumnKeys -- the same feature exists three times

  UserErrorRequestsTable.vue  a real `visibleColumnKeys?: string[]` prop,
                              filtering a 12-column list
  OpsErrorLogTable.vue        the identical filter pattern, copy-pasted
  KeysView / UsersView        a THIRD, unrelated implementation: a hand-rolled
                              column-settings dropdown in the view, which
                              pre-filters and passes the result as `columns`

So "let the user hide columns" has been reinvented three times with three UIs.
Moving it onto DataTable gives all 22 consumers the affordance for free. It is
flagged as a call only because it means touching the one file 22 tables depend on.

### stickyActionsColumn -- why it was not flipped, in detail

`getStickyColumnClass()` pins the actions column `position: sticky; right: 0` so
it stays reachable while scrolling a wide table. UsersView and AccountsView carry
10+ columns; without pinning you scroll right to reach edit/delete, then back.

A second mechanism is entangled with it: `expandableActions` + `actionsCount` +
`checkActionsColumnWidth()` measures whether a row's action BUTTONS overflow
(AccountsView passes actions-count="7") and grows the column to fit them inline.

The row-actions menu replaces both at once: one 28px trigger and a popover
holding all nine actions, which needs no pinning because it teleports beside the
trigger. That is why three spec items are really one dependency -- the menu, the
sticky default, and the last gradient shadows. Flipping the default before the
menu exists would drop the actions column with nothing in its place.

DataTable needs NO new API to support it: the existing `cell-actions` slot is
already sufficient.

## CORRECTION: the action menu does NOT yet unblock the sticky column

I claimed AccountActionMenu would unblock three spec items at once (the row
actions menu, stickyActionsColumn defaulting false, and the last gradient
shadows). That was wrong, and the agent that built it checked rather than
assumed.

`AccountActionMenu.vue` is ONLY the popover content, driven by show/account/
position props. Three things it does not own still live in `AccountsView.vue`
(~lines 432-446 and 1648-1698):

  - the 28px hgi-more-horizontal trigger
  - the separate inline Edit and Delete buttons
  - openMenu()'s position math

So a row today renders three inline buttons PLUS this menu -- not the "one
trigger and a menu" the spec describes. `stickyActionsColumn` must stay true.

**The follow-up, in AccountsView.vue:**
1. Collapse the inline Edit / Delete / More into ONE 28px trigger that opens
   this menu with every action in it.
2. Put aria-haspopup and aria-expanded on that trigger, and return focus to it
   when the menu closes. AccountActionMenu cannot own these -- it never renders
   the trigger. This is a real accessibility gap until then, reported not hidden.
3. Only then flip DataTable's stickyActionsColumn default to false and retire
   expandableActions / actionsCount / checkActionsColumnWidth(), and the last
   gradient shadows go with them.

Note the menu also has no destructive group: account deletion is a separate
inline button in AccountsView, not one of this component's eleven emits. When
delete moves into the menu it becomes the fourth, --destructive, last group.

## Part 08 cells: 7 more failing tests, and why they are the right kind of failure

`UpstreamBillingRateCell.spec.ts` (2), `OllamaCloudUsageCell.spec.ts` (2),
`UserPlatformQuotaCell.spec.ts` (3). Total repo red is now 10.

Every one asserts something the migration removes ON PURPOSE:

  - literal Tailwind classes (`text-emerald-400`) -- CONVENTIONS rule 5 forbids
    Tailwind utilities in converted components, so these cannot pass and also be
    correct
  - the removed `UsageProgressBar` subcomponent
  - the old exhaustive per-window listing that the closest-limit rule replaces
  - a literal ellipsis loading glyph, now a flat skeleton

Satisfying them would mean keeping Tailwind classes or the pre-June layout.
They need rewriting to the new behaviour, not patching. Do not weaken them.

### Colour-by-category found and fixed -- the rule these cells broke most

- `OllamaCloudUsageCell` passed `color="indigo"` / `"emerald"` to
  UsageProgressBar, colouring the chip by WHICH WINDOW it was. That is category
  colour, exactly what ground rule 5 forbids. Now on CapacityBar's shared risk
  threshold.
- `UserConcurrencyCell` had its own red/yellow/grey cutoffs. Moved onto the same
  shared rule.
- `UpstreamBillingRateCell` reuses GroupBadge's existing `--warm-strong`
  precedent for a rate mismatch (state) rather than inventing a second override.

### Open items from this pass

1. **`AccountsView.vue:307` still passes `:max-display="4"`**, overriding the new
   default of 2 that the spec asked for. The row no longer grows vertically, but
   four chips can still push width. One-line fix in a file this agent did not own.
2. **`UserPlatformQuotaCell` deviates from the literal spec.** "One bar per
   platform stacked" would stretch a 36px row for accounts with 3-5 platforms,
   so it shows the window closest to its limit plus a `+N` chip, mirroring
   AccountUsageCell's own resolution. Full detail stays in the existing modal.
3. **tabular-nums deliberately not used**, overriding the prototype's own demo
   styling, per 01-TOKENS: only for a live-ticking value in a fixed-width
   container. Matches CapacityBar and DataTable, which use it nowhere.
4. **A per-row interval was removed** — OllamaCloudUsageCell ran a ticking
   countdown clock per table row. Now computed once per render.

## Part 08 departures: what was actually wrong, and what is still owed

### The billable-call bug was not where the spec said

The spec frames departure 3 as "a cell never fetches -- the table batches by
visible row id". `GrokQuotaProbeCell` was never auto-fetching; `handleProbe`
only ever ran from a click. The real problem: `AccountUsageCell` embedded that
probe button inline on EVERY visible Grok row, which normalises a real billable
xAI call as a routine per-row action. Removed the embedding.

And no batch path exists for it. The spec concedes this: the probe is inherently
a live upstream call. So the fix is not batching, it is making the column
opt-in and off by default, with the cost stated at the point of switching it on.

### Owed: the opt-in Grok column (DataTable / AccountsView, unowned)

An off-by-default column, gated by a chooser that says "sends one billable
request per visible row", rendering GrokQuotaProbeCell per row only once enabled.

### Owed: the column-header window picker (AccountsView, unowned)

AccountUsageCell now accepts an additive optional `pinnedWindowKey`. A Select in
the column header sets it through the existing `#cell-usage` slot
(AccountsView ~line 316). Window keys: five_hour, seven_day, seven_day_sonnet,
seven_day_fable, gemini3_pro/flash/image, claude, grok_24h/7d/30d,
pro_daily/flash_daily/shared_daily, quota_daily/weekly/total.
For the column to SORT by the pinned window, AccountsView also needs a derived
sortable field computed the same way.

### Owed: the 55 -> 9 quota collapse (unowned files)

```ts
interface QuotaDimension {
  key: 'daily' | 'weekly' | 'total'
  limit: number | null
  used: number
  resetHour: number | null
  resetDay: number | null
  resetTimezone: string | null
  notifyEnabled: boolean | null
  notifyThreshold: number | null
  notifyThresholdType: 'percent' | 'absolute' | null
}
```
QuotaLimitCard 27 props -> 3. QuotaDimensionRow 26 -> 4. Files: those two plus
their callers CreateAccountModal.vue and EditAccountModal.vue -- which are the
6,338 and 4,799 line files part 05 calls "a project rather than a pass".

### Owed: 19 i18n keys (orchestrator owns src/i18n/)

admin.accounts.usageWindow.{fiveHour '5 hour', sevenDay '7 day',
sevenDaySonnet '7 day Sonnet', sevenDayFable '7 day Fable', thirtyDay '30 day',
oneDay '1 day', total 'Total', grok24h '24 hour', moreWindows '{count} more',
hideWindows 'Hide extra windows'}
admin.accounts.capacity.dimension.{concurrency 'Concurrency',
windowCost '5 hour cost', sessions 'Sessions', rpm 'RPM',
quotaDaily 'Daily quota', quotaWeekly 'Weekly quota', quotaTotal 'Total quota'}
admin.accounts.capacity.{moreLimits '{count} more', hideLimits 'Hide extra limits'}

Until these land, AccountUsageCell and AccountCapacityCell render raw key paths.

### One deviation worth knowing

CapacityBar's label typography is --fs-md/--foreground at every size; the usage
cell's anatomy wants --fs-sm/muted in a table-cell context. The agent composed
the shared component as-is rather than forking its styling. If a cell label
looks too heavy, that is why, and the fix belongs in CapacityBar.

## Accurate red-test count at the end of phase 3: 38, across 9 files

Not the 20 previously recorded. The full list:

  components/common/__tests__/DataTable.spec.ts
  components/layout/__tests__/TablePageLayout.spec.ts
  components/account/__tests__/AccountUsageCell.spec.ts
  components/account/__tests__/OllamaCloudUsageCell.spec.ts
  components/account/__tests__/UpstreamBillingRateCell.spec.ts
  components/user/__tests__/UserPlatformQuotaCell.spec.ts
  components/charts/__tests__/ModelDistributionChart.spec.ts
  components/charts/__tests__/GroupDistributionChart.spec.ts
  components/charts/__tests__/TokenUsageTrend.spec.ts

Every one is the same category: it asserts pre-June behaviour the redesign
deliberately removes. Four recurring causes:

1. **Literal Tailwind classes** (`text-emerald-400`, `text-primary-600`).
   CONVENTIONS rule 5 forbids Tailwind in converted components, so these cannot
   pass AND be correct.
2. **Removed DOM**: the doughnut markup, `UsageProgressBar`, the paired sort
   chevrons, the exhaustive per-window listing.
3. **`.setValue()` on a June Checkbox.** It listens on `click`, not `change`,
   because jsdom does not synthesise `change` for a programmatic click. Every
   such test breaks as Checkbox.vue is adopted -- ~40 raw inputs still to
   migrate, so this recurs.
4. **jsdom cannot resolve `color-mix()`/`oklch()`**, so `useChartTokens` returns
   empty strings under test. New chart tests must stub `tokens` rather than
   assert resolved colour values.

**Do not weaken these.** They need rewriting against the new behaviour, which is
a real task -- roughly one pass per file -- and is the largest single piece of
outstanding work in the project.

Green at push time: build succeeds, vue-tsc 0 errors, lint clean across 65
files, 1490/1528 passing.

## All 38 tests fixed. Suite green at 1531/1531.

Nothing weakened, nothing skipped. Two files gained coverage: the chart specs
went 10 -> 12 (covering the split into two Line instances, which the originals
structurally could not test), and the cell specs 25 -> 40.

Three guarantees were genuinely dropped by the redesign and the tests say so
rather than pretending otherwise:

  - "every usage window is visible at once" -> now "every window is REACHABLE"
    via the expansion. Deliberate; it is the whole point of departure 1.
  - per-window token/cost text in AccountUsageCell -- see the regression below.
  - the used/limit fraction and the em-dash placeholder in UserPlatformQuotaCell.

## TWO COMPONENT REGRESSIONS found while rewriting the tests -- BOTH NOW FIXED

Found by the test rewrite; fixed in a follow-up pass. Kept here because the
findings are the point: weakening the tests would have hidden both.

### 1. Reset times are invisible for single-window accounts  (user-facing)

`AccountUsageCell` renders `resetsLabel` ONLY for windows inside the
`.uc-expandlist` expansion. The primary, always-shown window never displays its
`resetsAt`.

For any account with a single active window -- Grok, single-image Antigravity,
both common -- the reset time is now completely absent from the UI, where it was
always shown before. An operator cannot see when a rate limit lifts, and there
may be no expansion to open.

This is a real loss of information, not a layout change. Fix: render the primary
window's reset label too.

### 2. WindowStats is fetched, typed, and silently dropped  (waste, maybe a bug)

Per-window requests/tokens/cost (`window_stats`) arrives on the wire and is
dropped before render in every `*Windows` computed -- anthropicWindows,
openAIWindows, antigravityWindows, grokWindows, geminiWindows. Nothing in the
template can observe it.

Either it moved to a detail modal deliberately, or it was lost in the rewrite.
Confirm with whoever owns the cell. We are paying for the data either way.

## Two test-mechanics notes worth keeping

- **jsdom cannot resolve color-mix() or oklch()**, so useChartTokens returns
  empty strings under test. Mock the composable with sentinel strings and assert
  which series reads which ramp index. Never assert a resolved colour.
- **Prefer semantic hooks over class names when re-expressing an assertion.**
  Swapping `text-emerald-400` for a June token name is the same brittleness in
  new clothing. The upstream-billing spec now asserts the rendered WORD; the
  sort spec asserts aria-sort and data-sorted, not the opacity CSS -- jsdom does
  not compute :hover or attribute-selector styles, so asserting those would be
  testing nothing.

### Resolution

**1. Reset times -- fixed.** `barTrailing()` now renders
`{percent} · {resetsLabel}` into CapacityBar's `trailing` prop on all six
primary bars. It costs ZERO extra height: CapacityBar's head row already exists
whenever `trailing` is passed, so the 36px row is untouched. It also reuses
`pctLabel` and `resetsLabel` verbatim rather than growing a second formatter.
Pre-June `UsageProgressBar` put percent and reset on one inline row too, so this
restores the old layout rather than inventing one.

Covered by a new test: a Grok Free account (single window, no expansion exists)
must still show its reset time. It fails against the pre-fix code.

**2. window_stats -- it was case (b), genuinely lost.** The investigation ruled
out a relocation: `AccountStatsModal` is the only other place showing numbers,
and it reads `stats.summary` from a different API entirely, not
`UsageProgress.window_stats`. The only thing that ever rendered this was the
deleted window-stats row in `UsageProgressBar`, which had no replacement.

Now surfaced in the EXPANSION rows only, never on the primary bar, so the 36px
budget holds. Reuses the `req` / compact-tokens / `A $` / `U $` convention the
today-stats chips in the same file already use.

**Correction to this document:** the claim above that `antigravityWindows` also
dropped window_stats was wrong. `AntigravityModelQuota` and `GrokBillingSummary`
never carried a request/token/cost breakdown, so there was nothing to drop for
Antigravity or the grok_7d/grok_30d billing bars. The real loss was in
anthropic, openAI, gemini, and Grok Free's grok_24h window.

Suite: 1532/1532.

## Browser verification against a live backend — 1 critical bug found and fixed

First time the app was ever loaded in a real browser since `AppHeader.vue` was deleted and
`AppLayout.vue` rewritten. Run against the real Docker backend, logged in as the seeded
admin, driven through Playwright at 1440x900. 19 screenshots in
`~/inferno-verify-screenshots/`.

### CRITICAL, FIXED: the entire sidebar navigation was dead

`AppSidebar.vue`'s `NavRowLink` built rows with:

```js
h('router-link' as any, { to: props.row.path, ... }, () => [icon, label])
```

A **string** tag in `h()` is treated as a native HTML element — it does NOT resolve a
globally-registered component. This emitted a literal `<router-link>` custom element with
no `href` and no click behaviour; and because a function was passed as children to what Vue
considered a native-tag vnode, the icon and label **did not render at all**.

Every nav row in both shells — the whole customer nav, every admin section's rows, the
"My account" groups — was an empty, non-clickable box. Visible in `07-admin-dashboard.png`
as bare group labels with blank space beneath them.

Fix: import `RouterLink` from `vue-router` and pass the component reference.

```js
import { useRoute, useRouter, RouterLink } from 'vue-router'
h(RouterLink, { to: props.row.path, ... }, () => [icon, label])
```

**Why the whole gate missed it.** `vue-tsc` was silenced by the `as any` — which is exactly
what it was suppressing. june-lint has no opinion on render functions. `vite build` doesn't
resolve components. And the 1,536 unit tests stub the router, so `router-link` renders as
`<router-link-stub>` and every assertion on `to="..."` passes. Four green gates, zero
navigation.

**Lesson: `as any` on a component tag is a defect smell, not a convenience.** Grep for
others before shipping. And no gate we own can catch a dead-but-well-formed vnode — only
loading the page can.

Re-verified after the fix, independently, not from the agent's self-report:
`vue-tsc` 0 errors · june-lint clean across 66 files · `vite build` OK ·
`vitest` 1536/1536 across 220 files.

### Confirmed working in a real browser

Section switcher popover (6 sections, correct counts) · Select popover (teleported,
searchable, checkmark on active) · avatar menu (exactly 3 items) · sidebar collapse ·
BaseDialog via the compliance dialog · sticky columns (`position: sticky; right: 0` on
Actions) · bulk-selection bar tinting with brand-tint clay, not blue.

Design tokens resolve for real: body background `oklch(0.9396 …)` light →
`oklch(0.1887 …)` dark, body font `ABC Diatype`, `.hgi-stroke` →
`hugeicons-stroke-rounded`, `data-brand="clay"`.

Chart tokens: no `<canvas>` ever mounts on an empty DB (components short-circuit to "No
data available" before instantiating Chart.js), so painted pixels are still unverified.
But `useChartTokens.ts`'s `resolveColorExpr` was exercised directly in the live page — all
9 probed tokens resolved to distinct non-empty colours and **re-resolved on theme toggle**
(`--foreground` `oklch(0.2724 …)` → `oklch(0.985 …)`). Mechanism sound; paint unproven.

### 4 further findings, NOT fixed — reported, not decided

1. **Legacy teal gradient on the primary CTA of the two busiest tables.**
   `AccountTableActions.vue`'s "Create Account" (and the "Create User" analogue) still use
   raw `class="btn btn-primary"`, rendering
   `linear-gradient(rgb(20,184,166), rgb(13,148,136))`. Confirmed byte-identical to pristine
   `frontend/`, so it is an unconverted surface, not a regression — but it is the loudest
   remaining teal in the product and it sits on `/admin/accounts` and `/admin/users`.
2. **The onboarding tour auto-navigates the browser through most admin routes unprompted.**
   After one interaction with a tour-tagged element, `useOnboardingTour.ts`'s interactive-step
   advance (`.click()` at lines 145 and 480) drove the app through
   `/admin/settings → /admin/audit-logs → /admin/groups → /admin/usage → /keys →
   /subscriptions → /admin/users → /admin/ops → /admin/dashboard → …`, fetching each page's
   data, with no further input. `history.length` reached 50. Whether the tour is meant to be
   an autonomous walkthrough or wait for a real click is a design call, so it was left alone.
3. **Section switcher desyncs from the route on hard reload / deep link.** Documented as
   intentional at `AppSidebar.vue:562-573` — cold start always defaults to "Accounts" and the
   route watcher only fires on client-side navigation, not on mount. Reproduced: hard reload
   into `/admin/dashboard` highlights "Accounts" until the next in-app navigation. Visible in
   `07-admin-dashboard.png`.
4. **36px row height is a floor, not a cap.** `.dt-tr { height: var(--dt-row-h) }` does not
   constrain a table row; measured 46.5px on `/admin/users` with real content. Spec gap, not
   render-breaking. Needs `line-height` + cell padding control, or `max-height` semantics.

### Still unverified

- **The `isAdmin === false` customer sidebar** — the 7-row nav with three group labels. The
  only seeded account is admin, and both `homePath` and the nav template branch on `isAdmin`,
  so admins never see the customer-only layout. Needs a non-admin test account.
- **Chart canvas pixels** — needs seeded usage data.
- **Row height under many rows** — only ever had 0-1 rows.

## The 4 reported findings — resolved

### 1. Legacy teal CTA — FIXED, and generalised

`btn btn-primary` appears **185 times** across views phase 5 has not reached. Converting
them one by one is that phase's job — but the teal lived in ONE class definition, so
re-skinning it retires the teal everywhere at once.

New file `src/design-system/legacy-bridge.css`, imported last from `june.css`. Re-skins
`.btn` and every variant in June tokens, mirroring `Button.vue` exactly.

**Why a new file rather than editing `style.css`.** That file is 773 lines of the original
Tailwind component layer and is still byte-identical to pristine `frontend/`. Editing it
would pull the whole thing into june-lint's scope, where its 7 unrelated gradients and 34
shadow utilities would fail the gate for things this change is not fixing — and it would
turn every future upstream sync of that file into a merge. Leaving it untouched keeps
those syncs conflict-free, which is the entire point of the two-tree layout.

**The non-obvious part:** `bg-gradient-to-r` sets `background-image`, not
`background-color`. Setting a background colour alone would have painted *underneath* the
gradient and changed nothing on screen. `background-image: none` is what actually removes
the teal.

Payment-provider buttons (`btn-stripe`, `btn-alipay`, `btn-wxpay`, `btn-airwallex`) keep
their brand hue deliberately — ground rule 5 bans colour that encodes a *category*, but a
provider's colour is its identity, not a classification we invented. They inherit June
geometry from `.btn` and only lose the glow.

Verified in a live browser, not inferred:

| property | before | after |
|---|---|---|
| `background-image` | `linear-gradient(#14b8a6,#0d9488)` | `none` |
| `background-color` | teal | `oklch(0.34 0.008 84.59)` = `--primary` |
| `box-shadow` | `shadow-md shadow-primary-500/25` | `none` |
| `height` | padding-derived | `36px` = `--s2a-h-md` |
| `transition-property` | `all` | `background` (ground rule 6) |
| `border-radius` / `font-weight` | `rounded-xl` | `8px` = `--r-md` / `600` = `--fw-medium` |

Override order confirmed in the shipped bundle: legacy gradient rule at char 30807, June
override at 664249. It wins on **source order, not specificity** — so the `@import` must
stay last in `june.css`. That ordering is load-bearing.

### 2. Runaway tour navigation — FIXED (root cause was not where it looked)

The two `.click()` calls are not autonomous; they are Enter/Next handlers. The actual cause
is that `globalKeyboardHandler` is registered on `document` with `capture: true` and its
Enter branch exempted only `INPUT`/`TEXTAREA`. While the tour was active, Enter on **any**
button or link anywhere in the app was `preventDefault`ed, `stopPropagation`ed and
redirected into clicking the tour's current target.

That is the cascade: each Enter activated whichever nav row the tour pointed at, AppSidebar
advanced the tour to the next row, and the next Enter did it again — walking the app
through most of the admin routes without the user ever choosing a destination.

Fix: one `ownsKeyboard()` guard, applied to all three of Enter / ArrowRight / ArrowLeft
(which had three inconsistent guards between them). It yields whenever focus is on
something that genuinely owns those keys. Focus on the body still falls through to the
tour, so the popover's own "press Enter to continue" hint keeps working, and driver.js's
buttons are real `<button>`s so they yield and their native click runs `onNextClick` —
the intended interactive advance is unchanged.

### 3. Section switcher desync on hard reload — FIXED without breaking the tour

The old cold start always defaulted to Accounts, so a bookmark or hard reload into any
other section showed the wrong one until the next in-app navigation.

Naively deriving the section from the route reintroduces the exact failure the old comment
guarded against: the tour's first two interactive steps target `#sidebar-group-manage` and
`#sidebar-channel-manage`, both inside Accounts, and they fire before any navigation — so a
fresh admin landing on Overview would have both targets hidden and the tour would break.

`initialSection()` now returns the route's section, *except* for an admin who has not yet
seen the tour. Read via a new `composables/onboardingTourStorage.ts` rather than a local
localStorage read, for two reasons: calling `useOnboardingTour()` would register lifecycle
hooks and an auto-start timer (starting a second tour just to read a flag), and even a
type-only path through that module pulls `driver.js` plus its stylesheet into every sidebar
unit test. Keeping the key derivation in one place also matters because it embeds
`STORAGE_VERSION` — two copies would drift on the next bump and the sidebar would believe
the tour was pending forever.

### 4. 36px row height — NOT A BUG. Do not "fix" it.

`height` on a table row is a **minimum**: CSS table layout grows a row to fit its tallest
cell, and there is no honoured `max-height` for `display: table-row`. `--dt-row-h` sets the
density floor and cannot enforce a ceiling.

The measured 46.5px on `/admin/users` is not a DataTable defect. UsersView is unconverted
and renders its groups cell as a vertically stacked chip list (`flex flex-col`,
`UsersView.vue:335`), which is genuinely taller than one line. Clamping the cell would
have traded a visible height for invisible data loss.

If a table needs taller rows, `data-density` is the knob and `twoLine` already exists.
Reasoning recorded in `DataTable.vue` above `.dt-tr` so nobody re-derives it wrongly.

### Bonus, found while verifying #1: the last teal a customer saw on every page

`NavigationProgress.vue` is global chrome rendered above every route — the one teal surface
still visible on every navigation, converted views included. It was a four-stop teal
gradient plus a second hard-coded teal gradient for dark mode.

Now a solid `var(--brand)` bar at 32% width. One declaration covers both themes because the
token is already redefined per theme, so the two can no longer drift. The keyframe end
value is 313%, not 100%: `translateX` percentages resolve against the element's **own**
width, so a 32%-wide bar needs 100/32 to clear a full track.

Verified live: `background-image: none`, `background-color: rgb(181,85,31)`, byte-identical
to the resolved `--brand`.

Gate after all of the above: vue-tsc **0 errors** · june-lint clean across **68** files ·
`vite build` ok · vitest **1536/1536** across 220 files. Thin-fork invariant re-checked and
still empty (the build writes into `backend/internal/web/dist`, which is gitignored at
`.gitignore:102`).

## Phase 4 begins: part 17 Login

The first screen every customer sees, and the loudest unconverted surface left.

### The panel

`components/auth/dotCutField.ts` + `components/auth/AuthFieldPanel.vue`. The field
algorithm is **ported from part 17's live prototype, not reimplemented** — the look depends
on exact constants (lattice pitch, bore ratio, per-transition delay curves) and an
approximate reinvention reads as "dots on a background" instead of one surface.

Four rules the spec is emphatic about, all load-bearing:

- **The mark is a hole.** Nothing is drawn on top; every cell the glyph covers is removed
  and the flat panel shows through. Inferno has no mark yet, so the field punches one out
  rather than importing a logo the brand does not own.
- **The circles touch.** Pitch equals diameter exactly, so the concave diamonds between
  every four carry the texture.
- **A cell has no on and off.** Every radius between solid and ring is valid.
- **Still, then quick.** Nothing moves during the hold; the change is over in under a second.

"Full colour ships" — six vivid two-tone pairs, one per scene, cross-blended on the morph.
This is the one place in Inferno that breaks the single-accent rule and it earns it by
being the only screen with nothing else on it. That is also why they are literal hex rather
than June tokens: reference art, not theme. The panel does not respond to dark mode by
design.

Lifecycle is the part that makes an always-running canvas shippable: `prefers-reduced-motion`
gets one static frame, offscreen or hidden tab stops the loop rather than throttling it, and
fonts are awaited before the first raster because the mark is cut from the serif and
rasterising against a fallback punches the wrong silhouette. Below 900px the panel is
`display: none`, and since the run gate tests `width > 0` the loop stops entirely — no
animation cost on mobile.

Verified live: the cut-out genuinely punches cells. Replicating `rasterize`'s text branch in
the page produced 14 punched cells forming a centred vertical bar, with
`document.fonts.check('16px "Martina Plantijn"')` true. As part 17 predicts, at this cell
count the letter reads as a plain bar — which is exactly why it flags the letter as a
placeholder and constrains any commissioned mark to a solid silhouette ~40 cells wide.

### The layout, rewritten once for nine views

`AuthLayout.vue` was 88 lines of everything June bans — gradient orbs, blur, glass, a teal
grid. It is now the split screen: a fixed 560px form column and the panel taking the rest,
so the reading measure never stretches on a wide monitor. Nine views render through this
slot (login, register, forgot, reset, verify, four OAuth callbacks), so they all inherit it.

The form column is deliberately plainer than anything else in the library — no card, no
shadow, the form sits directly on the page — because a front door has exactly one job. The
wordmark **is** the logo: the site name set in Martina Plantijn, no tile and no monogram. An
operator-uploaded logo still wins, because that is their brand and not ours to override.

### REGRESSION I introduced and fixed: `.input` padding vs icon insets

Adding `.input { padding: 0 10px }` to the bridge beat Tailwind's `pl-11` on source order
(utilities are emitted inside `style.css`; the bridge loads after it), so **twelve views**
that inset an icon into the field had the icon land on top of the placeholder. Caught by
loading `/forgot-password`, not by any gate.

Fixed by setting horizontal padding per side and only when the view has not set its own:

```css
.input:not([class*='pl-']):not([class*='px-']) { padding-left: 10px; }
.input:not([class*='pr-']):not([class*='px-']) { padding-right: 10px; }
```

The `:not()` guards never fight the utility — they simply do not match when one is present.
Those insets are legitimate on admin search fields, so the padding yields to them rather
than the other way round.

Measured after the fix: `/forgot-password` icon ends at 34px with text starting at 44px;
login's email field 10/10, its password field 10 left and 36 right so `pr-9` still clears
the reveal button.

### NOT done, and why: the two-step form

Part 17 specifies email first, password second, so "a GitHub user never sees a password
field". That benefit requires an identity-lookup endpoint the backend does not have — and
an endpoint that reveals whether an address is registered is an account-enumeration hole,
the very thing the spec's own reset copy is written to avoid. Without the lookup, two steps
is pure added friction for every password user and delivers none of the stated gain, so the
screen stays single-step. Revisit if a lookup ever ships.

Also deferred: the five interrupt states (captcha, 2FA, agreement, wrong password, rate
limit) still render as they do today — captcha and agreement inline, 2FA as a modal. Part
17 wants all five in the same column so the screen never changes shape. Rate limiting does
not exist in the product at all ("needs building").

### Also fixed here

Sentence case on the two visible Title Case strings (`Welcome Back` → `Welcome back`,
`Sign In` → `Sign in`, ground rule 1); the decorative icon on the CTA; the CTA spinner,
which hard-coded `text-white` and so assumed `--primary-foreground` was light when it is a
token; and `.fade-enter-active`'s `transition: all`, which animates border-color (ground
rule 6) and is now the two properties it actually changes.

Gate: vue-tsc **0 errors** · june-lint clean across **71** files · `vite build` ok ·
vitest **1536/1536**. Verified in a real browser at 1440x900 light and dark, and at 820px
where the panel drops and the page has no horizontal overflow.

### Still owed on the auth flows

The other eight views through this layout are unconverted inside their slot — centred
headings, Title Case, teal links, icon-in-button. `/forgot-password` is the clearest
example. They inherit the new shell but their own contents are still phase-4 work.

## Login: matched to the reference, and the fork is named Inferno

The screen now matches part 17's mock. Three rounds of getting it wrong, each
because I built from prose instead of measuring the mock:

1. Form on the left. The prose says "the panel on the left" -- I shipped it mirrored.
2. `D_COLS = 42` ported as a design constant. It is tuned to the mock's 575px
   preview box (13.2px cells). Fixed columns on a full-bleed panel make each
   circle 2.2x too big. The invariant is CELL SIZE; columns derive from it.
3. The form's own content -- providers first, then a rule, then the address.

**Branding.** Upstream defaults to "Sub2API" in eight customer-visible places
(login wordmark, tab title, plaza nav, home, key usage, register, verify, legal).
All now read `PRODUCT_NAME` from `src/config/brand.ts`. An operator's configured
`site_name` still wins -- their brand is theirs. `title.spec.ts` asserts against
the constant rather than a literal, because a hard-coded name is exactly how this
drifts back to upstream's.

**The legal line is NOT the agreement documents.** Wiring it to them put
"服务条款and使用政策and支持的国家和地区" under an English sign-in form: those are
the compliance dialog's, backend-seeded, Chinese, and there are four. It is two
fixed links to `/legal/terms` and `/legal/privacy`, which is a real public route.

### Two panel bugs, both fixed

- **Height ratchet.** `height: 100%` does not resolve against a content-sized
  grid row, so the canvas used its ATTRIBUTE height as intrinsic height ->
  grew the row -> ResizeObserver measured it -> wrote back larger. Left the page
  1710px tall in a 1138px viewport. The field is now `position: absolute; inset: 0`,
  so it cannot contribute to the height that measures it. Verified 0 overflow
  across resizes.
- **Blank band during drag-resize.** Assigning `canvas.width` clears the bitmap.
  Deferring resize to rAF let the browser composite the enlarged box with a
  cleared bitmap first. ResizeObserver fires before paint, so `resize()` now runs
  synchronously in it and size + pixels land in the same frame.

### june-lint promises added

Swapping one brand literal pulls a whole unconverted view into lint scope. Six
views are listed in `TOUCHED_NOT_CONVERTED` for exactly that reason, each with
the phase that will convert it. Two genuine ground-rule-6 breaks (`transition: all`
in RegisterView and EmailVerifyView) were fixed rather than promised.

### Local dev instance

Google and GitHub OAuth are enabled with placeholder credentials and `site_name`
is "Inferno", so the login screen renders as designed. Provider buttons are
data-driven -- they only appear when the operator enables them.

## Phase 5 begins: the seven archetypes

Part 14 assigns every one of the 62 route-level views to one of seven archetypes,
and gives each a rule. That map is the phase-5 work plan; it is in the prototype's
`routeMap`, extracted below in summary:

| | archetype | routes | rule |
|---|---|---|---|
| A | Table page | 21 | TablePageLayout. Title, one-line summary, filter bar, table, pager. **Nothing else above the table.** |
| B | Dashboard | 4 | A verdict sentence, four numbers, one trend, then everything else. Never more than two charts above the fold. |
| C | Settings page | 7 | Groups of rows, one border per group, **no card per setting**. |
| D | Detail page | 6 | One subject: identity and state, then its numbers, then its history as a table. |
| E | Flow step | 8 | One question per screen, a step count, a way back. A flow step that scrolls is two steps. |
| F | Interstitial | 9 | A wait, a statement of what is waited for, and a real recovery path. Nine routes to **one component**. |
| G | Message page | 7 | A single readable column at `--content-max` with **no chrome of its own**. |

### Primitives built first, deliberately

66 views converted without shared page primitives is 66 different headers. These
land before any view does:

- `PageHeader` — title, one-line summary, actions beside the title (A/B/C/D)
- `StatStrip` — part 03's number strip: one border, dividers, no icon tiles, no
  shadows. `StatCard` was already the cell; this is the frame that makes four
  numbers read as one instrument instead of four floating cards.
- `SettingsGroup` + `SettingsRow` — one border per GROUP, hairlines between rows.
  The "no card per setting" half is load-bearing: a card around every toggle is a
  wall of boxes where nothing is grouped.
- `MessagePage` — one column at `--content-max`, no card, no border. A message
  page that draws a card around its own text is pretending to be a dashboard.
- `InterstitialState` — the three states part 14 specifies (waiting / attention /
  failed), collapsing nine routes to one component.

`TablePageLayout` already existed from part 04, so A needed nothing.

### june-lint had a hole: untracked files were never checked

`ourChangedFiles()` scoped the lint with `git diff`, which cannot see untracked
files. A brand-new component -- which is most of what a conversion adds -- escaped
the lint entirely until it happened to be staged. All six primitives above were
written and reported "clean across 87 files" while not being checked at all;
staging them moved the count to 93 and only then were they linted.

Fixed by unioning `git ls-files --others --exclude-standard` into the scope.

## PENDING — the live checklist (update this, do not let it go stale)

### Phase 5, by archetype (part 14's map)

- [ ] **B Dashboard** (4) — **`admin/DashboardView` DONE** (`8cd38acb` archetype,
      `1dd02115` tiles). Remaining: `user/DashboardView`, `admin/UsageView`,
      `admin/orders/AdminPaymentDashboardView`.
      - Fold is now: verdict sentence -> four `StatTile`s -> range -> trend ->
        (fold) -> `StatStrip` of four -> distribution, user trend, quick actions.
      - `StatTile` is the nested-card primitive: tray + inner card + a context
        line on the tray floor. **Its two radii are coupled** — outer = inner +
        inset (10 + 8 = 18). A test asserts it; changing one means changing all.
      - Tray fill is `color-mix(in oklch, var(--card) 95%, var(--foreground))`,
        NOT `--sidebar`. `--sidebar`'s dark value (L 0.14) is below `--card`'s
        (0.195), so it inverted wrongly in dark mode.
      - Icon-tile colour is an owner-approved ground-rule-5 exception scoped to
        this fold, bounded by: **no tone may reuse a state colour**. Reverting is
        `tone="brand"` on all four.
      - Verdict "needs attention" rule settled: `error_accounts` only (excludes
        cooldown and rate-limited, which self-heal).
      - **The two rows split by TIME, not by kind.** Top = today (accounts,
        requests, tokens, cost); bottom = standing/lifetime (keys, users, total
        tokens, avg response). An earlier "health vs inventory" split was a grid
        constraint dressed up as a principle, and it separated the three
        same-day activity numbers across a chart.
      - `average_duration_ms` has **no window** (rpm/tpm are 5-minute), so it is
        a lifetime mean and belongs in the lifetime row. Its context is
        `total_requests` — a mean is unreadable without its sample size.
      - Token tiles show **cache read share**, not the input/output split:
        `total_tokens` includes cache, so input+output accounted for under a
        tenth of the headline and looked like it should sum. Reads only —
        counting cache creation would flatter it.
      - Still open: add "Errors, last hour" via
        `/admin/ops/request-errors?pageSize=1` -> `total` (agreed "option 2 now,
        option 3 later"); a proper aggregate endpoint with a per-reason
        breakdown later. New users today: restore below the fold, or drop.
- [ ] **G Message** (7) — `MessagePage` built, unused. Quickest win.
- [ ] **C Settings** (7) — `SettingsGroup`/`SettingsRow` built, unused. Includes splitting
      `SettingsView` (12,621 lines) into 9 routes; part 14 wants a redirect from the old
      URL and a release note, since admin bookmarks break.
- [ ] **D Detail** (6) — needs a detail-header primitive (identity + state, then numbers,
      then history).
- [ ] **F Interstitial** (9) — `InterstitialState` built. Fold the 5 auth callbacks into it;
      they were converted individually with shared classes instead.
- [ ] **A Table pages** (21) — headings done via route meta. Interiors still
      legacy-but-bridged.

### Owed cross-file work (shapes recorded earlier in this file)

- [ ] `AccountsView` action-button collapse -> unblocks `stickyActionsColumn`
- [ ] 55 -> 9 quota collapse (`QuotaDimension`)
- [ ] Opt-in Grok probe column (must state "one billable request per visible row")
- [ ] Column-header window picker (`pinnedWindowKey` already accepted)
- [ ] ~40 raw checkboxes -> `Checkbox.vue` / `Radio.vue` (every `.setValue()` test breaks)
- [ ] `AccountsView.vue` wire up `buildGrokUsageRefreshKey` (ported 2026-08-15,
      unused) so Grok quota/billing cells refresh reactively like OpenAI's do

### Known gaps, not blocking

- [ ] `/legal/terms` and `/legal/privacy` 404 — the login legal line links there
- [ ] Backend Chinese: ~2,500 strings. Frontend is clean; backend errors still reach
      customers. Fix is a translation map at `pkg/response/response.go:60`, gateway/auth
      errors only, never editing source strings (`ops_error_logger.go` matches on them).
- [ ] Icon subsetting (860KB woff2, 5,503 glyphs, we use a fraction)
- [ ] `gh auth refresh -h github.com -s workflow` to land the sync Action (needs Saksham)
- [ ] Chart canvas pixels still unverified — no `<canvas>` mounts on an empty DB

### On the promises list (june-lint TOUCHED_NOT_CONVERTED)

`CreateAccountModal`, `EditAccountModal`, `ProxiesView`, `AccountsView`, `HomeView`,
`KeyUsageView`, `PlazaNavBar`, `EmailVerifyView`, `LegalDocumentView`, `SettingsView`,
`AnnouncementBell`, `UserDashboardQuickActions`. Each is a deferred check, not a waiver:
remove the line when the file is converted and the lint starts holding it to the rules.

## Upstream reconciliation log (cont'd)

### 2026-08-16 — fourth sync, rebase only, nothing to port

**124 commits behind.** Range `c204d33b0` (2026-08-15, previous sync's endpoint)
.. `baeac1f3d` (2026-08-16, new last-reviewed SHA).

**Rebase: clean.** All 120 of our commits (everything past the fork point from
`upstream/main`) replayed with zero conflicts.

**Gate 5 asserted post-rebase:** `check-divergence.sh` exits 0 — 19 files differ
against `git merge-base HEAD upstream/main`, all 19 declared (D1 avatar_seed, D2
English legal defaults, plus the undeclared-but-outside-scope `.gitignore`
line). No accidental backend drift.

**Frontend gate:** `june-lint` 845 violation(s) across 270 converted file(s) —
same 845 total as the pre-rebase baseline (verified by running the identical
lint against `pre-sync-backup` in a scratch worktree: 845 across 273). The file
count dropped by 3 (`src/constants/channel.ts`,
`src/utils/accountUsageRefresh.ts`, `src/views/admin/groupsImagePricing.ts`)
because those three are pure 2026-08-15 wholesale ports with zero customization
-- upstream did not touch them again this cycle, so they went byte-identical to
the mirror and dropped out of lint scope. Confirmed neither file carried any
violation before or after, so this is the file-count drop GOAL.md warns to
distrust, checked and cleared, not silently accepted.
`npx vue-tsc --noEmit` 0 errors. `npx vitest run` 228 files / 1603 tests green
(no drop from the 224/1588 baseline recorded in GOAL.md; the increase reflects
work landed since that baseline was taken, not anything from this sync).
`npx vite build` succeeded, only the pre-existing >500kB chunk warnings.

**Backend gate, run because upstream's own commits touched `backend/` during
the rebase:** `go build ./...` clean. `go test -tags unit ./internal/... ./ent/...`
all packages `ok` (including `internal/server`, which carries D2's golden
fixture in `api_contract_test.go`).

## Diffed for portable work -- found none

Per the runbook, diffed everything the port policy cares about between
`c204d33b0` and `upstream/main`:

- `frontend/src/{api,stores,composables,utils,types}` -- **zero files changed.**
- `frontend/src/{App.vue,main.ts,router,style.css,styles,index.html,tailwind.config.js}`
  -- **zero files changed.**
- `backend/internal/handler/dto`, `backend/internal/handler/admin` -- **zero
  files changed.** No admin/user-facing response-shape drift to check against
  converted components.
- `backend/internal/handler` as a whole -- one file touched,
  `openai_gateway_handler.go` (+ its tests), reworking Codex remote-compaction-v2
  path detection (`isBareOpenAIResponsesPath`, `isOpenAIRemoteCompactionV2Request`)
  and adding `openAIResponsesRequiredCapabilityForRequest`. Entirely inside the
  gateway/proxy request path (the AI traffic path itself), not an admin/user SPA
  endpoint -- same category as the seven files logged skipped-with-reason on
  2026-08-15. Nothing under `frontend/src/api` reads it. Not ported.
- `frontend/src` as a whole, restricted to non-ignored paths (excludes
  `components/`, `views/`, `features/` per the standing rule) -- **only the two
  files above.**

**The only frontend-relevant upstream commit this cycle** is `fce41e318`
("make Codex fingerprint convergence opt-in and cover passthrough"), which
touches five files: `CreateAccountModal.vue`, `EditAccountModal.vue`,
`BulkEditAccountModal.vue` (all under the ignored `components/account/` bucket
-- the June redesign replaces them, not a merge target) plus
`i18n/locales/{en,zh}/admin/accounts.ts`, changing the copy on three existing
keys (`codexFingerprintModeDesc`, `codexFingerprintOff`, `codexFingerprintSession`)
to describe the new opt-in default. **Checked and skipped, not merely ignored:**
grepped `inferno-frontend/src/i18n/locales/{en,zh}/admin/accounts.ts` and all
three account modals for `codexFingerprint` -- zero matches anywhere in our
tree. The feature was never ported past what these modals started with, so
there is no key to update and no orphaned string would result from skipping
this. Recorded so a future sync does not need to re-derive this.

## Skipped, carried forward unchanged from 2026-08-15

`backup.ts` (multi-part download shape), `admin/groups.ts` `getUsageSummary`
(dropped `timezone` param, new `yesterday_cost` field), and their skipped spec
file -- re-checked against `upstream/main` and **none of the three changed
since `c204d33b0`**, so the 2026-08-15 skip reasons still apply verbatim. Not
re-litigated.

## Unsure about / flagging for review

- **The PR diff will look enormous and is not.** This branch is a straight
  rebase of `inferno-redesign` onto 124 new upstream commits; every one of our
  120 commits was replayed with a new hash. `git status` reports "244 and 120
  different commits" against `origin/inferno-redesign` -- 124 new upstream
  commits plus 120 rewritten hashes on our side, none of it real drift, the
  same effect GOAL.md's gate-5 section documents for `check-divergence.sh`
  against a fresh ref. **The actual new content in this PR is exactly one
  commit: this log entry.** Recommend the reviewer fast-forward
  `inferno-redesign` to this branch's tip (gates are green end to end) rather
  than attempting a merge; a merge across two divergent rebase lines is exactly
  what stranded sync PRs #1-#3.
- Nothing else surfaced with a real judgement call this cycle -- every item
  above resolved cleanly to "unchanged, still applies" or "out of tree, not
  reachable." Flagging the absence of a judgement call explicitly, since a
  quiet cycle should not read the same as an unreviewed one.

**Last reviewed upstream SHA: `baeac1f3de21d37b129405f092ef86c24b3f203d`**
(2026-08-15 13:40:21 UTC, "chore: sync VERSION to 0.1.177 [skip ci]").

### 2026-08-27 — fifth sync, BLOCKED on pre-existing Gate 5 breach; rebase + partial port land regardless

**430 commits behind**, by far the largest gap of any reconcile so far (prior
syncs: 13, 124, 0, 124). Range `baeac1f3de21d37b129405f092ef86c24b3f203d` (old
last-reviewed) .. `efb46db0a960fdad94502b1c3a982a0051cf5245` (2026-08-26, new
last-reviewed).

**Procedural note:** the container's clone of this repo was shallow. A naive
`git rev-list --count HEAD..upstream/main` read **4936**, because commits
before the shallow boundary are grafted with no parents and so are not
reachable from `HEAD` locally even though `merge-base` still resolves through
them. `git fetch --unshallow` before trusting any commit count; after that the
real number was 430. Flagging this so the next run doesn't repeat the scare or,
worse, trust the inflated number in a PR body.

**Rebase: two known conflict sites, one new one, all resolved.**
- Root `.gitignore`, twice (upstream added `/plugins/` and, later,
  `!docs/superpowers/`; both resolved by keeping both sides — see GOAL.md's D3/D4).
- `Dockerfile`'s `GOLANG_IMAGE`: our commit `d08220563` ("align Docker Go
  toolchain", 1.26.5→1.26.6) no longer applies — upstream's own `cbe258fd1`
  already moved both `Dockerfile` and `backend/go.mod` to 1.27.0 first. Took
  upstream's side; our commit is now a no-op. **D6 in GOAL.md is the
  `NODE_OPTIONS` heap bump, a different line — unaffected, not re-checked this
  cycle.**

**Gate 4 (`go generate ./ent`), run because upstream changed two entity
schemas this cycle** (`615e6901e` channel-monitor quota-mode fields, `901a0439f`
CN-provider support): regenerated with zero diff in the generated files
themselves (`go.sum` picked up new indirect checksums for entc's own CLI
deps — `cobra`, `tablewriter`, etc. — from running with `-mod=mod`; harmless,
not a real dependency addition). Confirms the rebase's automatic merge of
`ent/` was already correct; D1's `avatar_seed` survived intact.

**Gate 5 (`check-divergence.sh`): FAILS, and it predates this sync.**
12 undeclared backend + frontend files differ from `git merge-base`:
`backend/internal/config/config.go`,
`backend/internal/service/{auth_email_binding,auth_oauth_email_flow,auth_service,balance_notify_service,content_moderation,domain_constants,payment_order_result_test,setting_features,setting_parse,setting_service_update_test,totp_service}.go`.
Traced to a single existing commit already on `inferno-redesign` before this
session touched anything — `4817289c4` / rebased to `02b817bf7`, subject
`fix: make Inferno the default product branding`. It replaces hardcoded
`"Sub2API"` fallback strings with a new `DefaultSiteName`/`DefaultSiteSubtitle`
pair (`domain_constants.go`) across auth emails, TOTP issuer, content
moderation, balance-notify defaults, and the WebAuthn RP display name — a
coherent extension of D2's "English legal-document defaults" theme, but never
added to GOAL.md's ledger or `check-divergence.sh`'s `DECLARED` array. Per the
hard rule, **not added to DECLARED by this session** — that is a product
decision, not a reconcile-time one. `payment_order.go` and `user_service.go`,
touched by the same commit, don't show as undeclared because they already fall
under D5 and D1 respectively; the 12 above are the genuinely new surface.
**Owner: decide whether to fold this into D2 (rename it, e.g. "Inferno
branding defaults") or revert it, then update GOAL.md + the script together.**

**Frontend gate, run after every port below:** `june-lint` 899 violation(s)
across 283 file(s) (started the cycle at 899/282 immediately post-rebase; the
one extra file is `ChannelsView.vue` and friends now differing from the mirror
because of the type-consistency fixes below — an increase, the safe direction).
`npx vue-tsc --noEmit` 0 errors, but only after fixing four real breaks (below).
`npx vitest run` 229 files / 1619 tests, **2 pre-existing failures, not caused
by this sync**: `siteLogoSanitization.spec.ts` expects `AppSidebar.vue`,
`HomeView.vue`, and `KeyUsageView.vue` to sanitize `site_logo` with
`sanitizeUrl(...)`; `AppSidebar.vue` no longer references `sanitizeUrl` or
`siteLogo` at all as of `0e60b2e48` ("fix(ui): remove sidebar brand mark"),
which replaced the sidebar's logo with a text-only site name. Confirmed via
`git show pre-sync-backup:...AppSidebar.vue` that this predates today's
rebase entirely. **Owner: decide whether the sidebar logo removal was
intentional (then fix the test) or a regression (then restore the logo +
sanitizer) — same shape as the already-open `AppSidebar.vue:114` avatar_url
sanitizer gap in the log above.** `npx vite build` succeeded, same pre-existing
>500kB chunk warnings as every prior cycle.

**Backend gate, run because upstream touched `backend/` extensively:**
`go build ./...` clean. `go test -tags unit ./internal/... ./ent/...` all
packages `ok`.

**Ported** (headless API/type/composable/store/constants/utils layer only, per
the standing rule to ignore `components/`, `views/`, `features/`):
- Wholesale copy (file untouched by us since the June vendor point, mirror's
  version taken as-is): `src/api/tokenRefresh.ts` (+ its spec — fixes a real
  token-refresh race: a stale "boundary timer jitter" heuristic could mistake
  a fresh token for a completed peer refresh), `src/api/admin/accounts.ts`,
  `src/api/admin/channelMonitor.ts`, `src/api/admin/channels.ts`,
  `src/api/channelMonitor.ts`, `src/api/modelPlaza.ts`,
  `src/constants/channelMonitor.ts`, `src/utils/featureFlags.ts`,
  `src/utils/platformColors.ts` (still on the legacy Tailwind palette, so no
  June-token trap here), `src/i18n/locales/zh/dashboard.ts`,
  `src/stores/__tests__/app.spec.ts`.
- Hand merge (file already customized by us): `src/types/index.ts` (new
  optional fields + `Provider`/`GroupPlatform`/`AccountPlatform` union
  extensions for antigravity/kimi/zhipu/deepseek — all additive), `src/api/admin/settings.ts`
  (`SCHEDULING_THRESHOLD_PLATFORMS` gains kimi/zhipu, `channel_monitor_show_quota`
  field), `src/composables/useModelWhitelist.ts` (+ spec — new grok-imagine and
  kimi model IDs; same "additive model list" shape as the 2026-08-15
  grok-4.6 precedent, no palette involved this time),
  `src/composables/useChannelMonitorFormat.ts` (**this one WAS the June-token
  trap**: upstream's diff is written against the pre-conversion Tailwind-rainbow
  version and adds 4 new providers with 4 new raw-Tailwind hues; our file is
  already on June tokens with exactly two buckets, brand-tint for first-party
  providers and neutral for everyone else. Ported the new `checkMode`/`quota`
  functions on their merits, but folded all 4 new providers into the existing
  neutral bucket instead of inventing 4 new hues — zero palette regression),
  `src/i18n/locales/en/dashboard.ts` (mirrored the zh additions; two lines
  needed a ground-rule-2 fix — upstream's copy used an en dash and an em dash,
  rewrote both to plain hyphens/wording before committing, confirmed lint
  count unchanged at 899 after the fix), `src/stores/app.ts`, `src/style.css`
  (`color-scheme: light`/`.dark { color-scheme: dark }`, a real native-control
  theming fix, unrelated to any feature flag).
- `dompurify` bumped `^3.3.1` → `^3.4.14` in `package.json` (+ a pnpm
  `overrides` floor) and the lockfile — upstream's own security bump, ported
  because we already lean on this library for our own `sanitizeUrl` avatar/logo
  work.
- Compile-fix-only changes forced by the above (no redesign work, no new
  product surface — these were TypeScript errors, not style choices):
  `src/components/admin/channel/types.ts` (`formIntervalsToAPI` now emits the
  4 new `null` multiplier fields `PricingInterval` requires),
  `src/components/admin/monitor/MonitorTemplateManagerDialog.vue` (provider
  count map gains the 4 new providers, all initialized to 0),
  `src/components/user/monitor/ProviderIcon.vue` (`PROVIDER_ICONS` changed
  from `Record<Provider, IconData>` to `Partial<...>` — we have no real
  brand-icon SVG paths for antigravity/kimi/zhipu/deepseek, and the component
  already had a graceful `iconInfo ?? null` → letter-avatar fallback; used it
  rather than inventing icon paths), `src/views/admin/ChannelsView.vue` (two
  object literals building `ChannelModelPricing` now set `time_pricing: null`,
  since the form has no time-pricing UI yet).

**Skipped, with reasons — the skips that matter this cycle:**
- **Everything CN-provider / plugin-management / channel-monitor-quota-mode
  shaped, at the UI layer.** Upstream added first-class Kimi/Zhipu/DeepSeek
  provider support, a plugin system, and quota-mode channel monitoring
  (linked-account quota snapshots instead of LLM probes) across ~15 backend
  handler files and a dozen frontend surfaces this cycle. None of it has a
  consuming view in `inferno-frontend` today — verified by grep before
  skipping, not assumed: zero references to `cnProviders`, `checkMode`,
  `linkedAccount`, or `PluginsView` anywhere under `src/components` or
  `src/views`. Backend handlers (`cn_provider_handler.go`, `plugin_handler.go`,
  `channel_monitor_handler.go` additions) were left entirely alone — untouched,
  not reverted, just not read into any client. Frontend: did NOT add the new
  `/admin/plugins` route (`router/index.ts`) since `PluginsView.vue` does not
  exist here and importing it would break the build; did NOT port
  `src/api/admin/{cnProviders,plugins}.ts`, `src/api/codex.ts`,
  `src/constants/platforms.ts`, `src/utils/codexCatalogConfig.ts` (all
  upstream-only files, no local equivalent to merge into); did NOT port the
  `plugins` i18n locale bundle or `admin.plugins.*`/`monitorAdmin.checkMode.*`
  hand-authored form copy in `admin/channels.ts` (time-based/multiplier
  pricing UI, monitor check-mode + linked-account picker) or `admin/accounts.ts`
  (CN-provider account-mode copy, shadow-account bulk-update messaging) — kept
  the type/API layer in sync (see Ported above) so a future UI pass has
  something to build against, but did not touch the locale files that only
  that unbuilt UI would read.
- **`admin/settings.ts` locale (en+zh), in full.** The diff bundles three
  unrelated things inside the single deferred `SettingsView.vue` surface
  (GOAL.md item 3, still awaiting an owner routing decision): removal of the
  entire Sora client/storage feature (~100 lines of now-dead keys upstream
  deleted), an `openaiFastPolicy` terminology rename ("whitelist" → "target
  models" — confirmed our copy still uses the old wording and old key names
  inside `SettingsView.vue`), and new Plugin Management / channel-monitor-quota
  settings copy (skipped per the point above). Untangling which of the ~106 deleted / 26
  added lines are safe to move independently of the SettingsView split is real
  work belonging to that project, not a mechanical port.
- **`admin/overview.ts`'s `concurrencyMin` → `concurrencyNonNegative` rename.**
  Real behavior change (0 becomes "unlimited" instead of being rejected) in
  `src/components/admin/user/UserEditModal.vue`, which is in the standing
  ignore-scope (`src/components/`). The old key is still called there, so the
  locale text was left as-is rather than adding an unreferenced new key next
  to a stale old one. Flagging for whoever converts that modal.
- **`admin/ops.ts`'s new `errorDetail.{upstreamStatus,rootCause,diagnosticPayloads,payloads.*}`
  keys.** `OpsErrorDetailModal.vue` — a *converted* component (GOAL.md item 2)
  — uses the `admin.ops.errorDetail.*` namespace extensively but has none of
  these four keys; confirmed by grep, not assumed. This is upstream adding new
  ops-diagnostic capability (root-cause classification, structured
  client/upstream diagnostic payloads) to a screen we've already redesigned.
  Porting the copy alone does nothing without new `<dl>` rows in the modal,
  which is real component + design work this reconcile is not positioned to
  do safely (GOAL.md requires rendering and measuring visual changes in a
  browser, not just typechecking). Flagged here as a concrete, scoped
  follow-up rather than silently dropped.
- `backup.ts` (multi-part download shape) and `admin/groups.ts`'s
  `getUsageSummary` (dropped `timezone`, new `yesterday_cost`) — re-checked
  against `upstream/main`, **unchanged since the 2026-08-15/08-16 skip**, same
  reasons still apply verbatim, not re-litigated.
- `channelMonitorV2.ts` (en+zh) — zero diff since last review, nothing to do.

**Unsure about / flagging for review:**
- This is the first reconcile cycle with a real, un-mechanical judgment call
  baked into a "Ported" item: `useChannelMonitorFormat.ts`'s new providers
  were deliberately given the *neutral* badge treatment instead of inventing
  purple/pink/indigo/teal hues, to match the file's already-established
  two-bucket palette. That's a design opinion, not just a merge — worth a
  second pair of eyes if/when the channel-monitor-quota UI actually gets
  built, since whoever builds it may want the 4 new providers visually
  distinguishable from each other, not just from "OpenAI/Anthropic/Gemini".
- Gate 5's breach (above) and the `siteLogoSanitization` test failure (above)
  are both **pre-existing and blocking**, not introduced by this sync. Opening
  this as a BLOCKED PR per the runbook's hard rule rather than silently
  merging past a red gate.
- The PR diff will look large for a reconcile (430 commits' worth of rebase,
  visible as every one of our commits getting a new hash) but the actual new
  content is: the rebase's 3 conflict resolutions, the ent regeneration
  no-op, the ~26-file frontend port above, and this log entry. Recommend
  fast-forwarding rather than merging, same as every prior cycle.

**New last reviewed upstream SHA: `efb46db0a960fdad94502b1c3a982a0051cf5245`**
(2026-08-26 15:49:38 +0800, "Merge pull request #5926 from
baryon/contrib/routed-codex-model-catalog").
