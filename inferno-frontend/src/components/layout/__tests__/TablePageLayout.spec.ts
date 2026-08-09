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
