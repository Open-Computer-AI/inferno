/**
 * The login panel's field: a square lattice of touching circles with a shape cut
 * out of it as negative space.
 *
 * Ported from part 17's live prototype rather than reimplemented, because the
 * look depends on exact constants -- the lattice pitch, the bore ratio, the
 * per-transition delay curves -- and reinventing them approximately produces
 * something that reads as "dots on a background" instead of one surface.
 *
 * The four rules the spec is emphatic about, all load-bearing:
 *
 *   1. The mark is a HOLE. Nothing is ever drawn on top. Every cell the glyph
 *      covers is removed and the flat panel shows through. Inferno has no mark
 *      yet, so the field punches one out rather than importing a logo the brand
 *      does not own.
 *   2. The circles TOUCH. Pitch equals diameter exactly, so neighbours meet and
 *      the concave diamonds between every four of them carry the texture. Pull
 *      them apart even slightly and it stops being a surface.
 *   3. A cell has no on and off. Every radius between solid and ring is valid,
 *      so a cell animates its own change rather than snapping.
 *   4. Still, then quick. Nothing moves during the hold; the change is over in
 *      under a second.
 *
 * Kept as a plain class rather than a composable so it owns no Vue reactivity --
 * it writes to a canvas 60 times a second, and routing that through refs would
 * be pure overhead.
 */

export interface Scene {
  kind: 'text' | 'rings' | 'columns' | 'checker' | 'boxes' | 'bars'
  value?: string
  transition: 'wipe' | 'ripple' | 'columns' | 'scatter' | 'collapse'
  palette: number
  style: 'drift' | 'grain' | 'streak' | 'swell'
}

/**
 * Cell pitch in CSS pixels, and the one number that has to be held constant.
 *
 * The prototype hard-codes 42 columns, which is right for the 575x600 preview
 * box it renders into: 575 / (42 + 2 * 0.75 margin) = 13.2px per cell.
 *
 * That constant does NOT survive being moved to a full-bleed panel. With the
 * column count fixed, every extra pixel of panel width goes into making each
 * circle bigger -- 13.2px at 575 wide, 20px at 1440, 29px at 2000 -- and the
 * dense field of touching circles turns into a coarse grid of saucers with a
 * blobby hole in it.
 *
 * So the invariant is the cell size, and the column count derives from it. The
 * mark still scales with the panel (it is sized in cells), which is what keeps
 * it proportionally identical to the prototype at any width.
 */
const CELL_PX = 575 / (42 + 1.5)

/** The prototype's column count, kept only as the floor for a narrow panel. */
export const D_COLS = 42
const D_HOLD = 2600
const D_MORPH = 900

export const SCENES: Scene[] = [
  { kind: 'text', value: 'I', transition: 'wipe', palette: 0, style: 'drift' },
  { kind: 'rings', transition: 'ripple', palette: 1, style: 'grain' },
  { kind: 'columns', transition: 'columns', palette: 2, style: 'streak' },
  { kind: 'checker', transition: 'scatter', palette: 3, style: 'swell' },
  { kind: 'boxes', transition: 'collapse', palette: 4, style: 'grain' },
  { kind: 'bars', transition: 'wipe', palette: 5, style: 'drift' }
]

/**
 * "Full colour ships." Six two-tone pairs, one per scene, cross-blended on the
 * morph. This is the one place in Inferno that breaks the single-accent ground
 * rule, and it earns it by being the only screen with nothing else on it --
 * which is also why these are literal hex rather than June tokens. They are
 * reference art, not theme.
 */
export const PALETTES: [string, string][] = [
  ['#8aa9ff', '#1f45f5'],
  ['#ffd166', '#e5484d'],
  ['#b8f2c9', '#0f8a5f'],
  ['#ffc2e2', '#c81d77'],
  ['#c7d2fe', '#4338ca'],
  ['#fde68a', '#b45309']
]

const dHash = (n: number): number => {
  const s = Math.sin(n * 127.1 + 311.7) * 43758.5453
  return s - Math.floor(s)
}
const dHash2 = (x: number, y: number): number => {
  const s = Math.sin(x * 127.1 + y * 311.7) * 43758.5453
  return s - Math.floor(s)
}
const dSmooth = (v: number, e0: number, e1: number): number => {
  const t = Math.min(1, Math.max(0, (v - e0) / (e1 - e0)))
  return t * t * (3 - 2 * t)
}
const dEaseOut = (t: number): number => 1 - Math.pow(1 - t, 3)
const dEaseInOut = (t: number): number =>
  t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2

/** Per-cell start offset. This is what gives each arrival its own character. */
function cellDelay(
  kind: Scene['transition'],
  x: number,
  y: number,
  cols: number,
  rows: number,
  rand: number
): number {
  const fx = cols > 1 ? x / (cols - 1) : 0
  const fy = rows > 1 ? y / (rows - 1) : 0
  if (kind === 'wipe') return Math.min(1, Math.max(0, (fx * 0.75 + fy * 0.25) * 0.85 + rand * 0.15))
  if (kind === 'ripple') return Math.min(1, (Math.hypot(fx - 0.5, fy - 0.5) / 0.707) * 0.9 + rand * 0.1)
  if (kind === 'scatter') return rand
  if (kind === 'collapse')
    return Math.min(1, (1 - Math.hypot(fx - 0.5, fy - 0.5) / 0.707) * 0.85 + rand * 0.15)
  return Math.min(1, fx * 0.9 + rand * 0.1)
}

/** Travel is deliberately well under one cell, so it reads as pressure not flight. */
function cellMotion(
  kind: Scene['transition'],
  t: number,
  dir: number,
  rand: number
): { scale: number; dx: number; dy: number } {
  const u = Math.sin(Math.min(1, Math.max(0, t)) * Math.PI)
  if (kind === 'wipe') return { scale: 1, dx: u * 0.16 * -dir, dy: 0 }
  if (kind === 'ripple') return { scale: 1 - u * 0.1, dx: 0, dy: u * -0.13 }
  if (kind === 'scatter')
    return {
      scale: 1,
      dx: u * 0.18 * Math.cos(rand * Math.PI * 2),
      dy: u * 0.18 * Math.sin(rand * Math.PI * 2)
    }
  if (kind === 'collapse') return { scale: 1 - u * 0.18, dx: 0, dy: 0 }
  return { scale: 1, dx: 0, dy: u * 0.22 }
}

/**
 * The SECOND pattern. The scene carves the field; this bores each surviving cell
 * open from a disc into a ring. The pair always differs in axis from the cut, so
 * the two read as two rather than as one blur.
 */
function styleField(
  scene: Scene,
  cols: number,
  rows: number,
  t: number,
  out: Float32Array,
  prev: Scene
): void {
  const cx = (cols - 1) / 2
  const cy = (rows - 1) / 2
  const maxR = Math.hypot(cols, rows) / 2
  const FLIP = 0.32

  const stateOf = (style: Scene['style'], x: number, y: number): number => {
    if (style === 'drift') {
      const a = Math.sin(x * 0.41 + y * 0.23)
      const b = Math.sin(x * 0.17 - y * 0.53 + 2.1)
      return dSmooth((a + b) * 0.5, -0.15, 0.75)
    }
    if (style === 'grain') {
      const n =
        dHash2(x, y) * 0.55 +
        dHash2(x + 1, y) * 0.15 +
        dHash2(x, y + 1) * 0.15 +
        dHash2(x + 1, y + 1) * 0.15
      return dSmooth(n, 0.34, 0.86)
    }
    if (style === 'swell') {
      const d = Math.hypot(x - cx, y - cy) / maxR
      const warp = Math.sin(Math.atan2(y - cy, x - cx) * 3.0) * 0.14
      return dSmooth(1 - (d + warp), 0.28, 0.92)
    }
    if (style === 'streak') {
      const s = Math.sin(x * 0.28 + y * 0.62)
      const cut = Math.sin(x * 0.09 - y * 0.11 + 1.3) * 0.5 + 0.5
      return dSmooth(s * cut, -0.05, 0.7)
    }
    return 0
  }

  for (let y = 0; y < rows; y++) {
    for (let x = 0; x < cols; x++) {
      let order = 0
      if (scene.style === 'drift') order = (x / cols) * 0.75 + Math.sin(y * 0.5) * 0.12 + 0.12
      else if (scene.style === 'grain')
        order = (x / cols) * 0.55 + (y / rows) * 0.25 + dHash2(x, y) * 0.2
      else if (scene.style === 'swell') order = Math.hypot(x - cx, y - cy) / maxR
      else if (scene.style === 'streak') order = (x / cols) * 0.8 + (y / rows) * 0.2
      const from = stateOf(prev ? prev.style : scene.style, x, y)
      const to = stateOf(scene.style, x, y)
      const u = Math.min(1, Math.max(0, (t - order * (1 - FLIP)) / FLIP))
      out[y * cols + x] = from + (to - from) * (u * u * (3 - 2 * u))
    }
  }
}

/**
 * Which cells survive. 1 = circle present, 0 = punched out.
 *
 * The text scene renders the glyph to a tiny offscreen canvas at grid resolution
 * and thresholds it -- that is what makes the mark an absence rather than an
 * overlay. Only bold closed forms survive at this cell count, which is a real
 * constraint on whatever mark eventually gets commissioned.
 */
export function rasterize(scene: Scene, cols: number, rows: number, fontFamily: string): Uint8Array {
  const out = new Uint8Array(cols * rows).fill(1)
  const cx = (cols - 1) / 2
  const cy = (rows - 1) / 2

  if (scene.kind === 'checker') {
    const b = Math.max(2, Math.round(cols / 14))
    for (let y = 0; y < rows; y++)
      for (let x = 0; x < cols; x++)
        if ((Math.floor(x / b) + Math.floor(y / b)) % 2 === 0) out[y * cols + x] = 0
    return out
  }
  if (scene.kind === 'bars') {
    for (let y = 0; y < rows; y++)
      for (let x = 0; x < cols; x++) if (Math.floor((x + y) / 3) % 2 === 0) out[y * cols + x] = 0
    return out
  }
  if (scene.kind === 'columns') {
    const bw = 4
    const bh = 3
    for (let y = 0; y < rows; y++) {
      const shift = Math.floor(y / bh) % 2 === 0 ? 0 : bw / 2
      for (let x = 0; x < cols; x++)
        if (Math.floor((x + shift) / bw) % 2 === 0) out[y * cols + x] = 0
    }
    return out
  }
  if (scene.kind === 'boxes') {
    for (let y = 0; y < rows; y++)
      for (let x = 0; x < cols; x++) {
        const d = Math.max(Math.abs(x - cx), Math.abs(y - cy))
        if (Math.floor(d / 2.5) % 2 === 0) out[y * cols + x] = 0
      }
    return out
  }
  if (scene.kind === 'rings') {
    const maxR = Math.hypot(cols, rows) / 2
    for (let y = 0; y < rows; y++)
      for (let x = 0; x < cols; x++)
        if (Math.floor((Math.hypot(x - cx, y - cy) / maxR) * 6.0) % 2 === 0) out[y * cols + x] = 0
    return out
  }

  const cv = document.createElement('canvas')
  cv.width = cols
  cv.height = rows
  const ctx = cv.getContext('2d', { willReadFrequently: true })
  const text = (scene.value || '').trim()
  if (!ctx || !text) return out

  ctx.fillStyle = '#000'
  ctx.fillRect(0, 0, cols, rows)
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'

  let size = rows * 0.8
  ctx.font = '600 ' + size + 'px ' + fontFamily
  const maxW = cols * 0.36
  const m = ctx.measureText(text)
  if (m.width > maxW) {
    size *= maxW / m.width
    ctx.font = '600 ' + size + 'px ' + fontFamily
  }
  const maxH = rows * 0.58
  const mm = ctx.measureText(text)
  const gh = mm.actualBoundingBoxAscent + mm.actualBoundingBoxDescent
  if (gh > maxH) {
    size *= maxH / gh
    ctx.font = '600 ' + size + 'px ' + fontFamily
  }
  ctx.fillText(text, cols / 2, rows / 2 + rows * 0.02)

  const data = ctx.getImageData(0, 0, cols, rows).data
  for (let i = 0; i < cols * rows; i++) if (data[i * 4] > 110) out[i] = 0
  return out
}

function mixHex(a: string, b: string, t: number): string {
  const pa = parseInt(a.slice(1), 16)
  const pb = parseInt(b.slice(1), 16)
  const r = Math.round(((pa >> 16) & 255) * (1 - t) + ((pb >> 16) & 255) * t)
  const g = Math.round(((pa >> 8) & 255) * (1 - t) + ((pb >> 8) & 255) * t)
  const bl = Math.round((pa & 255) * (1 - t) + (pb & 255) * t)
  return 'rgb(' + r + ',' + g + ',' + bl + ')'
}

export class DotCutField {
  private cv: HTMLCanvasElement
  private ctx: CanvasRenderingContext2D | null
  private font: string
  private palettes: [string, string][]

  private cols = D_COLS
  private rows = 12
  private pitch = 10
  private ox = 0
  private oy = 0
  private _w = 0
  private _h = 0

  private sceneIdx = 0
  private prevScene = 0
  private prevPalette = 0
  private paletteMix = 1
  private phase: 'hold' | 'morph' = 'hold'
  private phaseT = 0
  private styleT = 1

  private pointer: { x: number; y: number } | null = null
  /**
   * Cursor retraction radius, in CELLS.
   *
   * The prototype's 2.1 was tuned against its own 13.2px cells in a 575px box,
   * where it read as a ~28px pocket under the pointer. On a full-bleed panel
   * that is a small dimple on a very large surface, so it is widened here --
   * the effect should read from across the screen, not only where the cursor is.
   *
   * In cells rather than pixels on purpose: the falloff has to stay the same
   * shape relative to the lattice, or the retraction stops looking like the
   * circles are moving and starts looking like a blur.
   */
  private brush = 5.5
  private fill = 1.0
  private dpr: number

  private target = new Uint8Array(0)
  private live = new Float32Array(0)
  private from = new Float32Array(0)
  private delay = new Float32Array(0)
  private rnd = new Float32Array(0)
  private prog = new Float32Array(0)
  private dir = new Float32Array(0)
  private bore = new Float32Array(0)

  running = false
  private raf = 0
  private last = 0
  private dead = false

  constructor(canvas: HTMLCanvasElement, fontFamily: string, palettes: [string, string][]) {
    this.cv = canvas
    this.ctx = canvas.getContext('2d')
    this.font = fontFamily || 'serif'
    this.palettes = palettes
    // Capped at 2: beyond that the cost is real and the lattice gains nothing.
    this.dpr = Math.min(window.devicePixelRatio || 1, 2)
    this.resize()
  }

  resize(): void {
    if (!this.ctx || this.dead) return
    const r = this.cv.getBoundingClientRect()
    const w = Math.round(Math.max(1, r.width))
    const h = Math.round(Math.max(1, r.height))
    if (w < 4 || h < 4) return
    if (w === this._w && h === this._h) return
    this._w = w
    this._h = h
    this.cv.width = Math.round(w * this.dpr)
    this.cv.height = Math.round(h * this.dpr)

    // Columns derive from width at a fixed cell size, so the lattice reads at
    // the prototype's density on any panel. See CELL_PX -- doing this the other
    // way round (fixed columns, derived pitch) is what made the circles 2.2x
    // too big on a full-height panel.
    const margin = 0.75
    this.cols = Math.max(12, Math.round(w / CELL_PX - 2 * margin))
    this.pitch = w / (this.cols + 2 * margin)
    this.rows = Math.max(3, Math.floor((h - 2 * margin * this.pitch) / this.pitch))
    this.ox = (w - this.cols * this.pitch) / 2
    this.oy = (h - this.rows * this.pitch) / 2

    const n = this.cols * this.rows
    this.target = new Uint8Array(n)
    this.live = new Float32Array(n)
    this.from = new Float32Array(n)
    this.delay = new Float32Array(n)
    this.rnd = new Float32Array(n)
    this.prog = new Float32Array(n)
    this.dir = new Float32Array(n)
    this.bore = new Float32Array(n)
    for (let i = 0; i < n; i++) this.rnd[i] = dHash(i * 1.37 + 0.5)
    this.applyScene(SCENES[this.sceneIdx], true)
    styleField(SCENES[this.sceneIdx], this.cols, this.rows, 1, this.bore, SCENES[this.sceneIdx])
    this.draw(0)
  }

  private applyScene(scene: Scene, instant: boolean): void {
    const next = rasterize(scene, this.cols, this.rows, this.font)
    this.from.set(this.live)
    this.target = next
    for (let y = 0; y < this.rows; y++)
      for (let x = 0; x < this.cols; x++) {
        const i = y * this.cols + x
        this.delay[i] = cellDelay(scene.transition, x, y, this.cols, this.rows, this.rnd[i])
      }
    if (instant) {
      for (let i = 0; i < next.length; i++) this.live[i] = next[i]
      this.from.set(this.live)
    }
  }

  private advance(): void {
    this.prevScene = this.sceneIdx
    this.prevPalette = SCENES[this.sceneIdx].palette
    this.sceneIdx = (this.sceneIdx + 1) % SCENES.length
    this.paletteMix = 0
    this.phase = 'morph'
    this.phaseT = 0
    this.styleT = 0
    this.applyScene(SCENES[this.sceneIdx], false)
  }

  setPointer(p: { x: number; y: number } | null): void {
    this.pointer = p
  }

  toCell(px: number, py: number): { x: number; y: number } {
    return { x: (px - this.ox) / this.pitch, y: (py - this.oy) / this.pitch }
  }

  private step(dt: number): void {
    this.phaseT += dt * 1000
    if (this.phase === 'hold' && this.phaseT >= D_HOLD) this.advance()
    else if (this.phase === 'morph' && this.phaseT >= D_MORPH) {
      this.phase = 'hold'
      this.phaseT = 0
    }

    const p = this.phase === 'morph' ? Math.min(1, this.phaseT / D_MORPH) : 1
    const n = this.cols * this.rows
    for (let i = 0; i < n; i++) {
      const local = Math.min(1, Math.max(0, (p - this.delay[i] * 0.72) / 0.28))
      this.live[i] = this.from[i] + (this.target[i] - this.from[i]) * dEaseOut(local)
      this.prog[i] = this.from[i] !== this.target[i] && this.phase === 'morph' ? local : 0
      this.dir[i] = this.target[i] > this.from[i] ? 1 : -1
    }
    this.paletteMix = Math.min(1, this.paletteMix + dt * 2.2)
    this.styleT = this.phase === 'morph' ? Math.min(1, this.styleT + dt / (D_MORPH / 1000)) : 1
    styleField(
      SCENES[this.sceneIdx],
      this.cols,
      this.rows,
      this.styleT,
      this.bore,
      SCENES[this.prevScene]
    )
  }

  draw(dt: number): void {
    const ctx = this.ctx
    if (!ctx) return
    if (dt > 0) this.step(dt)

    const W = this.cv.width
    const H = this.cv.height
    const s = this.dpr
    const scene = SCENES[this.sceneIdx]
    const pals = this.palettes
    const a = pals[this.prevPalette % pals.length]
    const b = pals[scene.palette % pals.length]
    const m = dEaseInOut(this.paletteMix)
    const circle = mixHex(a[0], b[0], m)
    const back = mixHex(a[1], b[1], m)

    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.fillStyle = back
    ctx.fillRect(0, 0, W, H)

    const pitch = this.pitch * s
    const r = pitch / 2
    const stroke = Math.max(1.1 * s, r * 0.3)
    const brush = this.brush

    // One Path2D for every cell, filled once with evenodd. Thousands of
    // individual fill() calls would be the whole frame budget; the bore is an
    // opposite-wound arc in the same path, which is what makes it a ring.
    const path = new Path2D()

    for (let y = 0; y < this.rows; y++) {
      for (let x = 0; x < this.cols; x++) {
        const i = y * this.cols + x
        let v = this.live[i]
        if (v <= 0.004) continue

        // Cursor retraction, squared falloff.
        if (this.pointer && brush > 0) {
          const d = Math.hypot(x + 0.5 - this.pointer.x, y + 0.5 - this.pointer.y)
          if (d < brush) v *= Math.min(1, Math.pow(d / brush, 2))
        }
        if (v <= 0.004) continue

        const mo = cellMotion(scene.transition, this.prog[i], this.dir[i], this.rnd[i])
        const cx = this.ox * s + (x + 0.5) * pitch + mo.dx * pitch
        const cy = this.oy * s + (y + 0.5) * pitch + mo.dy * pitch
        const rr = r * v * mo.scale * this.fill
        if (rr <= 0.3) continue

        // Below ~3px a ring is indistinguishable from a disc and just costs fill.
        const bore = rr > 3.2 * s ? (rr - stroke) * this.bore[i] : 0
        path.moveTo(cx + rr, cy)
        path.arc(cx, cy, rr, 0, Math.PI * 2)
        if (bore > 0.4) {
          path.moveTo(cx + bore, cy)
          path.arc(cx, cy, bore, 0, Math.PI * 2, true)
        }
      }
    }
    ctx.fillStyle = circle
    ctx.fill(path, 'evenodd')
  }

  start(): void {
    if (this.running || this.dead || !this.ctx) return
    this.running = true
    this.last = performance.now()
    const tick = (now: number): void => {
      if (!this.running) return
      // Clamped so a backgrounded tab returning does not jump the animation.
      const dt = Math.min((now - this.last) / 1000, 1 / 30)
      this.last = now
      this.draw(dt)
      this.raf = requestAnimationFrame(tick)
    }
    this.raf = requestAnimationFrame(tick)
  }

  stop(): void {
    this.running = false
    if (this.raf) cancelAnimationFrame(this.raf)
    this.raf = 0
  }

  destroy(): void {
    this.dead = true
    this.stop()
    this.ctx = null
  }
}
