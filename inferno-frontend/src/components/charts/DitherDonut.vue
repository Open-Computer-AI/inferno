<script setup lang="ts">
/**
 * A rounded annular donut on a canvas, filled with a drifting square-tile
 * dither rather than flat colour, whose slice angles MORPH from wherever they
 * currently sit when the data changes.
 *
 * THE SIGNATURE MOVE, AND WHY IT IS BUILT THE WAY IT IS
 * Three share arrays are kept: displayed, from, target. On a data change the
 * component snapshots whatever is on screen RIGHT NOW -- mid-flight included
 * -- as `from`, sets the new shares as `target`, and eases between them. That
 * snapshot is the whole trick. Easing from the previous *target* instead would
 * make a fast second click jump backwards to a position never rendered; easing
 * from zero would make every period change an explosive redraw. Reading the
 * displayed value means an interrupted morph continues from the frame the user
 * actually saw.
 *
 * The curve is the manual exponential-out `1 - 2^(-10t)`, applied identically
 * every frame, on its own clock concurrent with the tile drift. The two must
 * not share a loop: the drift runs forever and the morph runs for 500ms, and
 * folding them together would either stall the drift between morphs or restart
 * the morph on every drift frame.
 *
 * GEOMETRY IS FIXED AT 200x200 LOGICAL UNITS
 * Every radius, gap and corner below is expressed in that space and the canvas
 * is scaled to fit its host, so the ring is resolution-independent and the
 * numbers here read as the spec wrote them. devicePixelRatio is folded into
 * the same transform and capped at 2 -- a 3x phone would trip the backing
 * store to 9x the pixels for no visible gain, and this canvas repaints every
 * frame.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export interface DonutSlice {
  key: string
  label: string
  value: number
  color: string
}

const props = defineProps<{
  slices: DonutSlice[]
  /** Index of the exploded slice, or null. Owned by the parent so the legend
   *  and the ring drive the same state. */
  hovered: number | null
}>()

const emit = defineEmits<{ 'update:hovered': [value: number | null] }>()

/* --- the spec's geometry, verbatim ------------------------------------- */
const LOGICAL = 200
const CX = 100
const CY = 100
const R_OUTER = 86
const R_INNER = 55
const START = -Math.PI / 2 // 12 o'clock
const GAP = 0.07 // radians, split half onto each edge
const CORNER = 6
const CELL = 4.6
const MORPH_MS = 500

const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)

const shares = computed(() => {
  const total = props.slices.reduce((sum, s) => sum + Math.max(0, s.value), 0)
  if (total <= 0) return props.slices.map(() => 0)
  return props.slices.map((s) => Math.max(0, s.value) / total)
})

/* displayed / from / target -- see the note at the top. */
let displayed: number[] = []
let from: number[] = []
let target: number[] = []
let morphStart = 0
let morphing = false

let time = 0
let raf: number | null = null
let ctx: CanvasRenderingContext2D | null = null
let cssSize = 0
let running = false

const reduced =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

/* --- small maths -------------------------------------------------------- */
function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = Math.min(1, Math.max(0, (x - edge0) / (edge1 - edge0)))
  return t * t * (3 - 2 * t)
}

/** Deterministic per-tile noise. A hash rather than Math.random because the
 *  jitter has to be STABLE across frames -- re-rolling it every frame makes
 *  the field boil instead of drift. */
function hash(x: number, y: number): number {
  const n = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453
  return n - Math.floor(n)
}

function withAlpha(color: string, alpha: number): string {
  const m = color.match(/rgba?\(([^)]+)\)/)
  if (!m) return color
  const [r, g, b] = m[1].split(',').map((v) => parseFloat(v))
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

const px = (angle: number, radius: number) => CX + Math.cos(angle) * radius
const py = (angle: number, radius: number) => CY + Math.sin(angle) * radius

/**
 * A wedge of an annulus with all four corners rounded.
 *
 * The corner radius is clamped twice, and both clamps are load-bearing. Against
 * the band width, or a thin ring swallows its own corners and the path
 * self-intersects into a bowtie. And against the angular span at the INNER
 * radius -- the inner arc is shorter than the outer for the same angle, so a
 * corner that fits at r=86 can overlap its opposite number at r=55 and invert
 * the slice.
 */
function wedgePath(c: CanvasRenderingContext2D, a0: number, a1: number) {
  const span = a1 - a0
  const r = Math.max(
    0,
    Math.min(CORNER, (R_OUTER - R_INNER) / 2, (span * R_INNER) / 2.2)
  )
  const dOuter = Math.min(r / R_OUTER, span / 2)
  const dInner = Math.min(r / R_INNER, span / 2)

  c.beginPath()
  c.arc(CX, CY, R_OUTER, a0 + dOuter, a1 - dOuter)
  c.quadraticCurveTo(
    px(a1, R_OUTER), py(a1, R_OUTER),
    px(a1, R_OUTER - r), py(a1, R_OUTER - r)
  )
  c.lineTo(px(a1, R_INNER + r), py(a1, R_INNER + r))
  c.quadraticCurveTo(
    px(a1, R_INNER), py(a1, R_INNER),
    px(a1 - dInner, R_INNER), py(a1 - dInner, R_INNER)
  )
  c.arc(CX, CY, R_INNER, a1 - dInner, a0 + dInner, true)
  c.quadraticCurveTo(
    px(a0, R_INNER), py(a0, R_INNER),
    px(a0, R_INNER + r), py(a0, R_INNER + r)
  )
  c.lineTo(px(a0, R_OUTER - r), py(a0, R_OUTER - r))
  c.quadraticCurveTo(
    px(a0, R_OUTER), py(a0, R_OUTER),
    px(a0 + dOuter, R_OUTER), py(a0 + dOuter, R_OUTER)
  )
  c.closePath()
}

/** The tile field for one wedge, accumulated into a single path and filled
 *  once. Per-rect fills would be thousands of draw calls a frame. */
function fillTiles(
  c: CanvasRenderingContext2D,
  a0: number,
  a1: number,
  isHot: boolean
) {
  const base = isHot ? 0.46 : 0.34
  c.beginPath()
  for (let y = CY - R_OUTER; y <= CY + R_OUTER; y += CELL) {
    for (let x = CX - R_OUTER; x <= CX + R_OUTER; x += CELL) {
      const dx = x - CX
      const dy = y - CY
      const dist = Math.hypot(dx, dy)
      if (dist < R_INNER - CELL || dist > R_OUTER + CELL) continue

      /* Normalise the tile's angle into [a0, a0+2pi) before the range test.
         Slices routinely straddle the -pi/+pi branch cut atan2 returns, and
         comparing raw angles there drops a wedge's entire left half. */
      let angle = Math.atan2(dy, dx)
      while (angle < a0) angle += Math.PI * 2
      while (angle > a0 + Math.PI * 2) angle -= Math.PI * 2
      if (angle > a1) continue

      const fullness = 0.62 + 0.38 * smoothstep(R_INNER, R_OUTER, dist)
      const flow =
        Math.sin(x * 0.055 + time) +
        Math.sin(y * 0.041 - time * 0.8) +
        Math.sin((x + y) * 0.028 + time * 1.3)
      const wave = smoothstep(-1.4, 1.4, flow)
      const jitter = hash(x, y)

      const size = CELL * (base + 0.36 * fullness + 0.26 * wave) * (0.78 + 0.42 * jitter)
      const half = size / 2
      c.rect(x - half, y - half, size, size)
    }
  }
  c.fill()
}

function draw() {
  const c = ctx
  if (!c) return

  if (morphing) {
    const progress = Math.min(1, (performance.now() - morphStart) / MORPH_MS)
    const eased = 1 - Math.pow(2, -10 * progress)
    displayed = target.map((v, i) => from[i] + (v - from[i]) * eased)
    if (progress >= 1) {
      displayed = target.slice()
      morphing = false
    }
  }

  c.clearRect(0, 0, LOGICAL, LOGICAL)

  const anyHot = props.hovered !== null
  let angle = START

  for (let i = 0; i < displayed.length; i += 1) {
    const sweep = displayed[i] * Math.PI * 2
    /*
     * The gap yields to thin slices instead of eating them.
     *
     * A fixed 0.07 rad gap is 1.1% of the circle, so any slice at or below
     * that vanishes outright -- and a vanished slice is worse than a cramped
     * one, because the legend still lists it with a value. Output is 1.2% of
     * our token mix, right on that line. Scaling the gap to a third of the
     * sweep keeps every slice visible at any share while leaving the gap at
     * its full width wherever there is room for it.
     */
    const gap = Math.min(GAP, sweep * 0.34)
    if (sweep <= 0.004) {
      angle += sweep
      continue
    }
    const a0 = angle + gap / 2
    const a1 = angle + sweep - gap / 2
    const isHot = props.hovered === i

    c.save()

    if (isHot) {
      /* Explode along the wedge's own midpoint, applied BEFORE the clip so the
         whole slice -- path, tiles and shadow -- moves as one piece. */
      const mid = (a0 + a1) / 2
      c.translate(Math.cos(mid) * 6, Math.sin(mid) * 6)
      c.shadowColor = withAlpha(props.slices[i].color, 0.55)
      c.shadowBlur = 5
    }

    wedgePath(c, a0, a1)
    c.clip()

    c.globalAlpha = isHot ? 1 : anyHot ? 0.3 * 0.72 : 0.72
    c.fillStyle = props.slices[i].color
    fillTiles(c, a0, a1, isHot)

    c.restore()
    angle += sweep
  }
}

function frame() {
  if (!running) return
  if (!reduced) time += 0.02
  draw()
  raf = requestAnimationFrame(frame)
}

function start() {
  if (running) return
  running = true
  raf = requestAnimationFrame(frame)
}

function stop() {
  running = false
  if (raf !== null) cancelAnimationFrame(raf)
  raf = null
}

function resize() {
  const el = canvas.value
  const box = host.value
  if (!el || !box) return
  const size = box.clientWidth
  if (size <= 0) return
  cssSize = size
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  el.width = Math.ceil(size * dpr)
  el.height = Math.ceil(size * dpr)
  el.style.height = `${size}px`
  ctx = el.getContext('2d')
  if (!ctx) return
  /* One transform folds the logical-to-CSS scale and the pixel ratio together,
     so every coordinate below stays in the 200x200 space. */
  const scale = (size / LOGICAL) * dpr
  ctx.setTransform(scale, 0, 0, scale, 0, 0)
  draw()
}

function hitTest(event: PointerEvent) {
  const el = canvas.value
  if (!el || cssSize <= 0) return
  const rect = el.getBoundingClientRect()
  const x = ((event.clientX - rect.left) / cssSize) * LOGICAL
  const y = ((event.clientY - rect.top) / cssSize) * LOGICAL
  const dist = Math.hypot(x - CX, y - CY)
  if (dist < R_INNER || dist > R_OUTER) {
    emit('update:hovered', null)
    return
  }
  let angle = Math.atan2(y - CY, x - CX)
  while (angle < START) angle += Math.PI * 2

  let walk = START
  for (let i = 0; i < displayed.length; i += 1) {
    const sweep = displayed[i] * Math.PI * 2
    if (angle >= walk && angle <= walk + sweep) {
      emit('update:hovered', i)
      return
    }
    walk += sweep
  }
  emit('update:hovered', null)
}

watch(
  shares,
  (next) => {
    if (displayed.length !== next.length) {
      displayed = next.slice()
      target = next.slice()
      from = next.slice()
      draw()
      return
    }
    from = displayed.slice() // whatever is on screen NOW, mid-flight included
    target = next.slice()
    if (reduced) {
      displayed = target.slice()
      morphing = false
      draw()
      return
    }
    morphStart = performance.now()
    morphing = true
  },
  { immediate: true, deep: true }
)

let observer: IntersectionObserver | null = null
let resizeObserver: ResizeObserver | null = null

function onVisibility() {
  if (document.hidden) stop()
  else if (host.value) start()
}

onMounted(() => {
  resize()
  resizeObserver = new ResizeObserver(resize)
  if (host.value) resizeObserver.observe(host.value)

  /* The drift runs forever by design, so it must not run while nobody can see
     it. Off-screen and background-tab both stop the loop outright rather than
     throttling it -- this canvas repaints every frame, and a dashboard is a
     page people leave open. */
  observer = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) start()
    else stop()
  })
  if (host.value) observer.observe(host.value)

  document.addEventListener('visibilitychange', onVisibility)
})

onBeforeUnmount(() => {
  stop()
  observer?.disconnect()
  resizeObserver?.disconnect()
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div ref="host" class="donut">
    <canvas
      ref="canvas"
      class="donut__canvas"
      @pointermove="hitTest"
      @pointerleave="emit('update:hovered', null)"
    />
  </div>
</template>

<style scoped>
.donut {
  width: 100%;
  max-width: 200px;
}

.donut__canvas {
  display: block;
  width: 100%;
  touch-action: none;
}
</style>
