<template>
  <div v-if="!isDesktopViewport" class="dt-cards">
    <template v-if="loading">
      <div v-for="i in 5" :key="i" class="dt-card dt-card--skeleton">
        <div class="dt-card__body">
          <div v-for="column in dataColumns" :key="column.key" class="dt-card__field">
            <span class="dt-skel" style="width: 34%" />
            <span class="dt-skel" style="width: 46%" />
          </div>
          <div v-if="hasActionsColumn" class="dt-card__actions-skel">
            <span class="dt-skel" style="width: 100%; height: 24px" />
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="!data || data.length === 0">
      <div class="dt-card dt-card--empty">
        <slot name="empty">
          <EmptyState :title="t('empty.noData')" />
        </slot>
      </div>
    </template>

    <template v-else>
      <div v-if="selectable" class="dt-cards__selectall">
        <Checkbox
          :model-value="allVisibleSelected"
          :indeterminate="someVisibleSelected"
          :aria-label="t('common.selectAll')"
          data-test="select-all-mobile"
          @update:model-value="toggleAllVisible"
        >
          {{ t('common.selectAll') }}
        </Checkbox>
      </div>
      <div
        v-for="(row, index) in sortedData"
        :key="resolveRowKey(row, index)"
        class="dt-card"
        :data-clickable="clickableRows || undefined"
        :data-selected="(selectable && isRowSelected(row, index)) || undefined"
        @click="clickableRows && emit('rowClick', row)"
      >
        <div class="dt-card__body">
          <div v-if="selectable" class="dt-card__select">
            <Checkbox
              :model-value="isRowSelected(row, index)"
              :aria-label="getRowSelectionLabel(row, index)"
              data-test="select-row"
              @click.stop
              @update:model-value="(checked: boolean) => toggleRowSelection(row, index, checked)"
            />
          </div>
          <div
            v-for="column in dataColumns"
            :key="column.key"
            :data-field="column.key"
            class="dt-card__field min-w-0"
          >
            <span class="dt-card__label">{{ column.label }}</span>
            <div class="dt-card__value min-w-0 max-w-full">
              <slot :name="`cell-${column.key}`" :row="row" :value="row[column.key]" :expanded="actionsExpanded">
                {{ column.formatter ? column.formatter(row[column.key], row) : row[column.key] }}
              </slot>
            </div>
          </div>
          <div v-if="hasActionsColumn" class="dt-card__actions">
            <slot name="cell-actions" :row="row" :value="row['actions']" :expanded="actionsExpanded"></slot>
          </div>
        </div>
      </div>
    </template>
  </div>

  <div
    v-else
    ref="tableWrapperRef"
    class="table-wrapper"
    :data-density="density"
    :class="{
      'actions-expanded': actionsExpanded,
      'is-scrollable': isScrollable
    }"
  >
    <table class="dt-table" :style="{ '--dt-cell-px': adaptivePaddingPx + 'px' }">
      <thead class="dt-thead">
        <tr class="dt-tr-head">
          <th v-if="selectable" scope="col" class="dt-th dt-th--select">
            <Checkbox
              :model-value="allVisibleSelected"
              :indeterminate="someVisibleSelected"
              :aria-label="t('common.selectAll')"
              data-test="select-all"
              @update:model-value="toggleAllVisible"
            />
          </th>
          <th
            v-for="(column, index) in columns"
            :key="column.key"
            scope="col"
            :aria-sort="column.sortable ? getColumnAriaSort(column.key) : undefined"
            :data-sortable="column.sortable || undefined"
            :data-sorted="sortKey === column.key || undefined"
            :class="['dt-th', getStickyColumnClass(column, index), column.class]"
            @click="column.sortable && handleSort(column.key)"
          >
            <span class="dt-th__inner" :class="getHeaderAlignClass(column)">
              <slot
                :name="`header-${column.key}`"
                :column="column"
                :sort-key="sortKey"
                :sort-order="sortOrder"
              >
                <span>{{ column.label }}</span>
              </slot>
              <i
                v-if="column.sortable"
                class="hgi-stroke dt-th__caret"
                :class="sortKey === column.key && sortOrder === 'asc' ? 'hgi-arrow-up-01' : 'hgi-arrow-down-01'"
                aria-hidden="true"
              />
            </span>
          </th>
        </tr>
      </thead>
      <tbody class="dt-tbody">
        <!-- Loading: five flat rows, no pulse, widths approximating real content. -->
        <tr v-if="loading" v-for="i in 5" :key="i" class="dt-tr dt-tr--skeleton">
          <td v-if="selectable" class="dt-td dt-td--select"></td>
          <td v-for="(column, colIndex) in columns" :key="column.key" class="dt-td">
            <span class="dt-skel" :style="{ width: skeletonWidth(i, colIndex) }"></span>
          </td>
        </tr>

        <!-- Empty state: composes the shared EmptyState, not a bespoke inbox glyph. -->
        <tr v-else-if="!data || data.length === 0">
          <td :colspan="tableColumnCount" class="dt-td dt-td--empty">
            <slot name="empty">
              <EmptyState :title="t('empty.noData')" />
            </slot>
          </td>
        </tr>

        <!-- Data rows: windowed when large, fully rendered when small (shared row/cell template) -->
        <template v-else>
          <tr v-if="virtualPaddingTop > 0" aria-hidden="true">
            <td :colspan="tableColumnCount"
                :style="{ height: virtualPaddingTop + 'px', padding: 0, border: 'none' }">
            </td>
          </tr>
          <tr
            v-for="item in renderRows"
            :key="resolveRowKey(item.row, item.index)"
            :data-row-id="resolveRowKey(item.row, item.index)"
            :data-index="item.index"
            :ref="item.measure ? measureElement : undefined"
            class="dt-tr"
            :data-clickable="clickableRows || undefined"
            :data-selected="(selectable && isRowSelected(item.row, item.index)) || undefined"
            @click="clickableRows && emit('rowClick', item.row)"
          >
            <td v-if="selectable" class="dt-td dt-td--select">
              <Checkbox
                :model-value="isRowSelected(item.row, item.index)"
                :aria-label="getRowSelectionLabel(item.row, item.index)"
                data-test="select-row"
                @click.stop
                @update:model-value="(checked: boolean) => toggleRowSelection(item.row, item.index, checked)"
              />
            </td>
            <td
              v-for="(column, colIndex) in columns"
              :key="column.key"
              :class="['dt-td', getStickyColumnClass(column, colIndex), column.class]"
            >
              <slot :name="`cell-${column.key}`"
                    :row="item.row"
                    :value="item.row[column.key]"
                    :expanded="actionsExpanded">
                {{ column.formatter
                   ? column.formatter(item.row[column.key], item.row)
                   : item.row[column.key] }}
              </slot>
            </td>
          </tr>
          <tr v-if="virtualPaddingBottom > 0" aria-hidden="true">
            <td :colspan="tableColumnCount"
                :style="{ height: virtualPaddingBottom + 'px', padding: 0, border: 'none' }">
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
/**
 * DataTable — part 04, section 01 ("the part that matters most").
 *
 * Migration note from the prototype:
 *   "Remove uppercase and tracking-wider from the header cells, the four rgba
 *    gradients from the sticky pseudo elements, animate-pulse from the
 *    skeleton rows, and the double chevron from sortable headers. Column
 *    model, virtualizer, sort persistence and the mobile card layout are
 *    unchanged."
 *
 * This is a presentation rewrite: every prop, emit and slot below is
 * unchanged from before, including the scoped slot payload shapes
 * (`{ row, value, expanded }` on cell-*, `{ column, sortKey, sortOrder }` on
 * header-*). 22 consumers bind these directly. The one addition is
 * `density`, called out in the spec as "the only prop added in this part".
 *
 * Two props keep their old defaults rather than the spec's proposed new
 * ones, both deliberately:
 *
 *   stickyActionsColumn   spec: "becomes false by default once actions
 *                          collapse into the row menu." The row actions menu
 *                          (part 04, section 05) is a separate component that
 *                          is not built or wired into any of the 22 call
 *                          sites yet. Flipping this default now would drop
 *                          the sticky actions column with nothing standing
 *                          in for it. Stays true until that menu ships.
 *
 *   selectionLabel         spec: "should be required for accessibility."
 *                          Making it required is a type-level breaking
 *                          change for every non-selectable table. Stays
 *                          optional.
 *
 * estimateRowHeight's default does change, 56 to 36, matching the new
 * default row density -- an explicit default-value change the props table
 * calls for, and safe because it only affects the virtualizer's initial
 * guess before real measurement lands.
 */
import { computed, ref, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useVirtualizer, observeElementRect as observeElementRectDefault } from '@tanstack/vue-virtual'
import { useI18n } from 'vue-i18n'
import type { Column } from './types'
import Checkbox from './Checkbox.vue'
import EmptyState from './EmptyState.vue'

const { t } = useI18n()

const desktopViewportQuery = '(min-width: 768px)'
const isDesktopViewport = ref(
  typeof window === 'undefined' ? true : window.matchMedia(desktopViewportQuery).matches
)

const emit = defineEmits<{
  sort: [key: string, order: 'asc' | 'desc']
  rowClick: [row: any]
  'update:selectedKeys': [keys: Array<string | number>]
  selectionChange: [keys: Array<string | number>]
}>()

// Table container ref: outside-click for nothing here, but the scroll and
// resize observers below hang off it, and it is the `.table-wrapper` hook
// TablePageLayout's `:deep()` scroll chain looks for.
const tableWrapperRef = ref<HTMLElement | null>(null)
const isScrollable = ref(false)
const actionsColumnNeedsExpanding = ref(false)

// --- Virtual scrolling "whole table blank" root cause fix ---
// Root cause: this component's root .table-wrapper is flex:1 / min-h-0, so its
// height is decided by the parent flex chain. @tanstack's virtualizer only
// writes scrollRect inside the observeElementRect callback; if that callback
// ever reads a height of 0 (the instant flex has not settled on mount, or a
// reflow triggered by dynamic row-height correction mid-scroll), scrollRect
// gets pinned to 0, calculateRange returns null, and the whole table goes
// blank. The fix (see the virtualizer options below):
//   1) override observeElementRect to drop any height<=0 reading, so
//      scrollRect is never pinned to 0;
//   2) give initialRect a one-screen fallback height, so rows render before
//      the first real reading arrives, never a blank frame.
// Fallback height: the table area is roughly viewport height minus the
// top bar / margins / filters / pagination, about 320px.
const estimatedViewportHeight = () => {
  if (typeof window === 'undefined') return 600
  return Math.max(window.innerHeight - 320, 400)
}

// Overrides the default observeElementRect: filters out zero-height readings
// (the key fix for the whole-table-blank bug).
const observeElementRectNonZero = (
  instance: any,
  cb: (rect: { width: number; height: number }) => void
) => observeElementRectDefault(instance, (rect) => {
  if (rect.height > 0) cb(rect)
})

// Whether the table can scroll horizontally.
const checkScrollable = () => {
  if (tableWrapperRef.value) {
    isScrollable.value = tableWrapperRef.value.scrollWidth > tableWrapperRef.value.clientWidth
  }
}

// Whether the (legacy) expandable actions column needs to expand.
const checkActionsColumnWidth = () => {
  if (!props.expandableActions) {
    actionsColumnNeedsExpanding.value = false
    actionsExpanded.value = false
    return
  }
  if (!tableWrapperRef.value) return

  const firstActionCell = tableWrapperRef.value.querySelector('tbody tr:first-child td:last-child')
  if (!firstActionCell) return

  const actionsContainer = firstActionCell.querySelector('div')
  if (!actionsContainer) return

  // Temporarily expand to measure the full width.
  const wasExpanded = actionsExpanded.value
  actionsExpanded.value = true

  nextTick(() => {
    const actionItems = actionsContainer.querySelectorAll('button, a, [role="button"]')
    if (actionItems.length <= 2) {
      actionsColumnNeedsExpanding.value = false
      actionsExpanded.value = wasExpanded
      return
    }

    let totalWidth = 0
    actionItems.forEach((item, index) => {
      totalWidth += (item as HTMLElement).offsetWidth
      if (index < actionItems.length - 1) {
        totalWidth += 4 // gap-1 = 4px
      }
    })

    const cellWidth = (firstActionCell as HTMLElement).clientWidth - 32 // minus left/right padding

    actionsColumnNeedsExpanding.value = totalWidth > cellWidth

    actionsExpanded.value = wasExpanded
  })
}

// Size tracking.
let resizeObserver: ResizeObserver | null = null
let resizeHandler: (() => void) | null = null
let desktopViewportMediaQuery: MediaQueryList | null = null
let desktopViewportListener: ((event: MediaQueryListEvent) => void) | null = null

const detachDesktopTableTracking = () => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (resizeHandler) {
    window.removeEventListener('resize', resizeHandler)
    resizeHandler = null
  }
}

const attachDesktopTableTracking = () => {
  checkScrollable()
  checkActionsColumnWidth()
  if (tableWrapperRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(() => {
      checkScrollable()
      checkActionsColumnWidth()
    })
    resizeObserver.observe(tableWrapperRef.value)
  } else {
    // Fallback when ResizeObserver is unsupported: window resize.
    resizeHandler = () => {
      checkScrollable()
      checkActionsColumnWidth()
    }
    window.addEventListener('resize', resizeHandler)
  }
}

onMounted(() => {
  if (typeof window !== 'undefined') {
    desktopViewportMediaQuery = window.matchMedia(desktopViewportQuery)
    isDesktopViewport.value = desktopViewportMediaQuery.matches
    desktopViewportListener = (event: MediaQueryListEvent) => {
      isDesktopViewport.value = event.matches
    }
    if (typeof desktopViewportMediaQuery.addEventListener === 'function') {
      desktopViewportMediaQuery.addEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.addListener(desktopViewportListener)
    }
  }
})

onUnmounted(() => {
  detachDesktopTableTracking()
  if (desktopViewportMediaQuery && desktopViewportListener) {
    if (typeof desktopViewportMediaQuery.removeEventListener === 'function') {
      desktopViewportMediaQuery.removeEventListener('change', desktopViewportListener)
    } else {
      desktopViewportMediaQuery.removeListener(desktopViewportListener)
    }
    desktopViewportListener = null
  }
  desktopViewportMediaQuery = null
})

interface Props {
  columns: Column[]
  data: any[]
  loading?: boolean
  stickyFirstColumn?: boolean
  stickyActionsColumn?: boolean
  expandableActions?: boolean
  actionsCount?: number // total action buttons, used to decide whether to expand
  rowKey?: string | ((row: any) => string | number)
  /**
   * Default sort configuration (only applied when there is no persisted sort state)
   */
  defaultSortKey?: string
  defaultSortOrder?: 'asc' | 'desc'
  /**
   * Persist sort state (key + order) to localStorage using this key.
   * If provided, DataTable will load the stored sort state on mount.
   */
  sortStorageKey?: string
  /**
   * Enable server-side sorting mode. When true, clicking sort headers
   * will emit 'sort' events instead of performing client-side sorting.
   */
  serverSideSort?: boolean
  /** Emit 'rowClick' on row/card click and show pointer cursor (interactive cells should @click.stop) */
  clickableRows?: boolean
  /** Estimated row height in px for the virtualizer (default 36, matching the default density) */
  estimateRowHeight?: number
  /** Number of rows to render beyond the visible area (default 5) */
  overscan?: number
  /**
   * Only virtualize when the row count exceeds this threshold (default 100).
   * Smaller lists render in full, avoiding the scroll-compensation jank caused by
   * estimated-vs-actual row heights when rows have variable height.
   */
  virtualizeThreshold?: number
  /** Enable controlled row selection. Stable row keys are strongly recommended. */
  selectable?: boolean
  /** Selected row keys. Keys outside the current data page are preserved. */
  selectedKeys?: Array<string | number>
  /** Accessible label for a row selection checkbox. */
  selectionLabel?: string | ((row: any) => string)
  /**
   * Row density: compact (28px, a table nested in a card or modal), default
   * (36px, every full page table) or twoLine (44px, a cell that pairs a
   * status with its reason). The only prop this part of the redesign adds.
   */
  density?: 'compact' | 'default' | 'twoLine'
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  stickyFirstColumn: true,
  stickyActionsColumn: true,
  expandableActions: true,
  defaultSortOrder: 'asc',
  serverSideSort: false,
  selectable: false,
  selectedKeys: () => [],
  density: 'default'
})

const sortKey = ref<string>('')
const sortOrder = ref<'asc' | 'desc'>('asc')
const actionsExpanded = ref(false)

type PersistedSortState = {
  key: string
  order: 'asc' | 'desc'
}

const collator = new Intl.Collator(undefined, {
  numeric: true,
  sensitivity: 'base'
})

const getSortableKeys = () => {
  const keys = new Set<string>()
  for (const col of props.columns) {
    if (col.sortable) keys.add(col.key)
  }
  return keys
}

const normalizeSortKey = (candidate: string) => {
  if (!candidate) return ''
  const sortableKeys = getSortableKeys()
  return sortableKeys.has(candidate) ? candidate : ''
}

const normalizeSortOrder = (candidate: any): 'asc' | 'desc' => {
  return candidate === 'desc' ? 'desc' : 'asc'
}

const readPersistedSortState = (): PersistedSortState | null => {
  if (!props.sortStorageKey) return null
  try {
    const raw = localStorage.getItem(props.sortStorageKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedSortState>
    const key = normalizeSortKey(typeof parsed.key === 'string' ? parsed.key : '')
    if (!key) return null
    return { key, order: normalizeSortOrder(parsed.order) }
  } catch (e) {
    console.error('[DataTable] Failed to read persisted sort state:', e)
    return null
  }
}

const writePersistedSortState = (state: PersistedSortState) => {
  if (!props.sortStorageKey) return
  try {
    localStorage.setItem(props.sortStorageKey, JSON.stringify(state))
  } catch (e) {
    console.error('[DataTable] Failed to persist sort state:', e)
  }
}

const resolveInitialSortState = (): PersistedSortState | null => {
  const persisted = readPersistedSortState()
  if (persisted) return persisted

  const key = normalizeSortKey(props.defaultSortKey || '')
  if (!key) return null
  return { key, order: normalizeSortOrder(props.defaultSortOrder) }
}

const applySortState = (state: PersistedSortState | null) => {
  if (!state) return
  sortKey.value = state.key
  sortOrder.value = state.order
}

const getColumnAriaSort = (key: string) => {
  if (sortKey.value !== key) return 'none'
  return sortOrder.value === 'asc' ? 'ascending' : 'descending'
}

// One caret, on the sorted column, pointing the way the data runs -- not a
// permanent up/down pair on every sortable header. Alignment is read from
// the caller-supplied column.class (still a plain pass-through prop) but the
// class this component hands back to its own markup is ours, not Tailwind's.
const getHeaderAlignClass = (column: Column) => {
  const className = column.class || ''
  if (className.includes('text-center')) return 'dt-align-center'
  if (className.includes('text-right')) return 'dt-align-end'
  return ''
}

const isNullishOrEmpty = (value: any) => value === null || value === undefined || value === ''

const toFiniteNumberOrNull = (value: any): number | null => {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value === 'boolean') return value ? 1 : 0
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (!trimmed) return null
    const n = Number(trimmed)
    return Number.isFinite(n) ? n : null
  }
  return null
}

const toSortableString = (value: any): string => {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  if (value instanceof Date) return value.toISOString()
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const compareSortValues = (a: any, b: any): number => {
  const aEmpty = isNullishOrEmpty(a)
  const bEmpty = isNullishOrEmpty(b)
  if (aEmpty && bEmpty) return 0
  if (aEmpty) return 1
  if (bEmpty) return -1

  const aNum = toFiniteNumberOrNull(a)
  const bNum = toFiniteNumberOrNull(b)
  if (aNum !== null && bNum !== null) {
    if (aNum === bNum) return 0
    return aNum < bNum ? -1 : 1
  }

  const aStr = toSortableString(a)
  const bStr = toSortableString(b)
  const res = collator.compare(aStr, bStr)
  if (res === 0) return 0
  return res < 0 ? -1 : 1
}
const resolveStableRowKey = (row: any): string | number | undefined => {
  if (typeof props.rowKey === 'function') {
    const key = props.rowKey(row)
    return key ?? undefined
  }
  if (typeof props.rowKey === 'string' && props.rowKey) {
    const key = row?.[props.rowKey]
    return key ?? undefined
  }
  const key = row?.id
  return key ?? undefined
}

const resolveRowKey = (row: any, index: number) => resolveStableRowKey(row) ?? index

const dataColumns = computed(() => props.columns.filter((column) => column.key !== 'actions'))
const columnsSignature = computed(() =>
  props.columns.map((column) => `${column.key}:${column.sortable ? '1' : '0'}`).join('|')
)

watch(
  isDesktopViewport,
  async (isDesktop) => {
    detachDesktopTableTracking()
    if (!isDesktop) return
    await nextTick()
    attachDesktopTableTracking()
  },
  { immediate: true, flush: 'post' }
)

// Re-check scroll state when data/columns change. Cannot watch actionsExpanded
// directly: checkActionsColumnWidth temporarily flips it, which would loop.
watch(
  [() => props.data.length, columnsSignature],
  async () => {
    await nextTick()
    checkScrollable()
    checkActionsColumnWidth()
  },
  { flush: 'post' }
)

// Separately watch the expanded state, only to refresh scroll state.
watch(actionsExpanded, async () => {
  await nextTick()
  checkScrollable()
})

const handleSort = (key: string) => {
  let newOrder: 'asc' | 'desc' = 'asc'
  if (sortKey.value === key) {
    newOrder = sortOrder.value === 'asc' ? 'desc' : 'asc'
  }

  if (props.serverSideSort) {
    // Server-side sort mode: emit event and update internal state for UI feedback
    sortKey.value = key
    sortOrder.value = newOrder
    emit('sort', key, newOrder)
  } else {
    // Client-side sort mode: just update internal state
    sortKey.value = key
    sortOrder.value = newOrder
  }
}

const sortedData = computed(() => {
  // Server-side sort mode: return data as-is (server handles sorting)
  if (props.serverSideSort || !sortKey.value || !props.data) return props.data

  const key = sortKey.value
  const order = sortOrder.value

  // Stable sort (tie-break with original index) to avoid jitter when values are equal.
  return props.data
    .map((row, index) => ({ row, index }))
    .sort((a, b) => {
      const cmp = compareSortValues(a.row?.[key], b.row?.[key])
      if (cmp !== 0) return order === 'asc' ? cmp : -cmp
      return a.index - b.index
    })
    .map(item => item.row)
})

const tableColumnCount = computed(() => props.columns.length + (props.selectable ? 1 : 0))
const selectedKeySet = computed(() => new Set(props.selectedKeys))
const visibleRowKeys = computed(() =>
  (sortedData.value ?? []).map((row, index) => resolveRowKey(row, index))
)
const allVisibleSelected = computed(() =>
  visibleRowKeys.value.length > 0
  && visibleRowKeys.value.every((key) => selectedKeySet.value.has(key))
)
const someVisibleSelected = computed(() => {
  if (allVisibleSelected.value) return false
  return visibleRowKeys.value.some((key) => selectedKeySet.value.has(key))
})

const emitSelection = (next: Set<string | number>) => {
  const keys = Array.from(next)
  emit('update:selectedKeys', keys)
  emit('selectionChange', keys)
}

const isRowSelected = (row: any, index: number) =>
  selectedKeySet.value.has(resolveRowKey(row, index))

const getRowSelectionLabel = (row: any, index: number) => {
  if (typeof props.selectionLabel === 'function') return props.selectionLabel(row)
  if (props.selectionLabel) return props.selectionLabel
  return `${t('common.selectOption')} ${resolveRowKey(row, index)}`
}

const toggleRowSelection = (row: any, index: number, checked: boolean) => {
  const next = new Set(props.selectedKeys)
  const key = resolveRowKey(row, index)
  if (checked) next.add(key)
  else next.delete(key)
  emitSelection(next)
}

const toggleAllVisible = (checked: boolean) => {
  const next = new Set(props.selectedKeys)
  for (const key of visibleRowKeys.value) {
    if (checked) next.add(key)
    else next.delete(key)
  }
  emitSelection(next)
}

// --- Virtual scrolling ---
// Only virtualize on desktop once the row count passes the threshold. Small
// lists render in full, sidestepping the virtualizer's estimate/measure/
// scroll-compensation chain entirely, which removes the jank that variable
// row heights would otherwise cause.
const shouldVirtualize = computed(() =>
  isDesktopViewport.value && (sortedData.value?.length ?? 0) > (props.virtualizeThreshold ?? 100)
)

const rowVirtualizer = useVirtualizer(computed(() => ({
  count: shouldVirtualize.value ? (sortedData.value?.length ?? 0) : 0,
  getScrollElement: () => tableWrapperRef.value,
  // Keyed by the row's stable key (matching the template's :key), not the
  // default index, so sort/filter/threshold changes reuse the right cached
  // row heights instead of stale index-keyed ones -- this is what removes
  // the height-correction jitter.
  getItemKey: (index: number) => {
    const row = sortedData.value?.[index]
    return row != null ? resolveRowKey(row, index) : index
  },
  estimateSize: () => props.estimateRowHeight ?? 36,
  overscan: props.overscan ?? 5,
  // Fallback height: render roughly a screen of rows before the first real
  // height reading lands, so there is never a blank frame.
  initialRect: { width: 0, height: estimatedViewportHeight() },
  // The fix for the whole-table-blank bug: drop zero-height readings so
  // scrollRect never gets pinned to 0, which is what makes calculateRange
  // return null.
  observeElementRect: observeElementRectNonZero,
  // Batch measurement ResizeObserver callbacks to a rAF, avoiding the
  // synchronous reflow storm mid-scroll that causes correction jitter/blanks.
  useAnimationFrameWithResizeObserver: true,
})))

const virtualItems = computed(() => rowVirtualizer.value.getVirtualItems())

const virtualPaddingTop = computed(() => {
  const items = virtualItems.value
  return items.length > 0 ? items[0].start : 0
})

const virtualPaddingBottom = computed(() => {
  const items = virtualItems.value
  if (items.length === 0) return 0
  return rowVirtualizer.value.getTotalSize() - items[items.length - 1].end
})

const measureElement = (el: any) => {
  if (el) {
    rowVirtualizer.value.measureElement(el as Element)
  }
}

type RowIdentityToken = string | number | object | symbol

const rowIdentityKeys = computed<RowIdentityToken[]>(() =>
  (sortedData.value ?? []).map((row) => {
    const stableKey = resolveStableRowKey(row)
    if (stableKey !== undefined) return stableKey

    // Object references survive pure reordering but change across page/filter results.
    // Primitive rows have no stable identity, so force conservative invalidation.
    return row !== null && typeof row === 'object' ? row : Symbol('unstable-row')
  })
)

const hasSameRowIdentitySet = (
  current: RowIdentityToken[],
  previous: RowIdentityToken[]
) => {
  if (current.length !== previous.length) return false
  const currentKeys = new Set(current)
  const previousKeys = new Set(previous)
  // Duplicate keys make row-to-cache ownership ambiguous, even when the unique
  // key set looks unchanged (for example [1, 1, 2] -> [1, 2, 2]).
  if (currentKeys.size !== current.length || previousKeys.size !== previous.length) return false
  return [...currentKeys].every(key => previousKeys.has(key))
}

watch(
  rowIdentityKeys,
  (current, previous) => {
    if (hasSameRowIdentitySet(current, previous)) return

    // The virtualizer owns caches across option updates. A new page/filter result
    // must release detached rows and sizes, while pure reordering keeps them.
    rowVirtualizer.value.measureElement(null)
    rowVirtualizer.value.measure()
  },
  { flush: 'post' }
)

// Unified render row list: windowed rows (needing virtualizer measurement)
// when virtualizing, every row (no measurement) when not. The template
// shares one row/cell structure across both modes.
const renderRows = computed<Array<{ index: number; row: any; measure: boolean }>>(() => {
  const data = sortedData.value ?? []
  if (shouldVirtualize.value) {
    return virtualItems.value.map(vr => ({ index: vr.index, row: data[vr.index], measure: true }))
  }
  return data.map((row, index) => ({ index, row, measure: false }))
})

const hasActionsColumn = computed(() => {
  return props.columns.some(column => column.key === 'actions')
})

const hasSelectColumn = computed(() => {
  return props.columns.length > 0 && props.columns[0].key === 'select'
})

// Sticky-column classes. Sticky columns are opaque with one border; there is
// no gradient shadow to compute here any more.
const getStickyColumnClass = (column: Column, index: number) => {
  const classes: string[] = []

  if (props.stickyFirstColumn) {
    if (hasSelectColumn.value) {
      // First column is the checkbox column: pin the first two (checkbox + name).
      if (index === 0) {
        classes.push('sticky-col sticky-col-left-first')
      } else if (index === 1) {
        classes.push('sticky-col sticky-col-left-second')
      }
    } else {
      if (index === 0) {
        classes.push('sticky-col sticky-col-left')
      }
    }
  }

  if (props.stickyActionsColumn && column.key === 'actions') {
    classes.push('sticky-col sticky-col-right')
  }

  return classes.join(' ')
}

// Horizontal cell padding shrinks as column count grows, so a ten-plus
// column table stays legible instead of running out of width. Expressed as
// a CSS custom property on the table root rather than a Tailwind px-* class.
const adaptivePaddingPx = computed(() => {
  const columnCount = props.columns.length
  if (columnCount >= 10) return 8
  if (columnCount >= 7) return 12
  if (columnCount >= 5) return 16
  return 24
})

// Deterministic skeleton bar widths: no Math.random (which would differ
// between server and client renders), just a small fixed set that varies
// enough to read as content rather than a single repeated block.
const SKELETON_WIDTHS = ['72%', '58%', '64%', '48%', '68%', '55%']
const skeletonWidth = (rowIndex: number, colIndex: number) =>
  SKELETON_WIDTHS[(rowIndex + colIndex) % SKELETON_WIDTHS.length]

// Init + keep persisted sort state consistent with current columns
const didInitSort = ref(false)

onMounted(() => {
  const initial = resolveInitialSortState()
  applySortState(initial)
  didInitSort.value = true
})

watch(
  columnsSignature,
  () => {
    // If current sort key is no longer sortable/visible, fall back to default/persisted.
    const normalized = normalizeSortKey(sortKey.value)
    if (!sortKey.value) {
      const initial = resolveInitialSortState()
      applySortState(initial)
      return
    }

    if (!normalized) {
      const fallback = resolveInitialSortState()
      if (fallback) {
        applySortState(fallback)
      } else {
        sortKey.value = ''
        sortOrder.value = 'asc'
      }
    }
  },
  { flush: 'post' }
)

watch(
  [sortKey, sortOrder],
  ([nextKey, nextOrder]) => {
    if (!didInitSort.value) return
    if (!props.sortStorageKey) return
    const key = normalizeSortKey(nextKey)
    if (!key) return
    writePersistedSortState({ key, order: normalizeSortOrder(nextOrder) })
  },
  { flush: 'post' }
)

defineExpose({
  virtualizer: rowVirtualizer,
  shouldVirtualize,
  sortedData,
  resolveRowKey,
  tableWrapperEl: tableWrapperRef,
})
</script>

<style scoped>
/* Horizontal + vertical scroll container. `.table-wrapper` is a public hook:
 * TablePageLayout's `:deep(.table-wrapper)` sets flex:1 / overflow on this
 * exact class name, so it is kept even though everything else here is new. */
.table-wrapper {
  /* 40px: the selection column's fixed width from the table anatomy spec. */
  --select-col-width: 40px;
  --dt-row-h: var(--s2a-h-md); /* 36px, the default density */
  position: relative;
  overflow-x: auto;
  overflow-y: auto;
  flex: 1;
  min-height: 0;
  isolation: isolate;
}

.table-wrapper[data-density='compact'] {
  --dt-row-h: var(--s2a-h-xs); /* 28px, nested in a card or modal */
}

.table-wrapper[data-density='twoLine'] {
  --dt-row-h: 44px; /* a cell that pairs a status with its reason */
}

.dt-table {
  width: 100%;
  min-width: max-content;
  border-collapse: collapse;
}

/* --- header ---------------------------------------------------------- */

.dt-thead {
  position: sticky;
  top: 0;
  z-index: 200;
  background: var(--card);
}

.dt-tr-head {
  height: var(--s2a-h-sm); /* 32px, fixed regardless of row density */
}

.dt-th {
  padding: 0 var(--dt-cell-px, 12px);
  border-bottom: 1px solid var(--border);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  font-weight: 400;
  text-align: left;
  white-space: nowrap;
  vertical-align: middle;
}

.dt-th[data-sortable] {
  cursor: pointer;
}

/* Faint caret preview on hover, background tints like any other hoverable
 * chrome. Never a colour category, just state feedback. */
.dt-th[data-sortable]:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.dt-th[data-sorted] {
  color: var(--foreground);
}

.dt-th--select {
  width: var(--select-col-width);
  min-width: var(--select-col-width);
  text-align: center;
}

.dt-th__inner {
  display: flex;
  align-items: center;
  gap: 4px;
}

.dt-align-center {
  justify-content: center;
}

.dt-align-end {
  justify-content: flex-end;
}

/* One caret, on the sorted column, pointing the way the data runs. Twelve
 * columns of permanent grey arrows said nothing; this says one thing. */
.dt-th__caret {
  font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */
  opacity: 0;
  transition: opacity var(--motion-hover);
}

.dt-th[data-sortable]:hover .dt-th__caret {
  opacity: 0.45;
}

.dt-th[data-sorted] .dt-th__caret {
  opacity: 1;
}

/* --- body -------------------------------------------------------------- */

.dt-tbody {
  position: relative;
  z-index: 0;
}

/* `height` on a table row is a MINIMUM, not a maximum -- CSS table layout grows
 * a row to fit its tallest cell and there is no honoured `max-height` for
 * `display: table-row`. So `--dt-row-h` sets the density floor; it cannot
 * enforce a ceiling, and it is not meant to.
 *
 * Measured 46.5px on /admin/users rather than 36. That is not a defect here:
 * UsersView is still unconverted and renders its groups cell as a vertically
 * stacked chip list (`flex flex-col`, UsersView.vue:335), which is genuinely
 * taller than one line. Clamping the cell would silently clip those chips.
 *
 * If a table legitimately needs taller rows, declare it -- `data-density` is the
 * knob, and `twoLine` already exists for it. Do not add an overflow clamp here
 * to make a number match; that trades a visible height for invisible data loss.
 */
.dt-tr {
  height: var(--dt-row-h);
  background: var(--card);
  border-top: 1px solid var(--border-subtle);
  /* Background only. Never border-color (ground rule 6). */
  transition: background var(--motion-hover);
}

.dt-tr[data-clickable] {
  cursor: pointer;
}

.dt-tr:hover {
  background: var(--sidebar-accent);
}

/* Selected rows tint; they never change border. */
.dt-tr[data-selected],
.dt-tr[data-selected]:hover {
  background: var(--brand-tint);
}

.dt-td {
  padding: 0 var(--dt-cell-px, 12px);
  overflow: hidden;
  color: var(--foreground);
  font-size: var(--fs-md);
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: middle;
}

.dt-td--select {
  width: var(--select-col-width);
  min-width: var(--select-col-width);
  text-align: center;
}

.dt-td--empty {
  padding: 0;
  white-space: normal;
}

/* --- sticky columns -----------------------------------------------------
 * Opaque fill plus one border. That is the whole requirement; no gradient,
 * no shadow. */
.sticky-col {
  position: sticky;
  z-index: 20;
  background: var(--card);
}

.sticky-col-left,
.sticky-col-left-second {
  left: 0;
  border-right: 1px solid var(--border);
}

.sticky-col-left-first {
  left: 0;
}

.sticky-col-left-second {
  left: var(--select-col-width);
}

.sticky-col-right {
  right: 0;
  border-left: 1px solid var(--border);
}

.dt-th.sticky-col {
  z-index: 220;
}

.dt-tr:hover .sticky-col {
  background: var(--sidebar-accent);
}

.dt-tr[data-selected] .sticky-col {
  background: var(--brand-tint);
}

/* --- loading skeleton -----------------------------------------------------
 * Flat, static bars. No pulse, no sweep (ground rule 7 / the loading rule). */
.dt-tr--skeleton:hover {
  background: var(--card);
}

.dt-skel {
  display: block;
  height: 8px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
}

/* --- mobile card fallback ------------------------------------------------
 * Kept exactly as built (same branch, same data-test/data-field hooks);
 * only the chrome moves off Tailwind utilities onto tokens. */
.dt-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.dt-cards__selectall {
  display: flex;
  justify-content: flex-end;
  padding: 0 2px;
}

.dt-card {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.dt-card[data-clickable] {
  cursor: pointer;
}

.dt-card[data-selected] {
  background: var(--brand-tint);
  border-color: var(--brand-line);
}

.dt-card__body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 16px;
}

.dt-card__select {
  display: flex;
  justify-content: flex-end;
}

.dt-card__field {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
}

.dt-card__label {
  font-size: var(--fs-xs);
  color: var(--muted-foreground);
}

.dt-card__value {
  text-align: right;
  font-size: var(--fs-md);
  color: var(--foreground);
}

.dt-card__actions {
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
}

.dt-card__actions-skel {
  padding-top: 12px;
  border-top: 1px solid var(--border-subtle);
}

.dt-card--empty {
  padding: 8px;
}

/* `min-w-0` / `max-w-full` are literal shrink-to-fit hooks: not Tailwind
 * utilities here, plain CSS rules named after what they do so a flex child
 * with a long value can still ellipsis instead of forcing the card wide. */
.min-w-0 {
  min-width: 0;
}

.max-w-full {
  max-width: 100%;
}
</style>

<style>
/* ==========================================================================
   Scrollbar visibility fix.
   The app's global style sheet sets `scrollbar-color: transparent` on every
   element for a hidden-until-hover look; a table that scrolls both ways
   needs its scrollbar visible at rest, or the fact that it scrolls at all is
   not discoverable. Global, not scoped, because scoped styles cannot reach
   the ::-webkit-scrollbar pseudo-elements reliably across browsers.
   ========================================================================== */

.table-wrapper {
  scrollbar-width: auto !important;
}

.table-wrapper::-webkit-scrollbar {
  height: 12px !important;
  width: 12px !important;
  display: block !important;
  background-color: transparent !important;
}

.table-wrapper::-webkit-scrollbar-track {
  background-color: color-mix(in oklch, var(--foreground) 3%, transparent) !important;
  border-radius: 6px !important;
  margin: 0 4px !important;
}

.table-wrapper::-webkit-scrollbar-thumb {
  background-color: color-mix(in oklch, var(--muted-foreground) 75%, transparent) !important;
  border-radius: 6px !important;
  border: 2px solid transparent !important;
  background-clip: padding-box !important;
  -webkit-appearance: none !important;
}
.table-wrapper::-webkit-scrollbar-thumb:hover {
  background-color: color-mix(in oklch, var(--muted-foreground) 90%, transparent) !important;
}

@supports (-moz-appearance: none) {
  .table-wrapper {
    scrollbar-width: thin !important;
    scrollbar-color: color-mix(in oklch, var(--muted-foreground) 50%, transparent) color-mix(in oklch, var(--foreground) 3%, transparent) !important;
  }
}
</style>
