<script setup lang="ts">
/**
 * An area chart drawn as a field of square tiles rather than a filled shape,
 * with a cursor-follow glow and a scrubber that springs on both axes.
 *
 * THE FIELD IS NOT A FILL WITH TEXTURE ON IT
 * Every cell in the grid paints a faint rest tile; cells under the curve paint
 * a second, larger, saturated one whose size falls off with depth. So the
 * "area" is an emergent property of which cells light up, not a path that gets
 * a pattern applied. That is what lets a range change reflow the shape
 * continuously -- the curve and the vertical scale are both interpolated and
 * the field is rebuilt from the blend every frame, so nothing remounts and the
 * axis re-zooms rather than cutting.
 *
 * ONE SPRING CONFIG ON BOTH SCRUBBER AXES
 * The marker's x and y share a single spring (650/42/0.5). Two configs, or a
 * tween on one axis, and the marker leaves the curve during travel -- it
 * arrives horizontally before it arrives vertically and visibly cuts the
 * corner. Same stiffness on both is what keeps it glued to the line.
 *
 * The glow buffer rises fast and decays slow (22% / 5% per frame) because the
 * asymmetry is what makes it read as a trail rather than as a spotlight: equal
 * rates look like a hard circle following the pointer.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useSpringValue } from '@/composables/useSpringValue'

const props = defineProps<{
  values: number[]
  labels: string[]
  /** Saturated tile colour, resolved to a literal by the parent. */
  color: string
  /** The faint always-on tile. */
  restColor: string
  markerColor: string
}>()

const TOP = 0.16 // headroom above the tallest point
const MORPH_MS = 460
const SPARK_RATE = 0.06

const host = ref<HTMLElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)

let ctx: CanvasRenderingContext2D | null = null
let width = 0
let height = 0
let raf: number | null = null
let running = false
let time = 0

/* The curve currently on screen, plus where it is heading. Interpolating both
   the values AND the max is what re-zooms the axis smoothly; blending values
   against a fixed max would make the shape reflow while the scale jumped. */
let shown: number[] = []
let fromValues: number[] = []
let fromMax = 1
let morphStart = 0
let morphing = false

let glow: Float32Array | null = null
let glowCols = 0
let glowRows = 0
let pointer: { x: number; y: number } | null = null
let smoothPointer: { x: number; y: number } | null = null
let pointerSpeed = 0

const reduced =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-reduced-motion: reduce)').matches

const targetMax = computed(() => Math.max(1, ...props.values))

function smoothstep(edge0: number, edge1: number, x: number): number {
  const t = Math.min(1, Math.max(0, (x - edge0) / (edge1 - edge0)))
  return t * t * (3 - 2 * t)
}

function hash(x: number, y: number): number {
  const n = Math.sin(x * 12.9898 + y * 78.233) * 43758.5453
  return n - Math.floor(n)
}

/** The curve at an arbitrary x fraction, eased between samples rather than
 *  linearly: at 90 days a cell column falls between points often enough that
 *  linear interpolation shows as visible faceting along the top edge. */
function sampleCurve(series: number[], f: number): number {
  if (series.length === 0) return 0
  if (series.length === 1) return series[0]
  const pos = f * (series.length - 1)
  const i = Math.min(series.length - 2, Math.floor(pos))
  const t = smoothstep(0, 1, pos - i)
  return series[i] + (series[i + 1] - series[i]) * t
}

/* --- the scrubber ------------------------------------------------------- */
const SCRUB = { stiffness: 650, damping: 42, mass: 0.5 }
const markerX = useSpringValue(50, SCRUB)
const markerY = useSpringValue(50, SCRUB)
const scrubIndex = ref<number | null>(null)

const scrubLabel = computed(() =>
  scrubIndex.value != null ? props.labels[scrubIndex.value] ?? '' : ''
)
const scrubValue = computed(() =>
  scrubIndex.value != null ? props.values[scrubIndex.value] ?? 0 : 0
)

function onScrub(event: PointerEvent) {
  const el = host.value
  if (!el || props.values.length === 0) return
  const rect = el.getBoundingClientRect()
  const f = Math.min(1, Math.max(0, (event.clientX - rect.left) / rect.width))
  const n = props.values.length
  const idx = n > 1 ? Math.round(f * (n - 1)) : 0
  scrubIndex.value = idx
  const max = Math.max(1, ...props.values)
  markerX.set(n > 1 ? (idx / (n - 1)) * 100 : 50)
  /* Same TOP headroom the tiles use, so the marker sits on the drawn edge
     rather than on a curve only the maths knows about. */
  markerY.set((TOP + (1 - props.values[idx] / max) * (1 - TOP)) * 100)
}

function endScrub() {
  scrubIndex.value = null
}

/* --- rendering ---------------------------------------------------------- */
function draw() {
  const c = ctx
  if (!c || width <= 0 || height <= 0) return

  let series = props.values
  let max = targetMax.value

  if (morphing) {
    const progress = Math.min(1, (performance.now() - morphStart) / MORPH_MS)
    if (progress >= 1) {
      morphing = false
    } else {
      /* Blend by FRACTION, not index: the two ranges have different lengths,
         so pairing element 0 with element 0 would stretch 7 days onto 90 and
         the shape would slide sideways instead of reflowing in place. */
      const steps = Math.max(props.values.length, fromValues.length)
      series = new Array(steps)
      for (let i = 0; i < steps; i += 1) {
        const f = steps > 1 ? i / (steps - 1) : 0
        const a = sampleCurve(fromValues, f)
        const b = sampleCurve(props.values, f)
        series[i] = a + (b - a) * progress
      }
      max = fromMax + (targetMax.value - fromMax) * progress
    }
  }
  shown = series
  max = Math.max(1, max)

  c.clearRect(0, 0, width, height)

  const cell = Math.max(3, Math.round(width / 180))
  const cols = Math.ceil(width / cell)
  const rows = Math.ceil(height / cell)

  if (!glow || glowCols !== cols || glowRows !== rows) {
    glow = new Float32Array(cols * rows)
    glowCols = cols
    glowRows = rows
  }

  /* Pointer is smoothed toward the raw position rather than used directly, so
     the lit region trails the cursor instead of snapping to it. */
  if (pointer) {
    if (!smoothPointer) smoothPointer = { ...pointer }
    const dx = pointer.x - smoothPointer.x
    const dy = pointer.y - smoothPointer.y
    pointerSpeed = Math.min(1, Math.hypot(dx, dy) / 40)
    smoothPointer.x += dx * 0.45
    smoothPointer.y += dy * 0.45
  }
  const radius = height * (0.24 + 0.26 * pointerSpeed)

  for (let cx = 0; cx < cols; cx += 1) {
    const x = cx * cell + cell / 2
    const f = cols > 1 ? cx / (cols - 1) : 0
    const value = sampleCurve(series, f)
    const norm = Math.min(1, Math.max(0, value / max))
    const curveY = height * (TOP + (1 - norm) * (1 - TOP))

    for (let cy = 0; cy < rows; cy += 1) {
      const y = cy * cell + cell / 2

      // Every cell carries the faint rest tile, inside the area or not.
      const restSize = cell * 0.3
      c.fillStyle = props.restColor
      c.fillRect(x - restSize / 2, y - restSize / 2, restSize, restSize)

      if (y < curveY) continue

      const depth = (y - curveY) / Math.max(1, height - curveY)
      const nearness = 0.42 + 0.58 * (1 - depth)
      // A slow band travelling top to bottom, +/-7%.
      const shimmer = 1 + 0.07 * Math.sin(y * 0.06 - time * 1.4)

      let lit = 0
      const gi = cy * cols + cx
      if (glow) {
        let target = 0
        if (smoothPointer) {
          const d = Math.hypot(x - smoothPointer.x, y - smoothPointer.y)
          target = Math.max(0, 1 - d / radius)
        }
        /* Rise fast, decay slow. Equal rates read as a hard circle following
           the pointer; the asymmetry is what makes it a trail. */
        const rate = target > glow[gi] ? 0.22 : 0.05
        glow[gi] += (target - glow[gi]) * rate
        lit = glow[gi]
      }

      const spark = hash(cx, cy) < SPARK_RATE
      const jitterAmt = spark ? hash(cx + time * 0.7, cy) * 0.5 : hash(cx, cy) * 0.3
      const size =
        cell * 0.7 * nearness * shimmer * (0.7 + 0.3 * jitterAmt) * (1 + 0.45 * lit)

      c.fillStyle = props.color
      c.globalAlpha = Math.min(1, 0.55 + 0.45 * nearness) * (0.75 + 0.25 * lit)
      const half = Math.min(size, cell * 0.98) / 2
      c.fillRect(x - half, y - half, half * 2, half * 2)
      c.globalAlpha = 1
    }
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
  // Square tiles must stay square: smoothing turns a 3px rect into a blur.
  ctx.imageSmoothingEnabled = false
  glow = null
  draw()
}

function onPointerMove(event: PointerEvent) {
  const el = host.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  pointer = { x: event.clientX - rect.left, y: event.clientY - rect.top }
  onScrub(event)
}

function onPointerLeave() {
  pointer = null
  smoothPointer = null
  endScrub()
}

watch(
  () => props.values,
  (next, prev) => {
    if (reduced || !prev || prev.length === 0) {
      morphing = false
      return
    }
    fromValues = shown.length ? shown.slice() : prev.slice()
    fromMax = Math.max(1, ...(prev.length ? prev : next))
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
})

onBeforeUnmount(() => {
  stop()
  observer?.disconnect()
  resizeObserver?.disconnect()
  document.removeEventListener('visibilitychange', onVisibility)
})

defineExpose({ scrubIndex })
</script>

<template>
  <div
    ref="host"
    class="area"
    @pointermove="onPointerMove"
    @pointerdown="onPointerMove"
    @pointerleave="onPointerLeave"
  >
    <canvas ref="canvas" class="area__canvas" />

    <template v-if="scrubIndex !== null">
      <span class="area__crosshair" :style="{ left: `${markerX.value.value}%` }" />
      <span
        class="area__marker"
        :style="{ left: `${markerX.value.value}%`, top: `${markerY.value.value}%`, background: markerColor }"
      />
      <span
        class="area__readout"
        :style="{ left: `${markerX.value.value}%`, top: `${markerY.value.value}%` }"
      >
        <span class="area__readout-date">{{ scrubLabel }}</span>
        <span class="area__readout-value">{{ scrubValue.toLocaleString() }}</span>
      </span>
    </template>
  </div>
</template>

<style scoped>
.area {
  position: relative;
  width: 100%;
  height: 100%;
  touch-action: none;
}

.area__canvas {
  display: block;
  width: 100%;
  height: 100%;
}

.area__crosshair {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 1px;
  background: color-mix(in oklch, var(--brand) 28%, transparent);
  pointer-events: none;
}

.area__marker {
  position: absolute;
  width: 11px;
  height: 11px;
  border-radius: var(--r-pill);
  /* The ring is --card, not white: on a dark card a white ring is a bright
     dot with a hole in it. */
  box-shadow:
    0 0 0 2px var(--card),
    0 1px 5px rgb(15 23 42 / 0.28);
  transform: translate(-50%, -50%);
  pointer-events: none;
}

/* Anchored above the marker and centred on it, then nudged wholly inside the
   plot by the transform -- a readout that overflows the card is worse than one
   that sits slightly off-centre. */
.area__readout {
  position: absolute;
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 7px 11px;
  border-radius: 10px;
  background: var(--card);
  box-shadow:
    0 0 0 1px var(--border),
    0 6px 20px -6px rgb(15 23 42 / 0.22);
  transform: translate(-50%, calc(-100% - 14px));
  white-space: nowrap;
  pointer-events: none;
}

.area__readout-date {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.area__readout-value {
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  font-variant-numeric: tabular-nums;
}
</style>
