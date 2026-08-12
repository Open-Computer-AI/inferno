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

describe('StatTile delta sentiment', () => {
  /* Mirrors the component's resolver. */
  const sentiment = (
    direction: 'up' | 'down',
    polarity: 'higher-is-better' | 'lower-is-better' | 'neutral'
  ) => {
    if (polarity === 'neutral') return 'neutral'
    const good = polarity === 'higher-is-better' ? direction === 'up' : direction === 'down'
    return good ? 'good' : 'bad'
  }

  /*
   * The pill is painted by SENTIMENT, not by direction. Colouring by direction
   * would break ground rule 5 -- up and down are not states -- while good and
   * bad are exactly what state means.
   */
  it('inverts for a metric where lower is better', () => {
    expect(sentiment('up', 'higher-is-better')).toBe('good')
    expect(sentiment('up', 'lower-is-better')).toBe('bad')
    expect(sentiment('down', 'higher-is-better')).toBe('bad')
    expect(sentiment('down', 'lower-is-better')).toBe('good')
  })

  it('stays toneless when neither direction is an outcome', () => {
    expect(sentiment('up', 'neutral')).toBe('neutral')
    expect(sentiment('down', 'neutral')).toBe('neutral')
  })

  it('paints the pill from the state tokens, mixed toward the surface', () => {
    // Mixed rather than a fixed rgba so the fill tracks the surface in both
    // themes instead of washing out on white or glaring on near-black.
    expect(tileSource).toContain("color-mix(in oklch, var(--success) 12%, var(--card))")
    expect(tileSource).toContain("color-mix(in oklch, var(--destructive) 12%, var(--card))")
  })
})

describe('StatTile context-line emphasis', () => {
  /*
   * Mirrors the component's regex. Kept as a local copy on purpose: the point
   * is to pin the BEHAVIOUR against real locale strings from both languages,
   * and importing from an SFC is not possible without mounting it.
   */
  const firstValue = (text: string) => text.match(/[$€£¥]?\d[\d,.]*%?/)?.[0] ?? ''

  it('emphasises the value in every context line the dashboard ships', () => {
    expect(firstValue('87% served from cache')).toBe('87%')
    expect(firstValue('$9.71 standard, $1.41 saved')).toBe('$9.71')
    expect(firstValue('across 4,240,000 requests')).toBe('4,240,000')
    expect(firstValue('1,204 per minute over the last 5')).toBe('1,204')
    expect(firstValue('8 active')).toBe('8')
    expect(firstValue('3 need attention')).toBe('3')
  })

  it('takes only the FIRST number, so one line never has two bold spans', () => {
    // Bolding "4 new today" as well as "12 active" would leave neither
    // emphasised; the first number is the finding, the rest is detail.
    expect(firstValue('12 active, 4 new today')).toBe('12')
    expect(firstValue('1,204 per minute over the last 5')).not.toBe('5')
  })

  it('leaves a line with no number entirely unemphasised', () => {
    expect(firstValue('All serving normally')).toBe('')
    expect(firstValue('Billed at the standard rate')).toBe('')
    expect(firstValue('None connected yet')).toBe('')
  })

  it('lands on the right token in Chinese, where the number is not leading', () => {
    expect(firstValue('87% 来自缓存')).toBe('87%')
    expect(firstValue('活跃 12，今日新增 4')).toBe('12')
    expect(firstValue('基于 950 次请求')).toBe('950')
  })

  it('renders the split as text nodes, never as injected markup', () => {
    // Locale strings are translator-authored. Rendering them through v-html
    // for the sake of one bold span would make every future translation an
    // injection surface.
    // Matches the directive in use (`v-html="`), not the bare word, which
    // also appears in the comment explaining why it is avoided.
    expect(tileSource).not.toMatch(/v-html\s*=/)
    expect(tileSource).toContain('contextParts.before')
    expect(tileSource).toContain('contextParts.after')
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

  /*
   * Wrong is loud, fine is quiet. 'attention' colours the whole line so a
   * broken account is the only thing on the fold that catches the eye unread;
   * 'good' tints ONLY the glyph and leaves its text muted, so a healthy
   * dashboard is not eight lines of green competing with the icon tiles above.
   * Making them symmetric would defeat the point of marking anything.
   */
  it('keeps the good/attention treatment asymmetric', () => {
    const foot = tileSource.slice(tileSource.indexOf(".tile__foot[data-tone='muted']"))
    // attention paints the line itself...
    expect(foot).toMatch(/\.tile__foot\[data-tone='attention'\]\s*\{\s*color:\s*var\(--s2a-attn\)/)
    // ...good paints only the glyph, and its text stays muted.
    expect(foot).toMatch(
      /\.tile__foot\[data-tone='good'\]\s*\{\s*color:\s*var\(--muted-foreground\)/
    )
    expect(foot).toMatch(
      /\.tile__foot\[data-tone='good'\]\s+\.tile__foot-icon\s*\{\s*color:\s*var\(--success\)/
    )
  })

  it('defaults the context line to muted, because most lines are facts', () => {
    // "1,204 per minute", "8 active", "across 4.24M requests" have no bad
    // version to contrast against. A mark on every line marks nothing.
    expect(tileSource).toContain("contextTone: 'muted'")
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
