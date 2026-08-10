import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../TablePageLayout.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('TablePageLayout responsive table scrolling', () => {
  // Old design: TablePageLayout reached into DataTable's internals with
  // `:deep(.table-wrapper)` and was responsible for keeping `overflow-x-auto` set
  // (not `overflow-visible`, which would silently disable horizontal scrolling) even
  // in `.mobile-mode`. The migration note for this rewrite says the opposite is now
  // true: "The scroll containment and sticky-column support live in DataTable.vue
  // itself (unchanged, per the migration note); this card only has to give it a
  // bounded flex-1/min-height:0 box to fill." So the guarantee -- a table's
  // horizontal scroll is never disabled in mobile mode -- now has to hold for a
  // different reason: TablePageLayout must not reach into `.table-wrapper` at all
  // (DataTable owns its own unconditional `overflow-x/y: auto`, see DataTable.vue),
  // and TablePageLayout's own box must never clip that content out from under it.

  it('no longer reaches into `.table-wrapper` -- DataTable owns its own scroll container now', () => {
    expect(componentSource).not.toMatch(/:deep\(\s*\.table-wrapper\s*\)/)
  })

  it('does not clip the table area in mobile mode, so DataTable\'s own scroll container stays usable', () => {
    const bodyBlocks = Array.from(componentSource.matchAll(/([^{}]*\.tpl__body[^{}]*)\{([^{}]*)\}/g))
    const cardBlocks = Array.from(componentSource.matchAll(/([^{}]*\.tpl__card[^{}]*)\{([^{}]*)\}/g))

    expect(bodyBlocks.length).toBeGreaterThan(0)
    expect(cardBlocks.length).toBeGreaterThan(0)

    const desktopBodyBlock = bodyBlocks.find(([selector]) => !selector.includes('[data-mobile]'))
    const mobileBodyBlock = bodyBlocks.find(([selector]) => selector.includes('[data-mobile]'))
    const mobileCardBlock = cardBlocks.find(([selector]) => selector.includes('[data-mobile]'))

    // Desktop: the card/body bound DataTable to a fixed box; DataTable scrolls inside it.
    expect(desktopBodyBlock?.[2]).toContain('overflow: hidden')

    // Mobile: DataTable swaps to a stacked card list that must grow with its content
    // (see DataTable.vue's own mobile-breakpoint comment), so nothing in the layout
    // may clip it -- overflow: visible, never hidden, on both the body and the card
    // wrapping it.
    expect(mobileBodyBlock?.[2]).toContain('overflow: visible')
    expect(mobileBodyBlock?.[2]).not.toContain('overflow: hidden')
    expect(mobileCardBlock?.[2]).toContain('overflow: visible')
    expect(mobileCardBlock?.[2]).not.toContain('overflow: hidden')
  })
})

describe('TablePageLayout width opt-out', () => {
  // Ruling: --content-max (controls.css) is defined under "Reading columns" -- prose
  // and forms, where a narrow measure aids reading. A dense admin table is not prose,
  // and the prototype's own table demo uses max-width:1080px, not the token, so
  // letterboxing every table view at the reading measure is a regression, not fidelity
  // to the spec. `width` lets a page opt out; default stays 'reading' so none of the
  // fifteen existing call sites (which pass neither prop today) change shape.

  it("defaults `width` to 'wide', because every consumer of this layout is a table", () => {
    // Flipped from 'reading'. --content-max is 760px (880 above 1440) and
    // 01-TOKENS defines it under "Reading columns" -- prose and forms. Part 14
    // says the same from the other side: a dense admin table is not a reading
    // column. Defaulting to 'reading' squeezed a 7-column grid into 880px and
    // left a third of the screen empty beside it on every one of the 21 table
    // routes. 'reading' remains available for a table that is genuinely prose.
    expect(componentSource).toMatch(/withDefaults\(defineProps<Props>\(\),\s*\{[\s\S]*?width:\s*'wide'/)
  })

  it("renders the root element's width as a data attribute, June's variant-as-attribute convention", () => {
    expect(componentSource).toMatch(/:data-width="width"/)
  })

  it("'reading' constrains the page to --content-max", () => {
    const tplBlocks = Array.from(componentSource.matchAll(/(^|\})\s*(\.tpl)\s*\{([^{}]*)\}/gm))
    const baseTplBlock = tplBlocks.find(([, , selector]) => selector === '.tpl')

    expect(baseTplBlock?.[3]).toContain('max-width: var(--content-max)')
  })

  it("'wide' drops the cap so the table fills the shell's inset card", () => {
    const wideBlocks = Array.from(
      componentSource.matchAll(/\.tpl\[data-width=['"]wide['"]\]\s*\{([^{}]*)\}/g)
    )

    expect(wideBlocks.length).toBeGreaterThan(0)
    expect(wideBlocks[0][1]).toContain('max-width: none')
  })
})
