<script setup lang="ts">
/**
 * A stacked bar chart whose bands are FIELDS of drifting square tiles rather
 * than solid blocks, on a canvas over a DOM plot.
 *
 * WHAT LIVES WHERE, AND WHY
 * Only the bars are canvas. The hatch, the gridlines, the axis and the labels
 * stay in the DOM -- they are static, they need to be selectable and
 * translatable, and redrawing them 60 times a second alongside the tiles would
 * be pure waste. The canvas sits over the plot and owns exactly the thing that
 * has to animate.
 *
 * THE MORPH REMEMBERS ITS LAST HEIGHT
 * Each band eases from the height it is CURRENTLY ON SCREEN to its new target,
 * never from zero -- so a period change grows the columns rather than
 * collapsing and rebuilding them. `drawn` is the map that makes that possible:
 * every frame writes back what it actually rendered, so the next change has a
 * real starting point even if it interrupts a morph mid-flight. Same principle
 * as the donut's displayed/from/target, expressed per band.
 *
 * Columns stagger by 5% of the duration each, which is what stops four
 * simultaneous grows from reading as one block sliding up.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export interface BarBand {
  key: string
  label: string
  value: number
  color: string
}

export interface BarColumn {
  key: string
  label: string
  bands: BarBand[]
}

export interface BarHover {
  column: number
  band: number | null
}

const props = defineProps<{
  columns: BarColumn[]
  /** Nice-rounded axis maximum, supplied by the parent so the DOM gridlines
   *  and the canvas bars cannot disagree about scale. */
  axisMax: number
  hovered: BarHover | null
  highlight: string
}>()

const emit = defineEmits<{ 'update:hovered': [value: BarHover | null] }>()

const MORPH_MS = 620
const STAGGER = 0.05
const BAND_GAP = 4
const MIN_BAND = 6

const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)

let ctx: CanvasRenderingContext2D | null = null
let width = 0
let height = 0
let raf: number | null = null
let running = false
let time = 0

/** Rendered height per band, keyed column:band. Written every frame so an
 *  interrupted morph still has a truthful starting point. */
const drawn = new Map<string, number>()
let morphFrom = new Map<string, number>()
let morphStart = 0
let morphing = false

const reduced =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = Math.min(1, Math.max(0, (x - edge0) / (edge1 - edge0)))
  return t * t * (3 - 2 * t)
}

function hash(x: number, y: number): number {
  const n = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453
  return n - Math.floor(n)
}

/** Target pixel height of every band, before any morph easing. */
const targets = computed(() => {
  const map = new Map<string, number>()
  if (props.axisMax <= 0 || height <= 0) return map
  props.columns.forEach((col, ci) => {
    col.bands.forEach((band, bi) => {
      const raw = (Math.max(0, band.value) / props.axisMax) * height
      /* A band with real traffic must never render as nothing: at this scale a
         1% band is sub-pixel, and a missing band with a populated legend row
         reads as a bug rather than as a small number. */
      map.set(`${ci}:${bi}`, band.value > 0 ? Math.max(MIN_BAND, raw) : 0)
    })
  })
  return map
})

/**
 * A rounded rect where each corner can differ.
 *
 * Every corner is rounded, including the ones between stacked bands -- the
 * spec is explicit that nothing is ever a hard square. Outer caps get more
 * (7 bottom, 8 top) so the bar reads as one object with softer ends than
 * joints. Radii are clamped to half the smaller side or a short band inverts.
 */
function roundedRect(
  c: CanvasRenderingContext2D,
  x: number, y: number, w: number, h: number,
  tl: number, tr: number, br: number, bl: number
) {
  const cap = Math.min(w, h) / 2
  const a = Math.min(tl, cap), b = Math.min(tr, cap)
  const d = Math.min(br, cap), e = Math.min(bl, cap)
  c.beginPath()
  c.moveTo(x + a, y)
  c.lineTo(x + w - b, y)
  c.quadraticCurveTo(x + w, y, x + w, y + b)
  c.lineTo(x + w, y + h - d)
  c.quadraticCurveTo(x + w, y + h, x + w - d, y + h)
  c.lineTo(x + e, y + h)
  c.quadraticCurveTo(x, y + h, x, y + h - e)
  c.lineTo(x, y + a)
  c.quadraticCurveTo(x, y, x + a, y)
  c.closePath()
}

function drawBand(
  c: CanvasRenderingContext2D,
  x: number, y: number, w: number, h: number,
  color: string, alpha: number, hot: boolean, isTop: boolean, isBottom: boolean
) {
  const cell = Math.max(3, Math.round(width / 200))

  c.save()
  roundedRect(c, x, y, w, h, isTop ? 8 : 5, isTop ? 8 : 5, isBottom ? 7 : 5, isBottom ? 7 : 5)
  if (hot) {
    c.save()
    c.strokeStyle = props.highlight
    c.lineWidth = 1.75
    c.shadowColor = props.highlight
    c.shadowBlur = 10
    c.stroke()
    c.restore()
  }
  c.clip()

  c.globalAlpha = alpha
  c.fillStyle = hot ? props.highlight : color
  c.beginPath()
  const base = hot ? 0.52 : 0.36
  for (let ty = y; ty <= y + h; ty += cell) {
    /* Tiles grow toward the TOP of each band, so a band reads as denser where
       it meets the next one and dissolves toward its own floor. */
    const fall = smoothstep(y + h, y, ty)
    for (let tx = x; tx <= x + w; tx += cell) {
      const flow =
        Math.sin(tx * 0.05 + time) +
        Math.sin(ty * 0.045 - time * 0.9) +
        Math.sin((tx + ty) * 0.026 + time * 1.2)
      const wave = smoothstep(-1.5, 1.5, flow)
      const jitter = hash(tx, ty)
      const size = cell * (base + 0.34 * fall + 0.26 * wave) * (0.76 + 0.44 * jitter)
      const half = size / 2
      c.rect(tx - half, ty - half, size, size)
    }
  }
  c.fill()
  c.restore()
}

/** Column geometry, shared by the renderer and the hit test so the two cannot
 *  drift apart. */
function columnBox(index: number) {
  const count = Math.max(1, props.columns.length)
  const colW = width / count
  const barW = Math.min(colW * 0.62, 54)
  return { colW, barW, x0: index * colW + (colW - barW) / 2 }
}

function draw() {
  const c = ctx
  if (!c || width <= 0 || height <= 0) return
  c.clearRect(0, 0, width, height)

  let progress = 1
  if (morphing) {
    progress = Math.min(1, (performance.now() - morphStart) / MORPH_MS)
    if (progress >= 1) morphing = false
  }

  const n = Math.max(1, props.columns.length)
  const hover = props.hovered

  props.columns.forEach((col, ci) => {
    const { barW, x0 } = columnBox(ci)
    /* Each column's ease starts 5% of the duration after the last, and the
       window is rescaled so the final column still completes at t=1. */
    const span = 1 - (n - 1) * STAGGER
    const local = Math.min(1, Math.max(0, (progress - ci * STAGGER) / span))
    const eased = 1 - Math.pow(1 - local, 3)

    let cursor = height
    col.bands.forEach((band, bi) => {
      const key = `${ci}:${bi}`
      const to = targets.value.get(key) ?? 0
      const from = morphFrom.get(key) ?? 0
      const h = morphing ? from + (to - from) * eased : to
      drawn.set(key, h)
      if (h <= 0) return

      const y = cursor - h
      cursor = y - BAND_GAP

      const isHotColumn = hover?.column === ci
      const isHotBand = isHotColumn && hover?.band === bi
      const alpha = !hover ? 1 : isHotBand ? 1 : isHotColumn ? 0.48 : 0.3

      drawBand(
        c, x0, y, barW, h,
        band.color, alpha, isHotBand,
        bi === col.bands.length - 1, bi === 0
      )
    })
  })
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
  const w = box.clientWidth
  const h = box.clientHeight
  if (w <= 0 || h <= 0) return
  width = w
  height = h
  const dpr = Math.min(window.devicePixelRatio || 1, 2)
  el.width = Math.ceil(w * dpr)
  el.height = Math.ceil(h * dpr)
  ctx = el.getContext('2d')
  if (!ctx) return
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  draw()
}

/** Column by x, band by walking the stack from the floor up. A point inside
 *  the column but outside the bar itself resolves to the column with no band,
 *  which is what lets the header read "at {column}" without a band. */
function hitTest(event: PointerEvent) {
  const el = canvas.value
  if (!el || width <= 0) return
  const rect = el.getBoundingClientRect()
  const x = event.clientX - rect.left
  const y = event.clientY - rect.top
  const count = Math.max(1, props.columns.length)
  const ci = Math.floor((x / width) * count)
  if (ci < 0 || ci >= props.columns.length) {
    emit('update:hovered', null)
    return
  }
  const { barW, x0 } = columnBox(ci)
  if (x < x0 || x > x0 + barW) {
    emit('update:hovered', { column: ci, band: null })
    return
  }
  let cursor = height
  for (let bi = 0; bi < props.columns[ci].bands.length; bi += 1) {
    const h = drawn.get(`${ci}:${bi}`) ?? 0
    if (h > 0 && y >= cursor - h && y <= cursor) {
      emit('update:hovered', { column: ci, band: bi })
      return
    }
    cursor = cursor - h - BAND_GAP
  }
  emit('update:hovered', { column: ci, band: null })
}

watch(
  () => [props.columns, props.axisMax] as const,
  () => {
    if (reduced) {
      morphing = false
      draw()
      return
    }
    morphFrom = new Map(drawn) // whatever is on screen NOW
    morphStart = performance.now()
    morphing = true
  },
  { deep: true }
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
  observer = new IntersectionObserver((entries) => {
    if (entries.some((e) => e.isIntersecting)) start()
    else stop()
  })
  if (host.value) observer.observe(host.value)
  document.addEventListener('visibilitychange', onVisibility)
  /* First paint grows from zero, which is the one time `from` should not be
     the previous frame. */
  morphFrom = new Map()
  morphStart = performance.now()
  morphing = !reduced
})

onBeforeUnmount(() => {
  stop()
  observer?.disconnect()
  resizeObserver?.disconnect()
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div ref="host" class="bars">
    <canvas
      ref="canvas"
      class="bars__canvas"
      @pointermove="hitTest"
      @pointerleave="emit('update:hovered', null)"
    />
  </div>
</template>

<style scoped>
.bars {
  position: absolute;
  inset: 0;
}

.bars__canvas {
  display: block;
  width: 100%;
  height: 100%;
  touch-action: none;
}
</style>
