<template>
  <!-- ref="containerRef" is used both for outside-click detection and for the
       teleported popover's getBoundingClientRect anchor, so it stays a plain
       block wrapper: the popover positions itself with `position: fixed`, not
       by being an absolutely-positioned child of this element. -->
  <div class="select" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger"
      :data-open="isOpen || undefined"
      :data-invalid="error ? '' : undefined"
      @click="toggle"
      :disabled="disabled"
      :aria-expanded="isOpen"
      :aria-haspopup="true"
      :aria-invalid="error ? true : undefined"
      :aria-disabled="disabled || undefined"
      :id="id"
      :aria-label="ariaLabel ?? 'Select option'"
      :aria-describedby="ariaDescribedby"
      @keydown.down.prevent="onTriggerKeyDown"
      @keydown.up.prevent="onTriggerKeyDown"
    >
      <span class="select-trigger__value">
        <slot name="selected" :option="selectedOption">
          {{ selectedLabel }}
        </slot>
      </span>
      <span
        v-if="clearable && hasValue && !disabled"
        class="select-trigger__clear"
        role="button"
        tabindex="-1"
        aria-label="Clear selection"
        @click.stop="clearSelection"
        @mousedown.stop
        @keydown.enter.stop.prevent="clearSelection"
      >
        <i class="hgi-stroke hgi-cancel-01" aria-hidden="true" />
      </span>
      <!-- Icon glyph, fixed chrome: sized in px, not a --fs-* token. -->
      <i
        class="hgi-stroke hgi-arrow-down-01 select-trigger__caret"
        :class="isOpen && 'select-trigger__caret--open'"
        aria-hidden="true"
      />
    </button>

    <!-- Teleport dropdown to body to escape stacking context -->
    <Teleport to="body">
      <Transition name="select-pop">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="select-dropdown-portal"
          :class="[instanceId]"
          :style="dropdownStyle"
          role="listbox"
          @click.stop
          @mousedown.stop
          @keydown="onDropdownKeyDown"
        >
          <!-- Search input -->
          <div v-if="isSearchable" class="select-pop__search">
            <i class="hgi-stroke hgi-search-01" aria-hidden="true" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              :placeholder="searchPlaceholderText"
              :aria-label="searchPlaceholderText"
              class="select-pop__search-input"
              @click.stop
            />
          </div>

          <!-- Options list, capped at 184px per the prototype anatomy -->
          <div class="select-pop__list" ref="optionsListRef">
            <div
              v-for="(option, index) in filteredOptions"
              :key="`${typeof getOptionValue(option)}:${String(getOptionValue(option) ?? '')}`"
              role="option"
              :aria-selected="isSelected(option)"
              :aria-disabled="isOptionDisabled(option)"
              @click.stop="!isOptionDisabled(option) && !isGroupHeaderOption(option) && selectOption(option)"
              @mouseenter="handleOptionMouseEnter(option, index)"
              class="select-pop__option"
              :data-group="isGroupHeaderOption(option) || undefined"
              :data-selected="isSelected(option) || undefined"
              :data-disabled="(isOptionDisabled(option) && !isGroupHeaderOption(option)) || undefined"
              :data-focused="(focusedIndex === index && !isGroupHeaderOption(option)) || undefined"
            >
              <slot name="option" :option="option" :selected="isSelected(option)">
                <span
                  class="select-pop__option-label"
                  :class="option._creatable && 'select-pop__option-label--creatable'"
                >{{ getOptionLabel(option) }}</span>
                <i
                  v-if="isSelected(option)"
                  class="hgi-stroke hgi-tick-01 select-pop__check"
                  aria-hidden="true"
                />
              </slot>
            </div>

            <!-- Empty state -->
            <div v-if="filteredOptions.length === 0" class="select-pop__empty">
              {{ emptyTextDisplay }}
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
/**
 * Select — part 02, section 05.
 *
 * The most capable control in the app: teleported dropdown, auto search above
 * five options, creatable entries, clearable, group headers, keyboard
 * navigation. This is a presentation rewrite; every prop in the prototype's
 * table is marked "unchanged", so the teleport, the click-outside handling
 * and all keyboard handling below are untouched. Only markup and styling move.
 *
 * Migration note from the prototype:
 *   "Keep the teleport and the keyboard handling. Remove the rotate-180 on a
 *    200ms default and use --t-normal --ease-out. Selected rows stop using
 *    text-primary-500. The creatable row keeps its italic but loses the
 *    search icon, which read as a second action."
 *
 * `--t-normal` does not exist in 01-TOKENS (--t-fast/--t-med/--t-slow do); the
 * caret and popover motion below use --t-med (160ms), the nearest token to the
 * 200ms default being replaced.
 */
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

// Instance ID for unique click-outside detection
const instanceId = `select-${Math.random().toString(36).substring(2, 9)}`

export interface SelectOption {
  value: string | number | boolean | null
  label: string
  disabled?: boolean
  [key: string]: unknown
}

interface Props {
  modelValue: string | number | boolean | null | undefined
  options: SelectOption[] | Array<Record<string, unknown>>
  placeholder?: string
  disabled?: boolean
  error?: boolean
  searchable?: boolean | 'auto'
  searchPlaceholder?: string
  emptyText?: string
  valueKey?: string
  labelKey?: string
  creatable?: boolean
  creatablePrefix?: string
  clearable?: boolean
  id?: string
  ariaLabel?: string
  ariaDescribedby?: string
}

interface Emits {
  (e: 'update:modelValue', value: string | number | boolean | null): void
  (e: 'change', value: string | number | boolean | null, option: SelectOption | null): void
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  error: false,
  searchable: 'auto',
  creatable: false,
  creatablePrefix: '',
  clearable: false,
  valueKey: 'value',
  labelKey: 'label'
})

const emit = defineEmits<Emits>()

const isOpen = ref(false)
const searchQuery = ref('')
const focusedIndex = ref(-1)
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const optionsListRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<'bottom' | 'top'>('bottom')
const triggerRect = ref<DOMRect | null>(null)
const dropdownViewportPadding = 8
const dropdownMinimumWidth = 200

// i18n placeholders
const placeholderText = computed(() => props.placeholder ?? t('common.selectOption'))
const searchPlaceholderText = computed(() => props.searchPlaceholder ?? t('common.searchPlaceholder'))
const emptyTextDisplay = computed(() => props.emptyText ?? t('common.noOptionsFound'))

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.options.length > 5
  return props.searchable
})

// Computed style for teleported dropdown
const dropdownStyle = computed(() => {
  if (!triggerRect.value) return {}

  const rect = triggerRect.value
  const viewportRight = Math.max(dropdownViewportPadding, window.innerWidth - dropdownViewportPadding)
  const left = Math.min(
    Math.max(dropdownViewportPadding, rect.left),
    viewportRight
  )
  const availableWidth = Math.max(0, viewportRight - left)
  const preferredMinWidth = Math.max(dropdownMinimumWidth, rect.width)
  const minWidth = Math.min(preferredMinWidth, availableWidth)
  const style: Record<string, string> = {
    position: 'fixed',
    left: `${left}px`,
    minWidth: `${minWidth}px`,
    maxWidth: `${availableWidth}px`,
    zIndex: '100000020'
  }

  if (dropdownPosition.value === 'top') {
    style.bottom = `${window.innerHeight - rect.top + 4}px`
  } else {
    style.top = `${rect.bottom + 4}px`
  }

  return style
})

const getOptionValue = (option: any): any => {
  if (typeof option === 'object' && option !== null) {
    return option[props.valueKey]
  }
  return option
}

const getOptionLabel = (option: any): string => {
  if (typeof option === 'object' && option !== null) {
    return String(option[props.labelKey] ?? '')
  }
  return String(option ?? '')
}

const isOptionDisabled = (option: any): boolean => {
  if (typeof option === 'object' && option !== null) {
    return !!option.disabled
  }
  return false
}

const isGroupHeaderOption = (option: any): boolean => {
  if (typeof option === 'object' && option !== null) {
    return option.kind === 'group'
  }
  return false
}

const selectedOption = computed(() => {
  return props.options.find((opt) => getOptionValue(opt) === props.modelValue) || null
})

const selectedLabel = computed(() => {
  if (selectedOption.value) {
    return getOptionLabel(selectedOption.value)
  }
  // In creatable mode, show the raw value if no matching option
  if (props.creatable && props.modelValue) {
    return String(props.modelValue)
  }
  return placeholderText.value
})

const hasValue = computed(
  () => props.modelValue !== null && props.modelValue !== undefined && props.modelValue !== ''
)

const filteredOptions = computed(() => {
  let opts = props.options as any[]
  if (isSearchable.value && searchQuery.value) {
    const query = searchQuery.value.toLowerCase()
    opts = opts.filter((opt) => {
      // Match label
      if (getOptionLabel(opt).toLowerCase().includes(query)) return true
      // Also match description if present
      if (opt.description && String(opt.description).toLowerCase().includes(query)) return true
      return false
    })
    // In creatable mode, always prepend a fuzzy search option
    if (props.creatable && searchQuery.value.trim()) {
      const trimmed = searchQuery.value.trim()
      const prefix = props.creatablePrefix || t('common.search')
      opts = [{ [props.valueKey]: trimmed, [props.labelKey]: `${prefix} "${trimmed}"`, _creatable: true }, ...opts]
    }
  }
  return opts
})

const isSelected = (option: any): boolean => {
  return getOptionValue(option) === props.modelValue
}

const findNextEnabledIndex = (startIndex: number): number => {
  const opts = filteredOptions.value
  if (opts.length === 0) return -1
  for (let offset = 0; offset < opts.length; offset++) {
    const idx = (startIndex + offset) % opts.length
    if (!isOptionDisabled(opts[idx])) return idx
  }
  return -1
}

const findPrevEnabledIndex = (startIndex: number): number => {
  const opts = filteredOptions.value
  if (opts.length === 0) return -1
  for (let offset = 0; offset < opts.length; offset++) {
    const idx = (startIndex - offset + opts.length) % opts.length
    if (!isOptionDisabled(opts[idx])) return idx
  }
  return -1
}

const handleOptionMouseEnter = (option: any, index: number) => {
  if (isOptionDisabled(option) || isGroupHeaderOption(option)) return
  focusedIndex.value = index
}

// Update trigger rect periodically while open to follow scroll/resize
const updateTriggerRect = () => {
  if (containerRef.value) {
    triggerRect.value = containerRef.value.getBoundingClientRect()
  }
}

const calculateDropdownPosition = () => {
  if (!containerRef.value) return
  updateTriggerRect()

  nextTick(() => {
    if (!dropdownRef.value || !triggerRect.value) return
    const dropdownHeight = dropdownRef.value.offsetHeight || 240
    const spaceBelow = window.innerHeight - triggerRect.value.bottom
    const spaceAbove = triggerRect.value.top

    if (spaceBelow < dropdownHeight && spaceAbove > dropdownHeight) {
      dropdownPosition.value = 'top'
    } else {
      dropdownPosition.value = 'bottom'
    }
  })
}

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
}

watch(isOpen, (open) => {
  if (open) {
    calculateDropdownPosition()
    // Reset focused index to current selection or first item
    if (filteredOptions.value.length === 0) {
      focusedIndex.value = -1
    } else {
      const selectedIdx = filteredOptions.value.findIndex(isSelected)
      const initialIdx = selectedIdx >= 0 ? selectedIdx : 0
      focusedIndex.value = isOptionDisabled(filteredOptions.value[initialIdx])
        ? findNextEnabledIndex(initialIdx + 1)
        : initialIdx
    }

    if (isSearchable.value) {
      nextTick(() => searchInputRef.value?.focus())
    }
    // Add scroll listener to update position
    window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
    window.addEventListener('resize', calculateDropdownPosition)
  } else {
    searchQuery.value = ''
    focusedIndex.value = -1
    window.removeEventListener('scroll', updateTriggerRect, { capture: true })
    window.removeEventListener('resize', calculateDropdownPosition)
  }
})

const selectOption = (option: any) => {
  const value = getOptionValue(option) ?? null
  emit('update:modelValue', value)
  emit('change', value, option)
  isOpen.value = false
  triggerRef.value?.focus()
}

const clearSelection = () => {
  if (props.disabled) return
  emit('update:modelValue', null)
  emit('change', null, null)
}

// Keyboards
const onTriggerKeyDown = () => {
  if (!isOpen.value) {
    isOpen.value = true
  }
}

const onDropdownKeyDown = (e: KeyboardEvent) => {
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      focusedIndex.value = findNextEnabledIndex(focusedIndex.value + 1)
      if (focusedIndex.value >= 0) scrollToFocused()
      break
    case 'ArrowUp':
      e.preventDefault()
      focusedIndex.value = findPrevEnabledIndex(focusedIndex.value - 1)
      if (focusedIndex.value >= 0) scrollToFocused()
      break
    case 'Enter':
      e.preventDefault()
      if (focusedIndex.value >= 0 && focusedIndex.value < filteredOptions.value.length) {
        const opt = filteredOptions.value[focusedIndex.value]
        if (!isOptionDisabled(opt)) selectOption(opt)
      }
      break
    case 'Escape':
      e.preventDefault()
      isOpen.value = false
      triggerRef.value?.focus()
      break
    case 'Tab':
      isOpen.value = false
      break
  }
}

const scrollToFocused = () => {
  nextTick(() => {
    const list = optionsListRef.value
    if (!list) return
    const focusedEl = list.children[focusedIndex.value] as HTMLElement
    if (!focusedEl) return

    if (focusedEl.offsetTop < list.scrollTop) {
      list.scrollTop = focusedEl.offsetTop
    } else if (focusedEl.offsetTop + focusedEl.offsetHeight > list.scrollTop + list.offsetHeight) {
      list.scrollTop = focusedEl.offsetTop + focusedEl.offsetHeight - list.offsetHeight
    }
  })
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // Check if click is inside THIS specific instance's dropdown or trigger
  const isInDropdown = !!target.closest(`.${instanceId}`)
  const isInTrigger = containerRef.value?.contains(target)

  if (!isInDropdown && !isInTrigger && isOpen.value) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('scroll', updateTriggerRect, { capture: true })
  window.removeEventListener('resize', calculateDropdownPosition)
})
</script>

<style scoped>
/* No styling of its own: the popover is fixed-positioned off the trigger's
   getBoundingClientRect, not absolutely positioned off this wrapper. */
.select {
  position: relative;
  width: 100%;
}

/* --- trigger --------------------------------------------------------------
   Modeled on Input's .fld__input: a bordered field with focus/invalid/
   disabled states and, unlike a row or a button, no hover treatment. Ground
   rule 6 also rules out the old hover:border-gray-300 -- borders are static
   chrome and only background may transition. */
.select-trigger {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 6px;
  width: 100%;
  height: var(--s2a-h-md);
  padding: 0 9px 0 11px;
  border: 1px solid var(--input);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  text-align: left;
  cursor: pointer;
}

.select-trigger__value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Open mirrors the prototype's sel.border binding: the ring-focus border,
   independent of whether the trigger also has keyboard focus. */
.select-trigger[data-open] {
  border-color: var(--ring-focus);
}

.select-trigger[data-invalid] {
  border-color: var(--destructive);
}

.select-trigger:focus-visible {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.select-trigger:disabled,
.select-trigger[aria-disabled='true'] {
  opacity: 0.55;
  cursor: not-allowed;
}

.select-trigger__clear {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  border-radius: var(--r-xs);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.select-trigger__clear:hover {
  background: var(--sidebar-accent);
}
.select-trigger__clear i {
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph */
}

.select-trigger__caret {
  flex-shrink: 0;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
  /* Migration: "remove the rotate-180 on a 200ms default and use --t-normal
     --ease-out". --t-normal does not exist; --t-med is the nearest token to
     the 200ms it replaces. */
  transition: transform var(--t-med) var(--ease-out);
}
.select-trigger__caret--open {
  transform: rotate(180deg);
}

/* --- popover ---------------------------------------------------------- */

.select-dropdown-portal {
  border: 1px solid var(--popover-border);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-md);
  overflow: hidden;
  pointer-events: auto !important;
}

.select-pop__search {
  display: grid;
  grid-template-columns: 15px minmax(0, 1fr);
  align-items: center;
  gap: 7px;
  padding: 7px 10px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--muted-foreground);
}
.select-pop__search i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
}

.select-pop__search-input {
  width: 100%;
  border: 0;
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  outline: none;
}
.select-pop__search-input::placeholder {
  color: var(--muted-foreground);
}

/* Capped at 184px per the anatomy spec, so a long option list scrolls inside
   the popover rather than pushing it off screen. */
.select-pop__list {
  max-height: 184px;
  overflow-y: auto;
  padding: 4px;
  outline: none;
}

.select-pop__option {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  min-height: 30px;
  padding: 5px 8px;
  border-radius: var(--r-sm);
  color: var(--foreground);
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover);
  pointer-events: auto !important;
}

/* Hover and selected share one token: colour is spent on state, not doubled
   up between "you're pointing at this" and "this is the value". Selected text
   stays --foreground -- the migration note retires text-primary-500, leaving
   only the check glyph carrying colour. */
.select-pop__option:hover:not([data-disabled]):not([data-group]),
.select-pop__option[data-focused],
.select-pop__option[data-selected] {
  background: var(--sidebar-accent);
}

.select-pop__option[data-disabled] {
  cursor: not-allowed;
  opacity: 0.55;
}

/* Group headers are inert: no hover, no pointer, no click target. */
.select-pop__option[data-group] {
  min-height: 22px;
  padding: 3px 8px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: default;
  user-select: none;
}
.select-pop__option[data-group]:hover {
  background: transparent;
}

.select-pop__option-label {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Creatable row keeps its italic; the migration note drops the leading search
   glyph, which "read as a second action" next to the trigger's own search row. */
.select-pop__option-label--creatable {
  color: var(--muted-foreground);
  font-style: italic;
}

.select-pop__check {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--warm-strong);
}

.select-pop__empty {
  padding: 12px 8px;
  text-align: center;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* Enter/leave: explicit properties, never `transition: all` (ground rule 6
   only names border-color, but `all` is the same footgun for any property).
   --t-med --ease-out replaces the 200ms default the migration note calls out. */
.select-pop-enter-active,
.select-pop-leave-active {
  transition: opacity var(--t-med) var(--ease-out), transform var(--t-med) var(--ease-out);
}
.select-pop-enter-from,
.select-pop-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

@media (prefers-reduced-motion: reduce) {
  .select-trigger__caret,
  .select-pop-enter-active,
  .select-pop-leave-active {
    transition: none;
  }
  .select-pop-enter-from,
  .select-pop-leave-to {
    transform: none;
  }
}
</style>
