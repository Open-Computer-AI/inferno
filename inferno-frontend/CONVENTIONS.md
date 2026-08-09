# June component conventions

Every agent building a component reads this first. It exists so parallel work
produces one dialect instead of eleven that each pass the lint.

## Where things are

| | |
|---|---|
| Prototypes | `/Users/saksham/Downloads/design_handoff_inferno_v2/prototypes/Library NN Name.dc.html` |
| Handoff docs | same folder, `README.md`, `01-TOKENS.md`, `03-OPEN-CALLS.md` |
| Tokens (spec, never edit) | `src/design-system/tokens/*.css` |
| Inferno-local tokens | `src/design-system/inferno.css` |
| Components | `src/components/common/` |
| Reference copy of upstream | `../frontend/` — pristine, never edit, use to diff |

## The rules that are not negotiable

1. **Never edit anything in `src/design-system/tokens/` or `components/`.** Those
   are copied byte-identical from the design bundle and get re-synced. A local
   edit turns every future sync into a merge. Need a token that does not exist?
   Add it to `src/design-system/inferno.css`.
2. **Never edit `../frontend/`.** It is the pristine upstream reference.
3. **Only touch the files you were assigned.** Never `SpecimenView.vue`,
   `INFERNO-BUILD.md`, `router/index.ts`, `main.ts`, `tailwind.config.js`,
   `june-lint.mjs`, or another agent's component. The orchestrator owns those.
4. **Preserve the existing public API when rewriting an existing component.**
   Props, emits, slot names and `defineExpose` stay identical unless the
   prototype's props table explicitly changes them. This is a presentation
   rewrite; call sites must keep working untouched. Read the current file first.
5. **Plain CSS in `<style scoped>`, on tokens. No Tailwind utilities.**
6. **Never hardcode user-facing English, and never edit `src/i18n/`.** The
   orchestrator owns the locale files. If your component needs new copy:
   - use `t('some.sensible.key')` as if the key already existed, and
   - list every key you invented, with its English value, in your final report.
   The orchestrator adds them in one pass, so there are no write collisions.
   Reusing an existing key is better than inventing one — grep `src/i18n/`
   first. Values must be sentence case with no en or em dashes (ground rules
   1 and 2), including any string you are merely moving.

## The ten ground rules (README, "a violation is a review failure")

1. Sentence case everywhere. Never ALL CAPS, never `text-transform: uppercase`.
2. No en or em dashes in user-facing copy. Hyphen, or the word "to" for a range.
3. Two font weights only: `400` and `var(--fw-medium)` (600). The family has two
   faces with `font-synthesis: none`, so 500 renders as 400 and 700 as 600.
4. Font sizes only from `--fs-2xs|xs|sm|md|lg|xl|2xl|display`. Never hand-code a
   `font-size` **except on icon glyphs**, which are fixed chrome and must not
   scale with `--font-scale`. Annotate those:
   `font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */`
5. Colour is spent, not sprayed. Hovers, rows, nav and menus are neutral grey
   (`--sidebar-accent`). Colour encodes **state**, never category.
6. **Never transition `border-color`.** Borders are static chrome. Hover changes
   background only. Never `transition: all`. Write `transition: background var(--motion-hover)`.
7. No gradients, glass, glow, mesh, or shimmer-on-skeleton.
8. No emoji.
9. Cards have no shadow: 1px `--border-subtle`, `--r-lg`, `--card` fill.
10. Respect `prefers-reduced-motion: reduce` on every animation.

## Interaction rules, system-wide

- **Hover** background only, transparent to `--sidebar-accent`, `var(--motion-hover)`.
  Icons lift `--muted-foreground` to `--foreground`.
- **Press** no scale, no shrink. Solid darkens: `color-mix(in oklch, var(--primary) 92%, var(--foreground))`.
  Active goes to 84%.
- **Focus** `outline: none; box-shadow: 0 0 0 3px var(--focus-ring)`, plus
  `border-color: var(--ring-focus)` on bordered controls. Warm grey, never blue.
- **Disabled** `opacity: .55; cursor: not-allowed`. A gated control never shows a
  validation error under it; explain the gate instead.
- **Durations** 100 / 160 / 240ms only, via `--t-fast` / `--t-med` / `--t-slow`.
  Curves `--ease-out`, `--ease-in-out`, `--ease-spring`, `--ease-pop`.
- **Loading** flat static `--surface-subtle` skeleton bars. No sweep, no pulse.

## Tokens you will actually use

```
Surface   --background --card --surface-subtle --popover --muted
Text      --foreground --body-copy --muted-foreground --on-solid
          --primary --primary-foreground
Line      --border --border-subtle --input --popover-border
State     --destructive --destructive-soft --success
          --s2a-attn --s2a-attn-bg --s2a-attn-soft   (attention, Inferno-local)
Accent    --brand --brand-tint --brand-line --warm-strong
Focus     --ring-focus (1px border)  --focus-ring (3px halo)
Radius    --r-xs 4 badges · --r-sm 6 icon buttons · --r-md 8 rows/inputs/popovers
          --r-lg 10 cards · --r-xl 14 dialogs · --r-pill
Height    --s2a-h-xs 28 · --s2a-h-sm 32 · --s2a-h-md 36 · --s2a-h-lg 40
          (June's own scale is --control-xs..xl = 22/26/28/32/36)
Type      --fs-2xs 10 · --fs-xs 11 · --fs-sm 12 · --fs-md 13 (body)
          --fs-lg 14 · --fs-xl 16 · --fs-2xl 20 · --fs-display 30
Family    --font-sans (UI) --font-serif (display moments only) --font-mono
          (only where content IS code: keys, ids, hosts, model names)
Motion    --motion-hover  = var(--t-fast) var(--ease-out)
Shadow    --shadow-sm/md/... pure elevation, never a ring. Compose hairlines separately.
```

Icons are Hugeicons stroke rounded, self-hosted:
`<i class="hgi-stroke hgi-NAME" aria-hidden="true" />`, always explicitly sized
(12-16px in chrome, 18-26px in empty states). Verify a glyph exists before using
it: `grep '\.hgi-NAME\b' src/design-system/hugeicons.css`.

## House style, from the components already built

Read `src/components/common/Button.vue` and `Input.vue` before starting. They
set the pattern. In particular:

- Variants and sizes are **data attributes**, not classes:
  `<button :data-variant="variant" :data-size="size">` styled as
  `.btn2[data-variant='solid']`. Not `:class="[...]"`.
- Block class names are short and component-scoped: `.fld`, `.fld__input`,
  `.fld__msg`. BEM-ish, no global names.
- Height and inline padding are set together per size, so a size is one decision.
- Boolean visual state goes through an attribute that also carries meaning to
  assistive tech (`aria-invalid`, `aria-busy`, `aria-disabled`) rather than a
  class that only paints.
- Every non-obvious decision gets a short comment saying **why**, quoting the
  prototype's migration note where it drove the choice. Do not narrate what the
  code does.

## Definition of done

A component is not done until all of these pass. Run them yourself; do not
report success without the output.

```sh
cd /Users/saksham/OpenComputerV2/inferno/inferno-frontend
node scripts/june-lint.mjs          # must be clean for your files
npx vue-tsc --noEmit -p tsconfig.json   # must be 0 errors
```

Then report back:
1. Files you created or modified, full paths.
2. Any measurement the prototype printed next to the component, and the value
   you implemented, so the orchestrator can assert it in the browser.
3. Any prop, emit or slot you changed, and why the prototype required it.
4. Anything in `03-OPEN-CALLS.md` you hit: implement the reversible option,
   leave `TODO(open-call-N)`, and say so. Do not decide it.
5. Anything the prototype specifies that you could not implement, and why.
   An honest gap is useful; a silent one is not.
