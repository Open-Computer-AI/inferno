<template>
  <!-- aria-hidden and no alt text on purpose: this is decoration with no
       informational content, and announcing "canvas" to a screen reader on the
       sign-in screen is noise in front of the one task that matters. -->
  <div class="field" aria-hidden="true">
    <canvas ref="canvasRef" class="field__cv" />
  </div>
</template>

<script setup lang="ts">
/**
 * The login panel. See `dotCutField.ts` for the field itself and why its
 * constants are ported rather than reinvented.
 *
 * This component owns only the lifecycle, which is most of what makes an
 * always-running canvas acceptable to ship:
 *
 *   - `prefers-reduced-motion` gets one static frame, never a loop.
 *   - Offscreen or hidden tab stops the loop entirely rather than throttling it.
 *   - Fonts are awaited before the first raster, because the mark is cut from
 *     the serif and rasterising against a fallback punches the wrong shape.
 *
 * Panel copy is deliberately absent. A sentence here would need a scrim, and a
 * scrim is a hole in the surface that is not the mark.
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { DotCutField, PALETTES } from './dotCutField'

const canvasRef = ref<HTMLCanvasElement | null>(null)
let field: DotCutField | null = null
let teardown: (() => void) | null = null

onMounted(() => {
  const cv = canvasRef.value
  if (!cv) return

  // Resolved from the token rather than hard-coded so the cut-out follows the
  // typeface if it ever changes. Falls back to a generic serif.
  const serif =
    getComputedStyle(document.documentElement).getPropertyValue('--font-serif').trim() || 'serif'

  const dot = new DotCutField(cv, serif, PALETTES)
  field = dot

  // The glyph is rasterised at grid resolution, so a fallback face during load
  // punches a visibly different silhouette. Re-raster once the real one lands.
  if (document.fonts?.ready) {
    document.fonts.ready.then(() => dot.resize()).catch(() => {})
  }

  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)')

  // Resize is coalesced to one per frame: ResizeObserver can fire several times
  // during a drag, and each resize reallocates eight typed arrays.
  let roQueued = 0
  const ro = new ResizeObserver(() => {
    if (roQueued) return
    roQueued = requestAnimationFrame(() => {
      roQueued = 0
      dot.resize()
    })
  })
  ro.observe(cv)

  const move = (e: PointerEvent): void => {
    const r = cv.getBoundingClientRect()
    dot.setPointer(dot.toCell(e.clientX - r.left, e.clientY - r.top))
  }
  const leave = (): void => dot.setPointer(null)
  cv.addEventListener('pointermove', move)
  cv.addEventListener('pointerleave', leave)

  const onScreen = (): boolean => {
    const r = cv.getBoundingClientRect()
    const vh = window.innerHeight || document.documentElement.clientHeight || 0
    return r.width > 0 && r.bottom > -240 && r.top < vh + 240
  }

  let queued = 0
  const run = (): void => {
    queued = 0
    if (onScreen() && !document.hidden && !reduce.matches) dot.start()
    else if (dot.running) dot.stop()
  }
  const sync = (): void => {
    if (!queued) queued = requestAnimationFrame(run)
  }

  const io = new IntersectionObserver(() => sync(), { threshold: 0, rootMargin: '240px' })
  io.observe(cv)
  document.addEventListener('visibilitychange', sync)
  reduce.addEventListener('change', sync)
  run()

  teardown = () => {
    ro.disconnect()
    io.disconnect()
    if (queued) cancelAnimationFrame(queued)
    if (roQueued) cancelAnimationFrame(roQueued)
    document.removeEventListener('visibilitychange', sync)
    reduce.removeEventListener('change', sync)
    cv.removeEventListener('pointermove', move)
    cv.removeEventListener('pointerleave', leave)
  }
})

onBeforeUnmount(() => {
  teardown?.()
  teardown = null
  field?.destroy()
  field = null
})
</script>

<style scoped>
.field {
  position: relative;
  height: 100%;
  width: 100%;
  overflow: hidden;
  /* Shows through wherever a circle is punched out. Matches the first palette's
     ground so the very first paint has nothing to flash against. */
  background: #1f45f5;
}

.field__cv {
  display: block;
  height: 100%;
  width: 100%;
}
</style>
