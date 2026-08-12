import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

/**
 * A deep-to-pale ramp on the brand hue, resolved to literal colours a canvas
 * can draw.
 *
 * DISTINCT FROM useChartTokens' `ramp`, on purpose. That one is CATEGORICAL:
 * eight steps that walk the hue (`calc(h + 44)`, `calc(h - 32)`) so no two
 * series look related. This one is SEQUENTIAL: one hue, lightness climbing and
 * chroma falling, so the steps read as an ordered series -- which is what a
 * donut of a single total needs, because its slices are parts of one quantity
 * and are drawn sorted by size.
 *
 * The progression mirrors the reference deck's blue ramp measured in OKLCH
 * (L 0.53 -> 0.84 while C 0.21 -> 0.07) transposed onto whatever --brand is
 * set to, so the ramp keeps its shape under a brand change instead of being
 * five frozen hex values.
 *
 * Resolution goes through a hidden probe and getComputedStyle, the same
 * mechanism useChartTokens uses and for the same reason: a canvas cannot read
 * `var()` or `oklch(from ...)`, so the browser's own cascade has to do the
 * maths and hand back an rgb() literal.
 */
const LIGHT = [
  'oklch(from var(--brand) 0.55 0.140 h)',
  'oklch(from var(--brand) 0.63 0.115 h)',
  'oklch(from var(--brand) 0.71 0.090 h)',
  'oklch(from var(--brand) 0.78 0.065 h)',
  'oklch(from var(--brand) 0.85 0.045 h)'
]

/* Dark keeps the same deep-to-pale direction, lifted overall: against a near
   black card the darkest step of the light ramp would read as a hole rather
   than as the heaviest slice. */
const DARK = [
  'oklch(from var(--brand) 0.62 0.145 h)',
  'oklch(from var(--brand) 0.69 0.120 h)',
  'oklch(from var(--brand) 0.76 0.095 h)',
  'oklch(from var(--brand) 0.82 0.070 h)',
  'oklch(from var(--brand) 0.88 0.050 h)'
]

/* Relative colour has been in every evergreen browser since 2023, but the
   fallback is not decorative: without it the ramp resolves to nothing and the
   canvas silently draws five invisible slices. color-mix reaches the same
   destination by a different road. */
const FALLBACK = [
  'var(--brand)',
  'color-mix(in oklch, var(--brand) 80%, var(--card))',
  'color-mix(in oklch, var(--brand) 60%, var(--card))',
  'color-mix(in oklch, var(--brand) 40%, var(--card))',
  'color-mix(in oklch, var(--brand) 24%, var(--card))'
]

let probeEl: HTMLSpanElement | null = null
function getProbe(): HTMLSpanElement {
  if (probeEl?.isConnected) return probeEl
  const el = document.createElement('span')
  el.setAttribute('aria-hidden', 'true')
  el.style.cssText =
    'position:fixed;left:-9999px;top:0;width:0;height:0;pointer-events:none;visibility:hidden'
  document.body.appendChild(el)
  probeEl = el
  return el
}

let supportsRelative: boolean | null = null
function relativeColorSupported(): boolean {
  if (supportsRelative === null) {
    supportsRelative =
      typeof CSS !== 'undefined' && typeof CSS.supports === 'function'
        ? CSS.supports('color', 'oklch(from red l c h)')
        : false
  }
  return supportsRelative
}

function resolve(expr: string): string {
  const probe = getProbe()
  probe.style.color = ''
  probe.style.color = expr
  return getComputedStyle(probe).color
}

export function useSequentialRamp(steps = 5): Ref<string[]> {
  const ramp = ref<string[]>([])

  function refresh() {
    const dark = document.documentElement.getAttribute('data-theme') === 'dark'
    const table = relativeColorSupported() ? (dark ? DARK : LIGHT) : FALLBACK
    ramp.value = table.slice(0, steps).map(resolve)
  }

  /* Re-resolve on a theme or brand change. --brand animates over 220ms
     (tokens/colors.css), so a single immediate read would freeze the ramp
     mid-transition; observing the attribute and re-resolving on the next frame
     lands after the swap. */
  let observer: MutationObserver | null = null

  onMounted(() => {
    refresh()
    observer = new MutationObserver(() => requestAnimationFrame(refresh))
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme', 'style', 'class']
    })
  })

  onBeforeUnmount(() => observer?.disconnect())

  return ramp
}
