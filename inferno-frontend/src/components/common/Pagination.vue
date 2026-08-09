<script setup lang="ts">
/**
 * Pagination — part 03, section 12.
 *
 * Migration note from the prototype:
 *   "Keep total, page, pageSize and the two emits. The range sentence replaces
 *    the bare '1-25 / 284'. Page sizes become inline buttons rather than a
 *    Select, since there are only a handful of values. A single page renders
 *    a sentence instead of a disabled pager."
 *
 * total / page / pageSize / pageSizeOptions / showPageSizeSelector / showJump
 * and both emits are unchanged: 28 call sites bind these today. The previous
 * two-layout markup (a `sm:hidden` mobile bar and a separate desktop bar) is
 * gone in favour of one row that wraps, since no call site or spec depends on
 * that split existing as two DOM branches.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getConfiguredTablePageSizeOptions, normalizeTablePageSize } from '@/utils/tablePreferences'
import { setPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()

interface Props {
  total: number
  page: number
  pageSize: number
  pageSizeOptions?: number[]
  showPageSizeSelector?: boolean
  showJump?: boolean
}

interface Emits {
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', pageSize: number): void
}

const props = withDefaults(defineProps<Props>(), {
  pageSizeOptions: () => getConfiguredTablePageSizeOptions(),
  showPageSizeSelector: true,
  showJump: false
})

const emit = defineEmits<Emits>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))

const fromItem = computed(() => {
  if (props.total === 0) return 0
  return (props.page - 1) * props.pageSize + 1
})

const toItem = computed(() => {
  const to = props.page * props.pageSize
  return to > props.total ? props.total : to
})

// The range sentence is the whole point of the migration: it replaces the
// bare "1-25 / 284" with words, using the same four pieces (showing/to/of/
// results) the source already had rather than a new combined key.
const rangeText = computed(
  () =>
    `${t('pagination.showing')} ${fromItem.value} ${t('pagination.to')} ${toItem.value} ${t('pagination.of')} ${props.total} ${t('pagination.results')}`
)

// Every configured size plus whatever the caller is currently on, so a
// persisted or server-driven pageSize outside the configured list still shows
// as a selected pill instead of silently matching nothing.
const pageSizeChoices = computed(() =>
  Array.from(new Set([...getConfiguredTablePageSizeOptions(), ...props.pageSizeOptions, normalizeTablePageSize(props.pageSize)])).sort(
    (a, b) => a - b
  )
)

const jumpPage = ref('')

const visiblePages = computed(() => {
  const pages: (number | string)[] = []
  const maxVisible = 7
  const total = totalPages.value

  if (total <= maxVisible) {
    for (let i = 1; i <= total; i++) pages.push(i)
  } else {
    pages.push(1)
    const start = Math.max(2, props.page - 2)
    const end = Math.min(total - 1, props.page + 2)
    if (start > 2) pages.push('...')
    for (let i = start; i <= end; i++) pages.push(i)
    if (end < total - 1) pages.push('...')
    pages.push(total)
  }

  return pages
})

const goToPage = (newPage: number) => {
  if (newPage >= 1 && newPage <= totalPages.value && newPage !== props.page) {
    emit('update:page', newPage)
  }
}

const handlePageSizeChange = (size: number) => {
  const newPageSize = normalizeTablePageSize(size)
  setPersistedPageSize(newPageSize)
  emit('update:pageSize', newPageSize)
}

const submitJump = () => {
  const value = jumpPage.value.trim()
  if (!value) return
  const pageNum = Number.parseInt(value, 10)
  if (Number.isNaN(pageNum)) return
  const nextPage = Math.min(Math.max(pageNum, 1), totalPages.value)
  jumpPage.value = ''
  goToPage(nextPage)
}
</script>

<template>
  <nav class="pager2" aria-label="Pagination">
    <p class="pager2__range">{{ rangeText }}</p>

    <div class="pager2__controls">
      <template v-if="totalPages > 1">
        <button
          type="button"
          class="pager2__nav"
          :disabled="page === 1"
          :aria-label="t('pagination.previous')"
          @click="goToPage(page - 1)"
        >
          <i class="hgi-stroke hgi-arrow-left-01" aria-hidden="true" />
        </button>

        <button
          v-for="(pageNum, index) in visiblePages"
          :key="`${pageNum}-${index}`"
          type="button"
          class="pager2__page"
          :disabled="typeof pageNum !== 'number'"
          :data-active="pageNum === page ? '' : undefined"
          :aria-current="pageNum === page ? 'page' : undefined"
          :aria-label="typeof pageNum === 'number' ? t('pagination.goToPage', { page: pageNum }) : undefined"
          @click="typeof pageNum === 'number' && goToPage(pageNum)"
        >
          {{ pageNum }}
        </button>

        <button
          type="button"
          class="pager2__nav"
          :disabled="page === totalPages"
          :aria-label="t('pagination.next')"
          @click="goToPage(page + 1)"
        >
          <i class="hgi-stroke hgi-arrow-right-01" aria-hidden="true" />
        </button>
      </template>

      <span v-if="totalPages > 1 && (showPageSizeSelector || showJump)" class="pager2__divider" aria-hidden="true" />

      <div v-if="showPageSizeSelector" class="pager2__sizes" role="group" :aria-label="t('pagination.perPage')">
        <button
          v-for="size in pageSizeChoices"
          :key="size"
          type="button"
          class="pager2__size"
          :data-active="size === pageSize ? '' : undefined"
          :aria-pressed="size === pageSize"
          @click="handlePageSizeChange(size)"
        >
          {{ size }}
        </button>
      </div>

      <span v-if="showPageSizeSelector && showJump" class="pager2__divider" aria-hidden="true" />

      <label v-if="showJump" class="pager2__jump">
        {{ t('pagination.jumpTo') }}
        <input
          v-model="jumpPage"
          type="text"
          inputmode="numeric"
          class="pager2__jump-input"
          :placeholder="t('pagination.jumpPlaceholder')"
          @keyup.enter="submitJump"
        />
        <button type="button" class="pager2__jump-btn" @click="submitJump">
          {{ t('pagination.jumpAction') }}
        </button>
      </label>
    </div>
  </nav>
</template>

<style scoped>
/* Footer chrome: sits flush under a table, so it takes a top border rather
 * than being its own bordered box (the table above already closes the card). */
.pager2 {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 16px;
  border-top: 1px solid var(--border-subtle);
  background: var(--card);
}

.pager2__range {
  margin: 0;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

.pager2__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.pager2__divider {
  width: 1px;
  height: 18px;
  margin: 0 4px;
  background: var(--border-subtle);
}

/* Every pager control is 28px (--s2a-h-xs), per the spec: prev/next, page
 * numbers, page sizes and the jump button all share one height token. */
.pager2__nav,
.pager2__page,
.pager2__size,
.pager2__jump-btn {
  height: var(--s2a-h-xs);
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  color: var(--body-copy);
  font-family: inherit;
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.pager2__nav {
  display: grid;
  place-items: center;
  width: var(--s2a-h-xs);
}
.pager2__nav i {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph */
  color: var(--foreground);
}
.pager2__nav:hover:not(:disabled) {
  background: var(--sidebar-accent);
}
.pager2__nav:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}
.pager2__nav:disabled i {
  color: var(--muted-foreground);
}

.pager2__page {
  min-width: var(--s2a-h-xs);
  padding: 0 7px;
  color: var(--body-copy);
}
.pager2__page:hover:not(:disabled):not([data-active]) {
  background: var(--sidebar-accent);
}
.pager2__page:disabled {
  cursor: default;
  color: var(--muted-foreground);
}
/* Only the current page is filled. Every other control on this bar, filled or
 * not, stays a neutral hover, never a category colour. */
.pager2__page[data-active] {
  background: var(--primary);
  color: var(--primary-foreground);
  font-weight: var(--fw-medium);
}

.pager2__sizes {
  display: flex;
  align-items: center;
  gap: 4px;
}
.pager2__size {
  padding: 0 8px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}
.pager2__size:hover {
  background: var(--sidebar-accent);
}
.pager2__size[data-active] {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.pager2__jump {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
.pager2__jump-input {
  width: 44px;
  height: var(--s2a-h-xs);
  padding: 0 7px;
  border: 1px solid var(--input);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  text-align: center;
  transition: background var(--motion-hover);
}
.pager2__jump-input:focus {
  outline: none;
  border-color: var(--ring-focus);
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.pager2__jump-btn {
  padding: 0 10px;
  color: var(--body-copy);
}
.pager2__jump-btn:hover {
  background: var(--sidebar-accent);
}
</style>
