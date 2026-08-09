<template>
  <!-- containerRef anchors the teleported popover via getBoundingClientRect,
       same split as Select.vue: this wrapper stays a plain block, the popover
       positions itself `position: fixed` rather than as an absolute child. -->
  <div class="pxsel" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger"
      :data-open="isOpen || undefined"
      :disabled="disabled"
      :aria-expanded="isOpen"
      :aria-haspopup="true"
      :aria-disabled="disabled || undefined"
      @click="toggle"
      @keydown.down.prevent="onTriggerKeyDown"
      @keydown.up.prevent="onTriggerKeyDown"
    >
      <span class="select-trigger__value">
        <template v-if="selectedProxy">
          <span class="pxsel-trigger__name">{{ selectedProxy.name }}</span>
          <span class="pxsel-trigger__addr">{{ selectedProxy.protocol }}://{{ selectedProxy.host }}:{{ selectedProxy.port }}</span>
        </template>
        <span v-else>{{ t('admin.accounts.noProxy') }}</span>
      </span>
      <i
        class="hgi-stroke hgi-arrow-down-01 select-trigger__caret"
        :class="isOpen && 'select-trigger__caret--open'"
        aria-hidden="true"
      />
    </button>

    <Teleport to="body">
      <Transition name="select-pop">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="pxsel-portal"
          :class="[instanceId]"
          :style="dropdownStyle"
          role="listbox"
          @click.stop
          @mousedown.stop
          @keydown="onDropdownKeyDown"
        >
          <div class="pxsel-portal__header">
            <i class="hgi-stroke hgi-search-01" aria-hidden="true" />
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.proxies.searchProxies')"
              :aria-label="t('admin.proxies.searchProxies')"
              class="pxsel-portal__search"
              @click.stop
            />
            <button
              v-if="proxies.length > 0"
              type="button"
              class="pxsel-portal__test-all"
              :disabled="batchTesting"
              @click.stop="handleBatchTest"
            >
              <span v-if="batchTesting" class="pxsel-spinner" aria-hidden="true" />
              <i v-else class="hgi-stroke hgi-play" aria-hidden="true" />
              <span>{{ t('admin.proxies.testAll') }}</span>
            </button>
          </div>

          <div class="pxsel-portal__list" ref="optionsListRef">
            <div
              v-for="(proxy, index) in filteredProxies"
              :key="proxy.id"
              role="option"
              :aria-selected="modelValue === proxy.id"
              class="pxsel-row"
              :data-selected="modelValue === proxy.id || undefined"
              :data-focused="focusedIndex === index || undefined"
              :title="`${proxy.protocol}://${proxy.host}:${proxy.port}`"
              @click="selectOption(proxy.id)"
              @mouseenter="focusedIndex = index"
            >
              <span class="pxsel-row__dot-box" :data-checked="modelValue === proxy.id || undefined" aria-hidden="true">
                <span class="pxsel-row__dot" />
              </span>

              <span class="pxsel-row__meta">
                <!-- Truncate from the start: the tail (the end of a hostname)
                     is what's meaningful, not the front. Container flips to
                     rtl so ellipsis lands on the left; the inner span stays
                     ltr so the host itself doesn't read backwards. -->
                <span class="pxsel-row__host">
                  <span class="pxsel-row__host-inner">{{ proxy.host }}</span>
                </span>
                <span class="pxsel-row__sub">{{ proxyMeta(proxy) }}</span>
              </span>

              <span class="pxsel-row__result" :style="{ color: resultColor(proxy) }">
                <span v-if="testingProxyIds.has(proxy.id)" class="pxsel-spinner" aria-hidden="true" />
                <span v-else-if="testResults[proxy.id]" class="pxsel-row__result-dot" aria-hidden="true" />
                <span class="pxsel-row__result-text">{{ resultText(proxy) }}</span>
              </span>

              <button
                type="button"
                class="pxsel-row__test"
                :disabled="testingProxyIds.has(proxy.id)"
                :title="t('admin.proxies.testConnection')"
                @click.stop="handleTestProxy(proxy)"
              >
                <span v-if="testingProxyIds.has(proxy.id)" class="pxsel-spinner" aria-hidden="true" />
                <i v-else class="hgi-stroke hgi-play" aria-hidden="true" />
              </button>
            </div>

            <div v-if="filteredProxies.length === 0 && searchQuery" class="pxsel-portal__empty">
              {{ t('common.noOptionsFound') }}
            </div>
          </div>

          <!-- Closes the list, per the prototype's anatomy: a fixed row below
               the scrolling proxies, always visible rather than the first,
               scrollable item it used to be. -->
          <div
            role="option"
            :aria-selected="modelValue === null"
            class="pxsel-direct"
            :data-selected="modelValue === null || undefined"
            :data-focused="focusedIndex === filteredProxies.length || undefined"
            @click="selectOption(null)"
            @mouseenter="focusedIndex = filteredProxies.length"
          >
            <span class="pxsel-row__dot-box" :data-checked="modelValue === null || undefined" aria-hidden="true">
              <span class="pxsel-row__dot" />
            </span>
            <span>{{ t('admin.proxies.directOption') }}</span>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
/**
 * ProxySelector — part 02, section 15.
 *
 * Migration note from the prototype:
 *   "Results already live in a reactive record keyed by proxy id, so nothing
 *    new is needed in state. Move them out of the toast and into the row, and
 *    keep them visible after the popover closes."
 *
 * testResults/testingProxyIds/batchTesting and the adminAPI calls below are
 * untouched from before this rewrite — the migration note only asks to move
 * the result's *display* into the row, not to change how it's fetched.
 *
 * The trigger reuses Select.vue's `select-trigger` class, not just its look:
 * composables/useOnboardingTour.ts does
 * `querySelector('.select-trigger')` to detect "this tour step highlights a
 * teleported dropdown, so don't wait for a click to bubble through it, the
 * popover isn't a DOM descendant of the trigger any more." The prototype's
 * migration note asks to move this popover to the same teleported chrome as
 * Select (it was absolutely-positioned before, easy to clip inside the
 * account modals this renders in), which makes that detection correct for
 * this component for the first time. No current call site puts a `data-tour`
 * step directly on ProxySelector, so this is a forward-compat fix, not a
 * regression risk today.
 */
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { Proxy } from '@/types'

const { t } = useI18n()

const instanceId = `pxsel-${Math.random().toString(36).substring(2, 9)}`

interface ProxyTestResult {
  success: boolean
  message: string
  latency_ms?: number
  ip_address?: string
  city?: string
  region?: string
  country?: string
}

interface Props {
  modelValue: number | null
  proxies: Proxy[]
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

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
const dropdownMinimumWidth = 240

// Test state — unchanged from before this rewrite.
const testResults = reactive<Record<number, ProxyTestResult>>({})
const testingProxyIds = reactive(new Set<number>())
const batchTesting = ref(false)

const selectedProxy = computed(() => {
  if (props.modelValue === null) return null
  return props.proxies.find((p) => p.id === props.modelValue) || null
})

const filteredProxies = computed(() => {
  if (!searchQuery.value) return props.proxies
  const query = searchQuery.value.toLowerCase()
  return props.proxies.filter((proxy) => {
    const name = proxy.name.toLowerCase()
    const host = proxy.host.toLowerCase()
    return name.includes(query) || host.includes(query)
  })
})

// "type and region line" per spec.proxyselector.anatomy: protocol is always
// real; region falls back to city, then country, whichever static field the
// proxy actually has, all already on the Proxy type.
const proxyMeta = (proxy: Proxy): string => {
  const place = proxy.region || proxy.city || proxy.country
  return place ? `${proxy.protocol} · ${place}` : proxy.protocol
}

// spec.proxyselector.tokens: "result ok --success, slow --sr attention,
// refused --destructive". --sr does not exist as a token; --s2a-attn is
// CONVENTIONS.md's named attention colour and is what the rest of this
// design pass (see GroupSelector's capacity bar) treats "sr attention" as.
// The 200ms ok/slow threshold matches ProxiesView.vue's existing
// `latency_ms < 200 ? 'badge-success' : 'badge-warning'` split, not a new
// number invented for this component.
const resultColor = (proxy: Proxy): string => {
  const result = testResults[proxy.id]
  if (!result) return ''
  if (!result.success) return 'var(--destructive)'
  if (typeof result.latency_ms === 'number' && result.latency_ms >= 200) return 'var(--s2a-attn)'
  return 'var(--success)'
}

const resultText = (proxy: Proxy): string => {
  if (testingProxyIds.has(proxy.id)) return ''
  const result = testResults[proxy.id]
  if (!result) return ''
  if (!result.success) return t('admin.proxies.testFailed')
  if (typeof result.latency_ms === 'number') return `${result.latency_ms}ms`
  return result.country || ''
}

// Combined nav order for the keyboard model: proxy rows, then the direct/no
// proxy row. Mirrors Select.vue's findNext/findPrevEnabledIndex, simplified
// because nothing here is ever individually disabled.
const navCount = computed(() => filteredProxies.value.length + 1)

const dropdownStyle = computed(() => {
  if (!triggerRect.value) return {}
  const rect = triggerRect.value
  const viewportRight = Math.max(dropdownViewportPadding, window.innerWidth - dropdownViewportPadding)
  const left = Math.min(Math.max(dropdownViewportPadding, rect.left), viewportRight)
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
    const dropdownHeight = dropdownRef.value.offsetHeight || 320
    const spaceBelow = window.innerHeight - triggerRect.value.bottom
    const spaceAbove = triggerRect.value.top
    dropdownPosition.value = spaceBelow < dropdownHeight && spaceAbove > dropdownHeight ? 'top' : 'bottom'
  })
}

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
}

const selectOption = (value: number | null) => {
  emit('update:modelValue', value)
  isOpen.value = false
  searchQuery.value = ''
  triggerRef.value?.focus()
}

const handleTestProxy = async (proxy: Proxy) => {
  if (testingProxyIds.has(proxy.id)) return
  testingProxyIds.add(proxy.id)
  try {
    const result = await adminAPI.proxies.testProxy(proxy.id)
    testResults[proxy.id] = result
  } catch (error: any) {
    testResults[proxy.id] = {
      success: false,
      message: error.response?.data?.detail || 'Test failed'
    }
  } finally {
    testingProxyIds.delete(proxy.id)
  }
}

const handleBatchTest = async () => {
  if (batchTesting.value || props.proxies.length === 0) return
  batchTesting.value = true
  const testPromises = props.proxies.map(async (proxy) => {
    testingProxyIds.add(proxy.id)
    try {
      const result = await adminAPI.proxies.testProxy(proxy.id)
      testResults[proxy.id] = result
    } catch (error: any) {
      testResults[proxy.id] = {
        success: false,
        message: error.response?.data?.detail || 'Test failed'
      }
    } finally {
      testingProxyIds.delete(proxy.id)
    }
  })
  await Promise.all(testPromises)
  batchTesting.value = false
}

const onTriggerKeyDown = () => {
  if (!isOpen.value) isOpen.value = true
}

const scrollToFocused = () => {
  nextTick(() => {
    const list = optionsListRef.value
    if (!list) return
    const focusedEl = list.children[focusedIndex.value] as HTMLElement | undefined
    if (!focusedEl) return
    if (focusedEl.offsetTop < list.scrollTop) {
      list.scrollTop = focusedEl.offsetTop
    } else if (focusedEl.offsetTop + focusedEl.offsetHeight > list.scrollTop + list.offsetHeight) {
      list.scrollTop = focusedEl.offsetTop + focusedEl.offsetHeight - list.offsetHeight
    }
  })
}

const onDropdownKeyDown = (e: KeyboardEvent) => {
  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      focusedIndex.value = navCount.value === 0 ? -1 : (focusedIndex.value + 1) % navCount.value
      if (focusedIndex.value < filteredProxies.value.length) scrollToFocused()
      break
    case 'ArrowUp':
      e.preventDefault()
      focusedIndex.value = navCount.value === 0 ? -1 : (focusedIndex.value - 1 + navCount.value) % navCount.value
      if (focusedIndex.value < filteredProxies.value.length) scrollToFocused()
      break
    case 'Enter':
      e.preventDefault()
      if (focusedIndex.value >= 0 && focusedIndex.value < filteredProxies.value.length) {
        selectOption(filteredProxies.value[focusedIndex.value].id)
      } else if (focusedIndex.value === filteredProxies.value.length) {
        selectOption(null)
      }
      break
    case 'Escape':
      e.preventDefault()
      isOpen.value = false
      searchQuery.value = ''
      triggerRef.value?.focus()
      break
    case 'Tab':
      isOpen.value = false
      break
  }
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  const isInDropdown = !!target.closest(`.${instanceId}`)
  const isInTrigger = containerRef.value?.contains(target)
  if (!isInDropdown && !isInTrigger && isOpen.value) {
    isOpen.value = false
    searchQuery.value = ''
  }
}

// Mirrors Select.vue's open/close watcher: position on open, focus the
// search box, track scroll/resize while open, reset on close.
watch(isOpen, (open) => {
  if (open) {
    calculateDropdownPosition()
    focusedIndex.value = -1
    nextTick(() => searchInputRef.value?.focus())
    window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
    window.addEventListener('resize', calculateDropdownPosition)
  } else {
    searchQuery.value = ''
    focusedIndex.value = -1
    window.removeEventListener('scroll', updateTriggerRect, { capture: true })
    window.removeEventListener('resize', calculateDropdownPosition)
  }
})

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
.pxsel {
  position: relative;
  width: 100%;
}

/* --- trigger --------------------------------------------------------------
   Literally Select.vue's `.select-trigger` chrome and class name: matches
   its class conventions, and useOnboardingTour.ts's `.select-trigger`
   querySelector (see the migration note above) depends on the literal name
   now that this popover teleports like Select's does. Scoped styles don't
   leak between components, so redeclaring the rule here is required, not
   just cosmetic. */
.select-trigger {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
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
.select-trigger[data-open] {
  border-color: var(--ring-focus);
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

.select-trigger__value {
  display: flex;
  align-items: baseline;
  gap: 6px;
  overflow: hidden;
}
.pxsel-trigger__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
/* Connection info is a technical identifier: --font-mono per 01-TOKENS,
   distinct from the proxy's prose name. */
.pxsel-trigger__addr {
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
}

.select-trigger__caret {
  flex-shrink: 0;
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--muted-foreground);
  /* --t-normal, named in the prototype's migration note elsewhere in this
     part, does not exist; --t-med is the nearest token to its 200ms. */
  transition: transform var(--t-med) var(--ease-out);
}
.select-trigger__caret--open {
  transform: rotate(180deg);
}

/* --- popover ---------------------------------------------------------- */

.pxsel-portal {
  border: 1px solid var(--popover-border);
  border-radius: var(--r-md);
  background: var(--popover);
  box-shadow: var(--shadow-md);
  overflow: hidden;
  pointer-events: auto !important;
}

.pxsel-portal__header {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 6px 8px 6px 11px;
  border-bottom: 1px solid var(--border-subtle);
  color: var(--muted-foreground);
}
.pxsel-portal__header > i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
  flex-shrink: 0;
}

.pxsel-portal__search {
  flex: 1;
  min-width: 0;
  border: 0;
  background: transparent;
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  outline: none;
}
.pxsel-portal__search::placeholder {
  color: var(--muted-foreground);
}

.pxsel-portal__test-all {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
  height: 26px;
  padding: 0 9px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--foreground);
  font-family: inherit;
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.pxsel-portal__test-all:hover:not(:disabled) {
  background: var(--sidebar-accent);
}
.pxsel-portal__test-all:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.pxsel-portal__test-all i {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
}

.pxsel-portal__list {
  max-height: 240px;
  overflow-y: auto;
  outline: none;
}

.pxsel-row,
.pxsel-direct {
  display: grid;
  align-items: center;
  gap: 10px;
  min-height: 38px;
  padding: 7px 11px;
  border-top: 1px solid var(--border-subtle);
  cursor: pointer;
  transition: background var(--motion-hover);
}
.pxsel-row {
  grid-template-columns: 15px minmax(0, 1fr) 108px 26px;
}
.pxsel-direct {
  grid-template-columns: 15px minmax(0, 1fr);
  min-height: 34px;
  color: var(--body-copy);
  font-size: var(--fs-md);
}

.pxsel-row:hover,
.pxsel-row[data-selected],
.pxsel-row[data-focused],
.pxsel-direct:hover,
.pxsel-direct[data-selected],
.pxsel-direct[data-focused] {
  background: var(--sidebar-accent);
}

/* 15 round, matches Radio.vue's chrome: border --brand when checked, fill
   only on the small inner dot, never the whole box. */
.pxsel-row__dot-box {
  display: grid;
  place-items: center;
  width: 15px;
  height: 15px;
  border: 1px solid var(--border);
  border-radius: 50%;
  background: var(--card);
}
.pxsel-row__dot-box[data-checked] {
  border-color: var(--brand);
}
.pxsel-row__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: transparent;
}
.pxsel-row__dot-box[data-checked] .pxsel-row__dot {
  background: var(--brand);
}

.pxsel-row__meta {
  min-width: 0;
}

/* Start-truncation: direction:rtl on the outer flips which end the ellipsis
   sits on; unicode-bidi:isolate on the inner keeps the host's own characters
   left-to-right so it doesn't visually reverse. Host is a technical
   identifier: --font-mono per 01-TOKENS. */
.pxsel-row__host {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  direction: rtl;
  text-align: left;
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  color: var(--foreground);
}
.pxsel-row__host-inner {
  direction: ltr;
  unicode-bidi: isolate;
}

.pxsel-row__sub {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.pxsel-row__result {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.pxsel-row__result-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pxsel-row__result-dot {
  flex-shrink: 0;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

.pxsel-row__test {
  display: grid;
  place-items: center;
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}
.pxsel-row__test:hover:not(:disabled) {
  background: var(--sidebar-accent);
  color: var(--foreground);
}
.pxsel-row__test:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.pxsel-row__test i {
  font-size: 13px; /* june-lint-disable ground-rule-4: icon glyph */
}

/* 11-13px currentColor ring spinner, shared by the row test button, the
   inline result and "Test all"; sizing set per call site below. */
.pxsel-spinner {
  display: inline-block;
  width: 11px;
  height: 11px;
  border: 1.5px solid currentColor;
  border-top-color: transparent;
  border-radius: 50%;
  animation: s2a-spin 0.7s linear infinite;
}
.pxsel-portal__test-all .pxsel-spinner,
.pxsel-row__test .pxsel-spinner {
  width: 13px;
  height: 13px;
}

.pxsel-portal__empty {
  padding: 12px 8px;
  text-align: center;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

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
  .select-pop-leave-active,
  .pxsel-spinner {
    transition: none;
    animation: none;
  }
  .select-pop-enter-from,
  .select-pop-leave-to {
    transform: none;
  }
}
</style>
