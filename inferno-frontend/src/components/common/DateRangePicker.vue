<template>
  <!-- Same containerRef/teleport split as Select.vue: containerRef is a plain block
       wrapper used for outside-click detection and as the popover's anchor rect: the
       popover itself is `position: fixed`, not an absolutely-positioned child. -->
  <div class="dr" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      class="dr-trigger date-picker-trigger"
      :data-open="isOpen || undefined"
      :aria-expanded="isOpen"
      :aria-haspopup="true"
      :aria-label="t('dates.selectDateRange')"
      @click="toggle"
    >
      <i class="hgi-stroke hgi-calendar-01 dr-trigger__icon" aria-hidden="true" />
      <span class="dr-trigger__value">{{ displayValue }}</span>
      <i
        class="hgi-stroke hgi-arrow-down-01 dr-trigger__caret"
        :class="isOpen && 'dr-trigger__caret--open'"
        aria-hidden="true"
      />
    </button>

    <Teleport to="body">
      <Transition name="dr-pop">
        <div
          v-if="isOpen"
          ref="popoverRef"
          class="dr-pop"
          :class="[instanceId]"
          :style="popoverStyle"
          @click.stop
          @mousedown.stop
        >
          <!-- Preset rail, 132px per the anatomy spec. Presets are the primary way to
               set a range; the migration note: "Presets move from a row above the
               calendar to a rail beside it, and the trigger starts naming the preset." -->
          <div class="dr-pop__rail">
            <Button
              v-for="preset in presets"
              :key="preset.value"
              variant="ghost"
              size="sm"
              type="button"
              class="dr-pop__preset date-picker-preset"
              :data-active="draftPreset === preset.value || undefined"
              @click="selectPreset(preset)"
            >
              {{ t(preset.labelKey) }}
            </Button>
          </div>

          <div class="dr-pop__body">
            <div class="dr-pop__fields">
              <label class="dr-pop__field">
                <span class="dr-pop__field-label">{{ t('dates.from') }}</span>
                <input
                  type="text"
                  class="dr-pop__field-input"
                  v-model.lazy="fromModel"
                  placeholder="YYYY-MM-DD"
                />
              </label>
              <label class="dr-pop__field">
                <span class="dr-pop__field-label">{{ t('dates.to') }}</span>
                <input
                  type="text"
                  class="dr-pop__field-input"
                  v-model.lazy="toModel"
                  placeholder="YYYY-MM-DD"
                />
              </label>
            </div>

            <!-- The month grid is read-only context, not a picker: the prototype's
                 calDays loop renders plain <span> cells with no onClick (unlike the
                 preset loop, which binds one), so the actual editing surface is the
                 preset rail and the two fields above. This grid just visualises the
                 range they produce. -->
            <div class="dr-pop__grid" aria-hidden="true">
              <span
                v-for="day in calendarDays"
                :key="day.iso"
                class="dr-pop__day"
                :data-out-of-month="!day.inMonth || undefined"
                :data-in-range="day.inRange || undefined"
                :data-start="day.isStart || undefined"
                :data-end="day.isEnd || undefined"
              >{{ day.n }}</span>
            </div>

            <div class="dr-pop__footer">
              <span class="dr-pop__span">{{ spanLabel }}</span>
              <span class="dr-pop__actions">
                <Button variant="ghost" size="sm" type="button" class="dr-pop__cancel" @click="close">
                  {{ t('common.cancel') }}
                </Button>
                <Button variant="solid" size="sm" type="button" class="dr-pop__apply date-picker-apply" @click="apply">
                  {{ t('dates.apply') }}
                </Button>
              </span>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
/**
 * DateRangePicker — part 02, section 11.
 *
 * Migration note from the prototype:
 *   "Presets move from a row above the calendar to a rail beside it, and the trigger
 *    starts naming the preset. activePreset already exists in the source, it just is
 *    not surfaced."
 *
 * The prop, emit and slot surface is byte-for-byte what it was: `startDate`,
 * `endDate`, `update:startDate`, `update:endDate`, `change`. This is a presentation
 * rewrite, so every existing call site keeps working untouched.
 *
 * Trigger sits in filter bars beside 32px toolbar buttons, not 36px form fields, so
 * it takes --s2a-h-sm (32px) per the prototype's printed height, not Input.vue's
 * --s2a-h-md.
 *
 * The popover reuses Select.vue's chrome verbatim: teleport to body, `position: fixed`
 * off the container's rect, 1px --popover-border, --popover fill, --shadow-md,
 * --r-md shell, --r-sm rows, top/bottom flip and scroll/resize tracking. --t-normal
 * from the prototype's other migration notes does not exist in 01-TOKENS; like
 * Select.vue, the popover transition and caret rotation use --t-med, the nearest
 * token to the 200ms default they replace.
 *
 * `.date-picker-trigger`, `.date-picker-preset` and `.date-picker-apply` are kept as
 * extra classes (not renamed away) because
 * src/components/common/__tests__/DateRangePicker.spec.ts drives the component
 * through them directly.
 */
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from './Button.vue'

const { t, locale } = useI18n()

interface DatePreset {
  labelKey: string
  value: string
  getRange: () => { start: string; end: string }
}

interface Props {
  startDate: string
  endDate: string
}

interface Emits {
  (e: 'update:startDate', value: string): void
  (e: 'update:endDate', value: string): void
  (e: 'change', range: { startDate: string; endDate: string; preset: string | null }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

// Instance ID for unique click-outside detection on the teleported portal, same
// pattern as Select.vue.
const instanceId = `date-range-${Math.random().toString(36).substring(2, 9)}`

const isOpen = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const popoverRef = ref<HTMLElement | null>(null)

// Draft state: mirrors props while closed, edits freely while open, and is only
// written back via `apply`. Closing any other way (Cancel, outside click, Escape,
// re-clicking the trigger) discards it.
const localStart = ref(props.startDate)
const localEnd = ref(props.endDate)

const ISO_RE = /^\d{4}-\d{2}-\d{2}$/

const formatDateToString = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const today = computed(() => formatDateToString(new Date()))

// Tomorrow's date, used to cap manual field edits so a user in a timezone behind the
// server can still reach "today" there. Preserved from the pre-rewrite component.
const tomorrow = computed(() => {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return formatDateToString(d)
})

const presets: DatePreset[] = [
  {
    labelKey: 'dates.today',
    value: 'today',
    getRange: () => {
      const t = today.value
      return { start: t, end: t }
    }
  },
  {
    labelKey: 'dates.yesterday',
    value: 'yesterday',
    getRange: () => {
      const d = new Date()
      d.setDate(d.getDate() - 1)
      const yesterday = formatDateToString(d)
      return { start: yesterday, end: yesterday }
    }
  },
  {
    labelKey: 'dates.last24Hours',
    value: 'last24Hours',
    getRange: () => {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
      return { start: formatDateToString(start), end: formatDateToString(end) }
    }
  },
  {
    labelKey: 'dates.last7Days',
    value: '7days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 6)
      return { start: formatDateToString(d), end }
    }
  },
  {
    labelKey: 'dates.last14Days',
    value: '14days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 13)
      return { start: formatDateToString(d), end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: '30days',
    getRange: () => {
      const end = today.value
      const d = new Date()
      d.setDate(d.getDate() - 29)
      return { start: formatDateToString(d), end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 1))
      return { start, end: today.value }
    }
  },
  {
    labelKey: 'dates.lastMonth',
    value: 'lastMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth() - 1, 1))
      const end = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 0))
      return { start, end }
    }
  }
]

const detectPreset = (start: string, end: string): string | null => {
  if (!start || !end) return null
  for (const preset of presets) {
    const range = preset.getRange()
    if (range.start === start && range.end === end) return preset.value
  }
  return null
}

// Trigger label reflects the applied props, not the in-progress draft: "trigger
// states the preset, not the raw dates" per the prototype's caption, and it should
// not change while the popover is open and unapplied.
const committedPreset = computed(() => detectPreset(props.startDate, props.endDate))
// Rail highlight reflects the draft, since it previews what Apply will commit.
const draftPreset = computed(() => detectPreset(localStart.value, localEnd.value))

const displayValue = computed(() => {
  if (committedPreset.value) {
    const preset = presets.find((p) => p.value === committedPreset.value)
    if (preset) return t(preset.labelKey)
  }
  if (props.startDate && props.endDate) {
    // Upstream shows the actual dates, not the word "Custom" -- the trigger is the
    // only place a hand-picked range is legible once the popover is closed.
    if (props.startDate === props.endDate) return formatTriggerDate(props.startDate)
    return `${formatTriggerDate(props.startDate)} - ${formatTriggerDate(props.endDate)}`
  }
  return t('dates.selectDateRange')
})

const formatTriggerDate = (dateStr: string): string => {
  const date = new Date(dateStr + 'T00:00:00')
  return date.toLocaleDateString(locale.value === 'zh' ? 'zh-CN' : 'en-US', {
    month: 'short',
    day: 'numeric'
  })
}

const spanLabel = computed(() => {
  if (!localStart.value || !localEnd.value) return ''
  const start = new Date(localStart.value + 'T00:00:00')
  const end = new Date(localEnd.value + 'T00:00:00')
  const diffDays = Math.round((end.getTime() - start.getTime()) / 86400000) + 1
  const days = Math.max(diffDays, 1)
  // vue-i18n v9 does not pluralise from a named `count` alone -- the third
  // argument is what selects the branch of a `singular | plural` message.
  return t('dates.daySpan', { count: days }, days)
})

// v-model.lazy computed setters: commit on change/blur, not per keystroke. An
// invalid or out-of-order entry is simply dropped, same "no error UI on a rejected
// edit" spirit as a gated control -- the field just keeps its last good value.
const fromModel = computed({
  get: () => localStart.value,
  set: (value: string) => {
    const v = value.trim()
    if (!ISO_RE.test(v) || v > tomorrow.value) return
    localStart.value = v
    if (localEnd.value && v > localEnd.value) localEnd.value = v
  }
})

const toModel = computed({
  get: () => localEnd.value,
  set: (value: string) => {
    const v = value.trim()
    if (!ISO_RE.test(v) || v > tomorrow.value) return
    localEnd.value = v
    if (localStart.value && v < localStart.value) localStart.value = v
  }
})

// Read-only month grid: shows the month containing the range's end (falling back to
// its start, then today), highlighting the draft range. 35 cells (5 weeks) per the
// prototype's hint-placeholder-count.
const calendarDays = computed(() => {
  const base = localEnd.value || localStart.value
  const anchor = base ? new Date(base + 'T00:00:00') : new Date()
  const year = anchor.getFullYear()
  const month = anchor.getMonth()
  const firstOfMonth = new Date(year, month, 1)
  const gridStart = new Date(year, month, 1 - firstOfMonth.getDay())

  const days: Array<{
    n: number
    iso: string
    inMonth: boolean
    inRange: boolean
    isStart: boolean
    isEnd: boolean
  }> = []

  for (let i = 0; i < 35; i++) {
    const d = new Date(gridStart)
    d.setDate(gridStart.getDate() + i)
    const iso = formatDateToString(d)
    const isStart = !!localStart.value && iso === localStart.value
    const isEnd = !!localEnd.value && iso === localEnd.value
    const inRange =
      !!localStart.value && !!localEnd.value && iso > localStart.value && iso < localEnd.value
    days.push({ n: d.getDate(), iso, inMonth: d.getMonth() === month, inRange, isStart, isEnd })
  }
  return days
})

const selectPreset = (preset: DatePreset) => {
  const range = preset.getRange()
  localStart.value = range.start
  localEnd.value = range.end
}

const apply = () => {
  emit('update:startDate', localStart.value)
  emit('update:endDate', localEnd.value)
  emit('change', {
    startDate: localStart.value,
    endDate: localEnd.value,
    preset: draftPreset.value
  })
  isOpen.value = false
}

// Discard the draft and close. Used by Cancel, outside click, Escape and re-clicking
// the trigger to close: all of them are "dismiss without applying."
const close = () => {
  localStart.value = props.startDate
  localEnd.value = props.endDate
  isOpen.value = false
}

const toggle = () => {
  if (isOpen.value) {
    close()
  } else {
    isOpen.value = true
  }
}

// --- popover positioning, mirrored from Select.vue ------------------------------
const dropdownPosition = ref<'bottom' | 'top'>('bottom')
const triggerRect = ref<DOMRect | null>(null)
const viewportPadding = 8
const popoverWidth = 372

const popoverStyle = computed(() => {
  if (!triggerRect.value) return {}
  const rect = triggerRect.value
  const viewportRight = Math.max(viewportPadding, window.innerWidth - viewportPadding)
  const left = Math.min(Math.max(viewportPadding, rect.left), viewportRight - popoverWidth)
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${Math.max(viewportPadding, left)}px`,
    width: `${popoverWidth}px`,
    zIndex: '100000020'
  }
  if (dropdownPosition.value === 'top') {
    style.bottom = `${window.innerHeight - rect.top + 4}px`
  } else {
    style.top = `${rect.bottom + 4}px`
  }
  return style
})

const updateTriggerRect = () => {
  if (containerRef.value) {
    triggerRect.value = containerRef.value.getBoundingClientRect()
  }
}

const calculatePosition = () => {
  if (!containerRef.value) return
  updateTriggerRect()
  nextTick(() => {
    if (!popoverRef.value || !triggerRect.value) return
    const height = popoverRef.value.offsetHeight || 320
    const spaceBelow = window.innerHeight - triggerRect.value.bottom
    const spaceAbove = triggerRect.value.top
    dropdownPosition.value = spaceBelow < height && spaceAbove > height ? 'top' : 'bottom'
  })
}

watch(isOpen, (open) => {
  if (open) {
    calculatePosition()
    window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
    window.addEventListener('resize', calculatePosition)
  } else {
    window.removeEventListener('scroll', updateTriggerRect, { capture: true })
    window.removeEventListener('resize', calculatePosition)
  }
})

// Sync draft with props whenever the parent changes them, same as before the
// rewrite (a v-model change from outside wins over an untouched draft).
watch(
  () => props.startDate,
  (v) => (localStart.value = v)
)
watch(
  () => props.endDate,
  (v) => (localEnd.value = v)
)

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  const isInPopover = !!target.closest(`.${instanceId}`)
  const isInTrigger = containerRef.value?.contains(target)
  if (!isInPopover && !isInTrigger && isOpen.value) {
    close()
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    close()
    triggerRef.value?.focus()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
  window.removeEventListener('scroll', updateTriggerRect, { capture: true })
  window.removeEventListener('resize', calculatePosition)
})
</script>

<style scoped>
.dr {
  position: relative;
  display: inline-block;
}

/* --- trigger ---------------------------------------------------------------
   Toolbar tier, not form-field tier: --s2a-h-sm (32px), matching the 32px buttons
   this sits beside in a filter bar, per the prototype's printed height. */
.dr-trigger {
  display: grid;
  grid-template-columns: 15px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  height: var(--s2a-h-sm);
  padding: 0 9px 0 10px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  cursor: pointer;
}

.dr-trigger__icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
}

.dr-trigger__value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: left;
}

.dr-trigger__caret {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
  transition: transform var(--t-med) var(--ease-out);
}
.dr-trigger__caret--open {
  transform: rotate(180deg);
}

/* Static swap on open, same as Select's [data-open]: colour changes, never
   transitions -- ground rule 6. */
.dr-trigger[data-open] {
  border-color: var(--ring-focus);
}

.dr-trigger:focus-visible {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

/* --- popover ----------------------------------------------------------------
   Same chrome as Select.vue's dropdown: 1px --popover-border, --popover fill,
   --shadow-md, --r-md shell. Fixed width: unlike Select's option list, this
   popover's content (two fields + a 7-column grid) doesn't reflow to content
   length, so it does not need Select's min/max-width viewport math. */
.dr-pop {
  display: grid;
  grid-template-columns: 132px minmax(0, 1fr);
  border: 1px solid var(--popover-border);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-md);
  overflow: hidden;
}

.dr-pop__rail {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 5px;
  border-inline-end: 1px solid var(--border-subtle);
  background: var(--surface-subtle);
}

.dr-pop__preset {
  display: block;
  width: 100%;
  min-height: 26px;
  padding: 4px 8px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
  transition: background var(--motion-hover);
}
.dr-pop__preset:hover:not([data-active]) {
  background: var(--sidebar-accent);
}
/* Selected preset: --brand-tint fill, --warm-strong ink, per the anatomy spec. */
.dr-pop__preset[data-active] {
  background: var(--brand-tint);
  color: var(--warm-strong);
}

.dr-pop__body {
  padding: 12px 14px;
}

.dr-pop__fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.dr-pop__field {
  display: block;
}

.dr-pop__field-label {
  display: block;
  margin-bottom: 5px;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

/* Mono, not a native type="date": the prototype's own popover IS the date-picker
   UI, so the fields are structured-data text entry, not a second native widget. */
.dr-pop__field-input {
  width: 100%;
  min-width: 0;
  height: 30px;
  padding: 0 8px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
}
.dr-pop__field-input:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.dr-pop__grid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  gap: 2px;
  margin-top: 12px;
}

.dr-pop__day {
  display: grid;
  place-items: center;
  height: 24px;
  border-radius: var(--r-sm);
  color: var(--foreground);
  font-size: var(--fs-sm);
}
.dr-pop__day[data-out-of-month] {
  color: var(--muted-foreground);
  opacity: 0.55;
}
/* Range fill: --brand-tint, squared so the run of days reads as one bar. */
.dr-pop__day[data-in-range] {
  background: var(--brand-tint);
  border-radius: 0;
}
/* Endpoints: solid --brand + --on-solid, rounded only on their outer edge so they
   cap the bar instead of breaking it. */
.dr-pop__day[data-start] {
  background: var(--brand);
  color: var(--on-solid);
  border-radius: var(--r-sm) 0 0 var(--r-sm);
}
.dr-pop__day[data-end] {
  background: var(--brand);
  color: var(--on-solid);
  border-radius: 0 var(--r-sm) var(--r-sm) 0;
}
.dr-pop__day[data-start][data-end] {
  border-radius: var(--r-sm);
}

.dr-pop__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px solid var(--border-subtle);
}

.dr-pop__span {
  flex-shrink: 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
  white-space: nowrap;
}

.dr-pop__actions {
  display: flex;
  gap: 6px;
}

.dr-pop__cancel {
  height: var(--s2a-h-xs);
  padding: 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-md);
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.dr-pop__cancel:hover {
  background: var(--sidebar-accent);
}

.dr-pop__apply {
  height: var(--s2a-h-xs);
  padding: 0 11px;
  border: 0;
  border-radius: var(--r-md);
  background: var(--primary);
  color: var(--primary-foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.dr-pop__apply:hover {
  background: color-mix(in oklch, var(--primary) 92%, var(--foreground));
}
.dr-pop__apply:active {
  background: color-mix(in oklch, var(--primary) 84%, var(--foreground));
}

/* Enter/leave: --t-normal named in the migration notes elsewhere in this spec does
   not exist; --t-med is the nearest token, same substitution Select.vue makes. */
.dr-pop-enter-active,
.dr-pop-leave-active {
  transition: opacity var(--t-med) var(--ease-out), transform var(--t-med) var(--ease-out);
}
.dr-pop-enter-from,
.dr-pop-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

@media (prefers-reduced-motion: reduce) {
  .dr-trigger__caret,
  .dr-pop-enter-active,
  .dr-pop-leave-active {
    transition: none;
  }
  .dr-pop-enter-from,
  .dr-pop-leave-to {
    transform: none;
  }
}
</style>
