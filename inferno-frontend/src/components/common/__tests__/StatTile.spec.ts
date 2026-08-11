import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const tileSource = readFileSync(resolve(here, '../StatTile.vue'), 'utf8')
const radiusTokens = readFileSync(
  resolve(here, '../../../design-system/tokens/radius.css'),
  'utf8'
)

function radius(token: string): number {
  const match = radiusTokens.match(new RegExp(`--${token}\\s*:\\s*(\\d+)px`))
  if (!match) throw new Error(`radius token --${token} not found`)
  return Number(match[1])
}

function declaredPx(prop: string): number {
  const match = tileSource.match(new RegExp(`--tile-inset:\\s*(\\d+)px`))
  if (prop !== 'inset' || !match) throw new Error('tile inset not found')
  return Number(match[1])
}

describe('StatTile nested-card geometry', () => {
  /*
   * Concentric rounded rectangles only look nested when their curves stay
   * parallel, which requires outer_radius === inner_radius + gap. Violating it
   * makes the visible gap pinch at each corner even though the padding is
   * uniform -- a defect that reads as "slightly cheap" without an obvious
   * cause, so it survives review easily. Asserting it means a future radius
   * retune cannot break the nesting silently.
   */
  it('keeps the tray radius equal to the inner radius plus the inset', () => {
    const inset = declaredPx('inset')
    expect(radius('r-lg') + inset).toBe(radius('r-2xl'))
  })

  it('uses those exact tokens rather than hard-coded pixel radii', () => {
    expect(tileSource).toContain('border-radius: var(--r-2xl)')
    expect(tileSource).toContain('border-radius: var(--r-lg)')
    expect(tileSource).toContain('padding: var(--tile-inset)')
  })
})

describe('StatTile surface colours', () => {
  /*
   * The tray has to step AWAY from the page surface, which is --card: darker in
   * light, lighter in dark. --sidebar only satisfies the first (its dark value
   * is L 0.14 against --card's 0.195), so a tray painted with it went darker
   * than the page in dark mode and read as a hole punched in the card. Mixing
   * toward --foreground inverts automatically.
   */
  it('derives the tray from --card and --foreground so it inverts per theme', () => {
    expect(tileSource).toContain('color-mix(in oklch, var(--card) 95%, var(--foreground))')
    expect(tileSource).not.toContain('background: var(--sidebar)')
  })

  it('never paints an icon tile with a state colour', () => {
    // An --s2a-attn tile sits amber on a healthy dashboard: a false alarm that
    // never clears, and it devalues the amber the verdict line uses for real.
    //
    // Scoped to the icon tone rules only. `.tile__foot[data-tone='attention']`
    // further down DOES take --s2a-attn, and correctly so: the foot restates
    // the verdict's evidence, which is real state rather than category.
    const iconBlock = tileSource.slice(
      tileSource.indexOf('.tile__icon['),
      tileSource.indexOf('.tile__label')
    )
    expect(iconBlock).not.toContain('var(--s2a-attn)')
    expect(iconBlock).not.toContain('var(--destructive)')
    expect(iconBlock).not.toContain('var(--success)')
  })
})
