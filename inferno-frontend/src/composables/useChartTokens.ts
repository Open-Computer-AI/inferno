import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'

/**
 * useChartTokens — the Chart.js <-> June token bridge, part 09.
 *
 * Chart.js paints to a <canvas>. A canvas has no cascade: `var(--brand)`
 * inside a `borderColor` is meaningless to the 2D context, which needs a
 * literal `rgb()`/`#hex` string. Every chart that wants a June token has to
 * resolve it once via `getComputedStyle`, and re-resolve it whenever the
 * theme or brand preset changes, because `[data-theme="dark"]` (and a
 * `data-brand` swap) redefine the custom properties on `:root` and the
 * canvas never repaints itself.
 *
 * main.ts mirrors Tailwind's `.dark` class onto `[data-theme]` on `<html>`
 * via a MutationObserver; this composable observes that same attribute, plus
 * `data-brand` (01-TOKENS documents five live presets, clay/rose/sage/ocean/
 * plum, even though only clay is wired up today), and republishes a plain
 * object of resolved literal colors on `tokens`. A chart reads `tokens.*`
 * inside its own `computed()` chart data/options; vue-chartjs deep-watches
 * both props and calls `chart.update()` on change, so nothing here has to
 * hold an imperative handle to any chart instance.
 *
 * --brand is `@property`-registered (`syntax:"<color>"`) and colors.css
 * declares `transition:--brand 220ms var(--ease-out)` on `:root`, so a brand
 * swap (not a light/dark swap, which snaps) interpolates. Reading
 * getComputedStyle the instant the attribute changes can land mid
 * interpolation. This resolves three times on a brand change: immediately
 * (a same-frame value, better than leaving the chart on the outgoing brand
 * for 220ms), on the `transitionend` event for the `--brand` property
 * (the precise settle point), and once more on a 240ms fallback timer in
 * case `transitionend` never fires (e.g. `prefers-reduced-motion` removes
 * the transition at the token level, per motion.css, in which case the
 * immediate resolve already has the final value and the extra two calls are
 * harmless no-op re-reads). No animation is attempted in between; the last
 * resolve simply wins.
 */

export interface ChartTokens {
  /**
   * The 8 step categorical ramp, --sr-1 to --sr-8 in order (part 09, section
   * 01), derived from the live --brand hue. A chart series is the one
   * legitimate categorical use of colour in June -- this is that ramp,
   * resolved to literals a canvas can draw. Chroma stays under 0.12 so no
   * step shouts, and none of the eight collides with the attention or
   * destructive colour.
   */
  ramp: string[]
  brand: string
  brandTint: string
  foreground: string
  bodyCopy: string
  mutedForeground: string
  border: string
  borderSubtle: string
  card: string
  surfaceSubtle: string
  attention: string
  success: string
  destructive: string
}

// --- the --sr- ramp -----------------------------------------------------
//
// 01-TOKENS/part 09 documents --sr-1..8 as Inferno-local custom properties,
// which would normally belong in inferno.css (CONVENTIONS.md rule 1). This
// task's owned-file list does not include inferno.css, so rather than widen
// scope onto a file another pass owns, the ramp is computed here from the
// exact formulas the prototype's own <style> block declares (both the
// oklch(from ...) relative-colour version and its color-mix fallback), and
// resolved against the live --brand the same way any other expression below
// is resolved. Functionally identical to a real token; textually, it is not
// one yet. Follow-up: promote these into inferno.css as real --sr-1..8
// custom properties so a non-chart consumer (e.g. a legend swatch built in
// plain CSS) can use the same ramp without a JS resolver.
const RAMP_FALLBACK = [
  'var(--brand)',
  'color-mix(in oklch, var(--brand) 54%, var(--card))',
  'color-mix(in oklch, var(--brand) 78%, var(--foreground))',
  'color-mix(in oklch, var(--brand) 32%, var(--card))',
  'color-mix(in oklch, var(--brand) 62%, var(--muted))',
  'color-mix(in oklch, var(--brand) 52%, var(--foreground))',
  'color-mix(in oklch, var(--brand) 18%, var(--card))',
  'var(--muted)'
]

const RAMP_RELATIVE_LIGHT = [
  'oklch(from var(--brand) 0.58 0.115 h)',
  'oklch(from var(--brand) 0.69 0.068 calc(h + 44))',
  'oklch(from var(--brand) 0.50 0.090 calc(h - 32))',
  'oklch(from var(--brand) 0.76 0.042 calc(h + 16))',
  'oklch(from var(--brand) 0.62 0.058 calc(h - 60))',
  'oklch(from var(--brand) 0.45 0.048 calc(h + 68))',
  'oklch(from var(--brand) 0.71 0.026 calc(h + 6))',
  'oklch(from var(--brand) 0.81 0.013 h)'
]

const RAMP_RELATIVE_DARK = [
  'oklch(from var(--brand) 0.66 0.115 h)',
  'oklch(from var(--brand) 0.77 0.068 calc(h + 44))',
  'oklch(from var(--brand) 0.58 0.090 calc(h - 32))',
  'oklch(from var(--brand) 0.84 0.042 calc(h + 16))',
  'oklch(from var(--brand) 0.70 0.058 calc(h - 60))',
  'oklch(from var(--brand) 0.53 0.048 calc(h + 68))',
  'oklch(from var(--brand) 0.79 0.026 calc(h + 6))',
  'oklch(from var(--brand) 0.62 0.013 h)'
]

let relativeColorSupport: boolean | null = null
function supportsRelativeColor(): boolean {
  if (relativeColorSupport === null) {
    relativeColorSupport =
      typeof CSS !== 'undefined' && typeof CSS.supports === 'function'
        ? CSS.supports('color', 'oklch(from red l c h)')
        : false
  }
  return relativeColorSupport
}

// --- resolving CSS colour expressions against the live :root -------------
//
// A single hidden probe element, reused rather than recreated, so resolving
// a dozen tokens on a theme change does not touch layout a dozen times.
let probeEl: HTMLSpanElement | null = null
function getProbe(): HTMLSpanElement {
  if (probeEl?.isConnected) return probeEl
  const el = document.createElement('span')
  el.setAttribute('aria-hidden', 'true')
  el.style.position = 'fixed'
  el.style.left = '-9999px'
  el.style.top = '0'
  el.style.width = '0'
  el.style.height = '0'
  el.style.pointerEvents = 'none'
  el.style.visibility = 'hidden'
  document.body.appendChild(el)
  probeEl = el
  return el
}

/** Resolves any CSS colour expression -- a var(), a color-mix(), a relative
 *  oklch(from ...) -- against the live :root, by letting the browser's own
 *  cascade do the maths and reading the computed result back as a literal
 *  rgb()/rgba() string a canvas 2D context can use directly. */
function resolveColorExpr(expr: string): string {
  const probe = getProbe()
  probe.style.color = ''
  probe.style.color = expr
  return getComputedStyle(probe).color
}

function resolveVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

function resolveRamp(isDark: boolean): string[] {
  const table = supportsRelativeColor()
    ? isDark
      ? RAMP_RELATIVE_DARK
      : RAMP_RELATIVE_LIGHT
    : RAMP_FALLBACK
  return table.map(resolveColorExpr)
}

function resolveAll(): ChartTokens {
  const isDark = document.documentElement.getAttribute('data-theme') === 'dark'
  return {
    ramp: resolveRamp(isDark),
    brand: resolveVar('--brand'),
    brandTint: resolveVar('--brand-tint'),
    foreground: resolveVar('--foreground'),
    bodyCopy: resolveVar('--body-copy'),
    mutedForeground: resolveVar('--muted-foreground'),
    border: resolveVar('--border'),
    borderSubtle: resolveVar('--border-subtle'),
    card: resolveVar('--card'),
    surfaceSubtle: resolveVar('--surface-subtle'),
    attention: resolveVar('--s2a-attn'),
    success: resolveVar('--success'),
    destructive: resolveVar('--destructive')
  }
}

/** A neutral, SSR-safe placeholder so `tokens` is never read before the
 *  component has mounted (charts render client-only anyway, but this keeps
 *  the type honest without an `| null`). Overwritten on mount before any
 *  chart's first paint. */
function placeholderTokens(): ChartTokens {
  return {
    ramp: ['#b5551f', '#b5551f', '#b5551f', '#b5551f', '#b5551f', '#b5551f', '#b5551f', '#b5551f'],
    brand: '#b5551f',
    brandTint: '#b5551f',
    foreground: '#272424',
    bodyCopy: '#3a3535',
    mutedForeground: '#8f8582',
    border: '#ddd4d0',
    borderSubtle: '#e7e0dd',
    card: '#ffffff',
    surfaceSubtle: '#f2ede9',
    attention: '#b98426',
    success: '#3f7d4a',
    destructive: '#b6432f'
  }
}

/**
 * @returns `tokens` -- a reactive, always-current object of literal colors.
 *   Read `tokens.ramp[i]` / `tokens.brand` / etc. straight inside a
 *   `computed()` that builds Chart.js `data`/`options`; vue-chartjs deep
 *   watches both props and updates the canvas whenever they change, so nothing
 *   further is needed to repaint on a theme or brand change.
 * @returns `prefersReducedMotion` -- reactive flag for
 *   `animation: prefersReducedMotion ? false : {...}` in a chart's options
 *   (ground rule 10; Chart.js does not read the CSS media query itself).
 */
export function useChartTokens() {
  const tokens = reactive<ChartTokens>(
    typeof document === 'undefined' ? placeholderTokens() : resolveAll()
  )
  const prefersReducedMotion = ref(
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )

  const refresh = () => Object.assign(tokens, resolveAll())

  let observer: MutationObserver | undefined
  let settleTimer: ReturnType<typeof setTimeout> | undefined
  let motionQuery: MediaQueryList | undefined
  let onMotionChange: ((e: MediaQueryListEvent) => void) | undefined
  let onBrandTransitionEnd: ((e: TransitionEvent) => void) | undefined

  onMounted(() => {
    refresh()

    observer = new MutationObserver(() => {
      refresh()
      // --brand/--brand-wash interpolate over 220ms (colors.css); catch the
      // settled value once the transition (if any) has finished.
      clearTimeout(settleTimer)
      settleTimer = setTimeout(refresh, 240)
    })
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme', 'data-brand']
    })

    onBrandTransitionEnd = (e: TransitionEvent) => {
      if (e.propertyName === '--brand' || e.propertyName === '--brand-wash') refresh()
    }
    document.documentElement.addEventListener('transitionend', onBrandTransitionEnd)

    motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    onMotionChange = (e) => {
      prefersReducedMotion.value = e.matches
    }
    if (typeof motionQuery.addEventListener === 'function') {
      motionQuery.addEventListener('change', onMotionChange)
    } else {
      // Safari < 14
      motionQuery.addListener(onMotionChange)
    }
  })

  onBeforeUnmount(() => {
    observer?.disconnect()
    clearTimeout(settleTimer)
    if (onBrandTransitionEnd) {
      document.documentElement.removeEventListener('transitionend', onBrandTransitionEnd)
    }
    if (motionQuery && onMotionChange) {
      if (typeof motionQuery.removeEventListener === 'function') {
        motionQuery.removeEventListener('change', onMotionChange)
      } else {
        motionQuery.removeListener(onMotionChange)
      }
    }
  })

  return { tokens, prefersReducedMotion, refresh }
}
