<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

/**
 * Segmented — part 02, section 09.
 *
 * Migration note from the prototype:
 *   "The current .tabs class is used for both jobs, so a value switcher and a
 *    panel switcher look identical. Split them." Segmented switches a value in
 *    place and lives in a tray (radiogroup semantics: it is state, not
 *    navigation). Tabs switch a whole panel and sit on a rule (tablist
 *    semantics). One component, two `variant`s, because both are still "pick
 *    one of these" controls with the same keyboard model — arrow keys roam,
 *    the picked one always holds the tab stop.
 *
 * The moving piece is the segmented thumb: a real sliding element, not a
 * per-button background swap, because 01-TOKENS reserves --ease-spring
 * specifically for "layout snaps and the segmented indicator." It only ever
 * transitions `transform` — never `width` — so ground rule 6's "background
 * and transform only" holds for the indicator too, not just hover states.
 * `--motion-layout` resolves to `0ms` under prefers-reduced-motion (see
 * tokens/motion.css), so the same transition declaration jumps instead of
 * sliding with no extra media query needed here.
 *
 * Gap: the prototype's tab-overflow fade is a literal `background:
 * linear-gradient(...)` (line 531 of the prototype). Ground rule 7 bans
 * gradients outright and the lint greps for the literal function, so that
 * cannot be ported as written. Implemented instead as an inset box-shadow on
 * the scroll row only, confined to the row's own box so it cannot bleed onto
 * the baseline rule below it (the rule lives on a separate element for
 * exactly that reason) — same "more content this way" cue, no gradient.
 */
interface SegmentedItem {
  value: string
  label: string
  disabled?: boolean
  testId?: string
}

const props = withDefaults(
  defineProps<{
    items: SegmentedItem[]
    modelValue: string
    /** Segmented: switches a value in place, radiogroup semantics. Tabs:
     *  switches a panel, tablist semantics, scrolls with overflow. */
    variant?: 'segmented' | 'tabs'
    ariaLabel?: string
  }>(),
  { variant: 'segmented' }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', value: string): void
}>()

const trackRef = ref<HTMLElement | null>(null)
const itemEls = ref<(HTMLElement | null)[]>([])

// Template ref callbacks are typed `Element | ComponentPublicInstance | null`
// by Vue; these are always plain <button>s, so a runtime cast is enough.
function setItemRef(el: unknown, i: number) {
  itemEls.value[i] = el as HTMLElement | null
}

const enabledIndexes = computed(() =>
  props.items.reduce<number[]>((acc, it, i) => {
    if (!it.disabled) acc.push(i)
    return acc
  }, [])
)

function select(item: SegmentedItem) {
  if (item.disabled || item.value === props.modelValue) return
  emit('update:modelValue', item.value)
  emit('change', item.value)
}

function focusIndex(i: number) {
  itemEls.value[i]?.focus()
}

/** Roving tabindex: arrow keys move focus AND selection together, same model
 *  native radio buttons use. Home/End jump to the ends. Disabled items are
 *  skipped, never landed on. */
function onKeydown(event: KeyboardEvent, i: number) {
  const enabled = enabledIndexes.value
  if (!enabled.length) return
  const pos = enabled.indexOf(i)
  let target: number | undefined

  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
    target = enabled[(pos + 1 + enabled.length) % enabled.length]
  } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
    target = enabled[(pos - 1 + enabled.length) % enabled.length]
  } else if (event.key === 'Home') {
    target = enabled[0]
  } else if (event.key === 'End') {
    target = enabled[enabled.length - 1]
  }

  if (target === undefined) return
  event.preventDefault()
  focusIndex(target)
  select(props.items[target])
}

// --- segmented thumb: measured, not guessed, so labels of any width work ---
const thumbX = ref(0)
const thumbW = ref(0)

function measureThumb() {
  if (props.variant !== 'segmented') return
  const i = props.items.findIndex((it) => it.value === props.modelValue)
  const el = itemEls.value[i]
  if (!el) return
  thumbX.value = el.offsetLeft
  thumbW.value = el.offsetWidth
}

const thumbStyle = computed(() => ({
  width: `${thumbW.value}px`,
  transform: `translateX(${thumbX.value}px)`
}))

// --- tabs overflow: fade only while there is more to scroll toward ---
const hasOverflow = ref(false)

function checkOverflow() {
  const el = trackRef.value
  if (!el || props.variant !== 'tabs') {
    hasOverflow.value = false
    return
  }
  hasOverflow.value = el.scrollWidth - el.clientWidth - el.scrollLeft > 1
}

let resizeObserver: ResizeObserver | undefined

onMounted(() => {
  nextTick(measureThumb)
  nextTick(checkOverflow)
  resizeObserver = new ResizeObserver(() => {
    measureThumb()
    checkOverflow()
  })
  if (trackRef.value) resizeObserver.observe(trackRef.value)
})

onBeforeUnmount(() => resizeObserver?.disconnect())

watch(
  () => [props.modelValue, props.items, props.variant],
  () => nextTick(() => {
    measureThumb()
    checkOverflow()
  }),
  { deep: true }
)
</script>

<template>
  <!-- Segmented: value switcher, tray, radiogroup. -->
  <div
    v-if="variant === 'segmented'"
    ref="trackRef"
    class="seg2"
    data-variant="segmented"
    role="radiogroup"
    :aria-label="ariaLabel || undefined"
  >
    <span class="seg2__thumb" :style="thumbStyle" aria-hidden="true" />
    <button
      v-for="(item, i) in items"
      :key="item.value"
      :ref="(el) => setItemRef(el, i)"
      type="button"
      class="seg2__item"
      role="radio"
      :aria-checked="item.value === modelValue"
      :aria-disabled="item.disabled || undefined"
      :tabindex="item.value === modelValue ? 0 : -1"
      :data-active="item.value === modelValue ? '' : undefined"
      :data-test="item.testId"
      @click="select(item)"
      @keydown="onKeydown($event, i)"
    >
      <slot name="item" :item="item">{{ item.label }}</slot>
    </button>
  </div>

  <!-- Tabs: panel switcher, rule baseline, scrolls with a fade when it overflows. -->
  <div v-else class="seg2" data-variant="tabs">
    <div
      ref="trackRef"
      class="seg2__track"
      role="tablist"
      :aria-label="ariaLabel || undefined"
      :data-overflow="hasOverflow ? '' : undefined"
      @scroll="checkOverflow"
    >
      <button
        v-for="(item, i) in items"
        :key="item.value"
        :ref="(el) => setItemRef(el, i)"
        type="button"
        class="seg2__tab"
        role="tab"
        :aria-selected="item.value === modelValue"
        :aria-disabled="item.disabled || undefined"
        :tabindex="item.value === modelValue ? 0 : -1"
        :data-active="item.value === modelValue ? '' : undefined"
        :data-test="item.testId"
        @click="select(item)"
        @keydown="onKeydown($event, i)"
      >
        <slot name="item" :item="item">{{ item.label }}</slot>
        <span class="seg2__rule" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.seg2 {
  display: inline-flex;
  max-width: 100%;
}

/* --- segmented: 28px tray, 2px padding, 24px thumbs ---------------------- */

.seg2[data-variant='segmented'] {
  position: relative;
  height: var(--s2a-h-xs);
  padding: 2px;
  gap: 2px;
  border-radius: var(--r-md);
  background: var(--muted);
}

.seg2__thumb {
  position: absolute;
  top: 2px;
  left: 0;
  height: calc(var(--s2a-h-xs) - 4px);
  border-radius: var(--r-sm);
  background: var(--card);
  box-shadow: var(--shadow-sm);
  pointer-events: none;
  /* Only transform moves. Width is set alongside it, unanimated, so a resize
     never reads as a border-color-style property sweeping in. */
  transition: transform var(--motion-layout);
}

.seg2__item {
  position: relative;
  z-index: 1;
  height: calc(var(--s2a-h-xs) - 4px);
  padding: 0 11px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: 400;
  cursor: pointer;
  transition: color var(--motion-hover);
}

/* State, not category: the active segment gets full-strength ink, riding the
   thumb underneath it. No brand color here, that is reserved for the tabs
   rule below. */
.seg2__item[data-active] {
  color: var(--foreground);
}

.seg2__item[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}

/* --- tabs: 32px, 16px gap, brand rule on a static baseline --------------- */

.seg2[data-variant='tabs'] {
  display: block;
  width: 100%;
  min-width: 0;
  border-bottom: 1px solid var(--border-subtle);
}

.seg2__track {
  display: flex;
  gap: 16px;
  height: var(--s2a-h-sm);
  overflow-x: auto;
  scrollbar-width: none;
}
.seg2__track::-webkit-scrollbar {
  display: none;
}

/* Scroll-edge cue: an inset shadow, not a gradient (ground rule 7 bans the
   CSS function outright and the lint greps for it). Scoped to this row's own
   box, so it can never paint over the baseline rule, which lives on the
   parent below. */
.seg2__track[data-overflow] {
  box-shadow: inset -20px 0 12px -12px color-mix(in oklch, var(--foreground) 12%, transparent);
}

.seg2__tab {
  position: relative;
  flex-shrink: 0;
  height: var(--s2a-h-sm);
  padding: 0 1px;
  border: 0;
  background: transparent;
  color: var(--muted-foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: 400;
  cursor: pointer;
  transition: color var(--motion-hover);
}

.seg2__tab[data-active] {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.seg2__tab[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}

.seg2__rule {
  position: absolute;
  inset-inline: 0;
  bottom: 0;
  height: 2px;
  border-radius: 1px 1px 0 0;
  background: transparent;
  transition: background var(--motion-hover);
}

.seg2__tab[data-active] .seg2__rule {
  background: var(--brand);
}

/* --- shared focus ------------------------------------------------------- */

.seg2__item:focus-visible,
.seg2__tab:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
</style>
