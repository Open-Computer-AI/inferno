<template>
  <canvas
    ref="canvasRef"
    class="watercolor-mosaic"
    aria-hidden="true"
    @pointermove="handlePointerMove"
    @pointerleave="handlePointerLeave"
  />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const PITCH = 40
const TILE_SIZE = 36
const TILE_GUTTER = PITCH - TILE_SIZE
const FRAME_MS = 1000 / 14.3
const LOOP_MS = 2100
const AXIS_ANGLE = (46.7 * Math.PI) / 180
const WASH_ANGLE = (60.7 * Math.PI) / 180
const COLUMN_PHASE = (-30.25 * Math.PI) / 180
const ROW_PHASE = (16.96 * Math.PI) / 180
const HOVER_PHASE = 0.55
const HOVER_RADIUS = PITCH * 2.3
const HOVER_IN_MS = 140
const HOVER_OUT_MS = 420
const SUPERELLIPSE_EXPONENT = 10
const PAPER = [247, 244, 237] as const

type Tile = {
  centerX: number
  centerY: number
  phase: number
  seed: number
  contour: Array<{ x: number; y: number }>
  hover: number
}

const canvasRef = ref<HTMLCanvasElement | null>(null)

let context: CanvasRenderingContext2D | null = null
let washCanvas: HTMLCanvasElement | null = null
let washContext: CanvasRenderingContext2D | null = null
let tiles: Tile[] = []
let frameHandle = 0
let immediateFrameHandle = 0
let frameScheduled = false
let lastFrameAt = 0
let elapsed = 0
let currentStep = 0
let cssWidth = 0
let cssHeight = 0
let devicePixelRatio = 1
let pointerX = -Infinity
let pointerY = -Infinity
let pointerActive = false
let finePointer = false
let reducedMotion = false
let resizeObserver: ResizeObserver | null = null
let motionQuery: MediaQueryList | null = null
let pointerQuery: MediaQueryList | null = null

const pigmentPalette = [
  [169, 83, 61],
  [183, 99, 54],
  [153, 92, 74],
  [171, 119, 61],
  [132, 111, 75],
  [149, 88, 93],
  [126, 103, 79],
  [184, 108, 67]
] as const

function hash(seed: number) {
  let value = seed | 0
  value = Math.imul(value ^ (value >>> 16), 0x45d9f3b)
  value = Math.imul(value ^ (value >>> 16), 0x45d9f3b)
  value = (value ^ (value >>> 16)) >>> 0
  return value / 4294967296
}

function tileHash(tile: Tile, step: number, sample: number) {
  return hash(tile.seed + step * 104729 + sample * 7919)
}

function makeContour(seed: number) {
  const points: Array<{ x: number; y: number }> = []
  const half = TILE_SIZE / 2
  const pointCount = 40

  for (let index = 0; index < pointCount; index += 1) {
    const angle = (index / pointCount) * Math.PI * 2
    const cos = Math.cos(angle)
    const sin = Math.sin(angle)
    const exponent = 2 / SUPERELLIPSE_EXPONENT
    const baseX = Math.sign(cos) * Math.pow(Math.abs(cos), exponent) * half
    const baseY = Math.sign(sin) * Math.pow(Math.abs(sin), exponent) * half

    let wobble = 0
    for (let harmonic = 1; harmonic <= 5; harmonic += 1) {
      wobble +=
        Math.sin(angle * harmonic + hash(seed + harmonic) * Math.PI * 2) *
        (0.26 - harmonic * 0.025)
    }
    for (let harmonic = 9; harmonic <= 17; harmonic += 1) {
      wobble +=
        Math.sin(angle * harmonic + hash(seed + harmonic * 13) * Math.PI * 2) * 0.055
    }

    const scale = 1 + wobble / half
    points.push({ x: baseX * scale, y: baseY * scale })
  }

  return points
}

function buildTiles() {
  tiles = []
  const columns = Math.ceil(cssWidth / PITCH) + 1
  const rows = Math.ceil(cssHeight / PITCH) + 1

  for (let row = 0; row < rows; row += 1) {
    for (let column = 0; column < columns; column += 1) {
      const centerX = column * PITCH + TILE_GUTTER / 2 + TILE_SIZE / 2
      const centerY = row * PITCH + TILE_GUTTER / 2 + TILE_SIZE / 2
      // Physical position keeps the travelling wave straight when the grid shifts.
      const phase = (centerX / PITCH) * COLUMN_PHASE + (centerY / PITCH) * ROW_PHASE
      const seed = Math.round(centerX * 17 + centerY * 31)

      tiles.push({ centerX, centerY, phase, seed, contour: makeContour(seed), hover: 0 })
    }
  }
}

function resizeCanvas() {
  const canvas = canvasRef.value
  if (!canvas) return

  const bounds = canvas.getBoundingClientRect()
  cssWidth = Math.max(1, bounds.width)
  cssHeight = Math.max(1, bounds.height)
  devicePixelRatio = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.round(cssWidth * devicePixelRatio)
  canvas.height = Math.round(cssHeight * devicePixelRatio)
  context = canvas.getContext('2d')
  context?.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0)
  if (context) context.imageSmoothingEnabled = true
  buildTiles()
  drawFrame(performance.now(), true)
}

function makeWash(tile: Tile, step: number, pigment: readonly [number, number, number]) {
  if (!washContext) return

  const image = washContext.createImageData(5, 5)
  for (let index = 0; index < 25; index += 1) {
    const noise = tileHash(tile, step, index)
    const alpha = Math.round(255 * (0.64 + noise * 0.36))
    const pixel = index * 4
    image.data[pixel] = pigment[0]
    image.data[pixel + 1] = pigment[1]
    image.data[pixel + 2] = pigment[2]
    image.data[pixel + 3] = alpha
  }
  washContext.putImageData(image, 0, 0)
}

function traceContour(tile: Tile) {
  if (!context) return
  context.beginPath()
  tile.contour.forEach((point, index) => {
    if (index === 0) context?.moveTo(point.x, point.y)
    else context?.lineTo(point.x, point.y)
  })
  context.closePath()
}

function drawTile(tile: Tile, step: number, openness: number, isFront: boolean, extraPass = false) {
  if (!context || !washCanvas) return

  const paletteIndex = Math.abs(Math.floor(tileHash(tile, 0, 97) * pigmentPalette.length))
  const pigment = pigmentPalette[paletteIndex]
  const strength = isFront ? 0.16 : 0.09
  const passAlpha = extraPass ? 0.22 : 1

  context.save()
  context.translate(tile.centerX, tile.centerY)
  // Rotate into the flip axis, squash across local y, and rotate back.
  context.rotate(AXIS_ANGLE)
  context.scale(1, Math.max(0.012, openness))
  context.rotate(-AXIS_ANGLE)
  traceContour(tile)
  context.clip()

  makeWash(tile, step, pigment)
  context.globalAlpha = strength * passAlpha
  context.save()
  context.rotate(WASH_ANGLE)
  context.drawImage(washCanvas, -TILE_SIZE / 2, -TILE_SIZE / 2, TILE_SIZE, TILE_SIZE)
  context.restore()

  context.globalAlpha = Math.min(1, strength * passAlpha * 0.74)
  context.lineWidth = 0.7
  context.strokeStyle = `rgb(${pigment[0]} ${pigment[1]} ${pigment[2]})`
  traceContour(tile)
  context.stroke()
  context.restore()
}

function smoothFalloff(distance: number) {
  if (distance >= HOVER_RADIUS) return 0
  const normalized = 1 - distance / HOVER_RADIUS
  return normalized * normalized * (3 - 2 * normalized)
}

function updateHover(tile: Tile, delta: number) {
  if (!finePointer || !pointerActive) {
    tile.hover *= Math.exp(-delta / HOVER_OUT_MS)
    return
  }

  const distance = Math.hypot(tile.centerX - pointerX, tile.centerY - pointerY)
  const target = smoothFalloff(distance)
  const duration = target > tile.hover ? HOVER_IN_MS : HOVER_OUT_MS
  const amount = 1 - Math.exp(-delta / duration)
  tile.hover += (target - tile.hover) * amount
}

function scheduleFrame() {
  if (reducedMotion || frameScheduled) return

  frameScheduled = true
  frameHandle = requestAnimationFrame((timestamp) => {
    frameScheduled = false
    drawFrame(timestamp)
  })
}

function drawFrame(timestamp: number, force = false) {
  if (!context) return

  const delta = lastFrameAt ? timestamp - lastFrameAt : FRAME_MS
  if (!force && !reducedMotion && delta < FRAME_MS) {
    scheduleFrame()
    return
  }
  lastFrameAt = timestamp
  if (!reducedMotion) elapsed += Math.min(delta, FRAME_MS * 3)
  currentStep = reducedMotion ? 0 : Math.floor(elapsed / FRAME_MS)

  context.save()
  context.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0)
  context.fillStyle = `rgb(${PAPER[0]} ${PAPER[1]} ${PAPER[2]})`
  context.fillRect(0, 0, cssWidth, cssHeight)

  const loopProgress = (elapsed % LOOP_MS) / LOOP_MS
  for (const tile of tiles) {
    updateHover(tile, Math.max(delta, FRAME_MS))
    const psi = tile.phase + loopProgress * Math.PI * 2 + tile.hover * HOVER_PHASE
    const signedOpenness = Math.cos(psi)
    const openness = Math.abs(signedOpenness)
    drawTile(tile, currentStep, openness, signedOpenness >= 0)
    if (tile.hover > 0.002) drawTile(tile, currentStep, openness, signedOpenness >= 0, true)
  }
  context.restore()

  scheduleFrame()
}

function scheduleImmediateFrame() {
  if (immediateFrameHandle) return

  immediateFrameHandle = requestAnimationFrame((timestamp) => {
    immediateFrameHandle = 0
    drawFrame(timestamp, true)
  })
}

function handlePointerMove(event: PointerEvent) {
  if (!finePointer || (event.pointerType !== 'mouse' && event.pointerType !== 'pen')) return
  const bounds = canvasRef.value?.getBoundingClientRect()
  if (!bounds) return
  pointerX = event.clientX - bounds.left
  pointerY = event.clientY - bounds.top
  pointerActive = true
  // Redraw the current keyed wash immediately so hover never waits for a frame.
  scheduleImmediateFrame()
}

function handlePointerLeave() {
  pointerActive = false
  pointerX = -Infinity
  pointerY = -Infinity
  scheduleImmediateFrame()
}

function handleMotionPreferenceChange(event: MediaQueryListEvent) {
  reducedMotion = event.matches
  cancelAnimationFrame(frameHandle)
  cancelAnimationFrame(immediateFrameHandle)
  frameHandle = 0
  immediateFrameHandle = 0
  frameScheduled = false
  lastFrameAt = 0
  drawFrame(performance.now(), true)
}

function handlePointerPreferenceChange(event: MediaQueryListEvent) {
  finePointer = event.matches
  if (!finePointer) handlePointerLeave()
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return

  washCanvas = document.createElement('canvas')
  washCanvas.width = 5
  washCanvas.height = 5
  washContext = washCanvas.getContext('2d')
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  pointerQuery = window.matchMedia('(pointer: fine)')
  reducedMotion = motionQuery.matches
  finePointer = pointerQuery.matches
  motionQuery.addEventListener('change', handleMotionPreferenceChange)
  pointerQuery.addEventListener('change', handlePointerPreferenceChange)

  resizeObserver = new ResizeObserver(resizeCanvas)
  resizeObserver.observe(canvas)
  resizeCanvas()
  drawFrame(performance.now(), true)
})

onBeforeUnmount(() => {
  cancelAnimationFrame(frameHandle)
  cancelAnimationFrame(immediateFrameHandle)
  resizeObserver?.disconnect()
  motionQuery?.removeEventListener('change', handleMotionPreferenceChange)
  pointerQuery?.removeEventListener('change', handlePointerPreferenceChange)
})
</script>

<style scoped>
.watercolor-mosaic {
  display: block;
  height: 100%;
  width: 100%;
  background: rgb(247 244 237);
  touch-action: none;
}
</style>
