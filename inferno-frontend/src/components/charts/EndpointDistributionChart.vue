<script setup lang="ts">
/**
 * EndpointDistributionChart -- part 09, section 02 to 04.
 *
 * Same migration as GroupDistributionChart (see its header comment for the
 * full argument): the doughnut, ArcElement, and the twelve-colour
 * chartColors array are gone, replaced by one ranked bar list sharing a
 * single --brand fill.
 *
 * This file's own note from the prototype: "Endpoint paths are identifiers,
 * so the labels go mono and truncate from the left, which keeps the
 * distinguishing tail visible." A path like /v1/organizations/acme/keys/42
 * loses nothing by losing its common prefix; truncating from the right would
 * hide exactly the part that tells two rows apart. `.dist2__label--start`
 * gets there with `direction: rtl` rather than a snippet of JS string
 * surgery, so the browser's own bidi + ellipsis algorithm does the trimming
 * and stays correct as the row resizes.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Segmented from '@/components/common/Segmented.vue'
import UserBreakdownSubTable from './UserBreakdownSubTable.vue'
import type { EndpointStat, UserBreakdownItem } from '@/types'
import { getUserBreakdown } from '@/api/admin/dashboard'

const { t } = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'
type EndpointSource = 'inbound' | 'upstream' | 'path'

const props = withDefaults(
  defineProps<{
    endpointStats: EndpointStat[]
    upstreamEndpointStats?: EndpointStat[]
    endpointPathStats?: EndpointStat[]
    loading?: boolean
    title?: string
    metric?: DistributionMetric
    source?: EndpointSource
    showMetricToggle?: boolean
    showSourceToggle?: boolean
    enableBreakdown?: boolean
    startDate?: string
    endDate?: string
    filters?: Record<string, any>
  }>(),
  {
    upstreamEndpointStats: () => [],
    endpointPathStats: () => [],
    loading: false,
    title: '',
    metric: 'tokens',
    source: 'inbound',
    showMetricToggle: false,
    showSourceToggle: false,
    enableBreakdown: true
  }
)

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
  'update:source': [value: EndpointSource]
}>()

const sourceItems = computed(() => [
  { value: 'inbound', label: t('usage.inbound') },
  { value: 'upstream', label: t('usage.upstream') },
  { value: 'path', label: t('usage.path') }
])

const metricItems = computed(() => [
  { value: 'tokens', label: t('admin.dashboard.metricTokens') },
  { value: 'actual_cost', label: t('admin.dashboard.metricActualCost') }
])

const expandedKey = ref<string | null>(null)
const breakdownItems = ref<UserBreakdownItem[]>([])
const breakdownLoading = ref(false)

const toggleBreakdown = async (endpoint: string) => {
  const key = `endpoint-${endpoint}`
  if (expandedKey.value === key) {
    expandedKey.value = null
    return
  }
  expandedKey.value = key
  breakdownLoading.value = true
  breakdownItems.value = []
  try {
    const res = await getUserBreakdown({
      ...props.filters,
      start_date: props.startDate,
      end_date: props.endDate,
      endpoint,
      endpoint_type: props.source,
    })
    breakdownItems.value = res.users || []
  } catch {
    breakdownItems.value = []
  } finally {
    breakdownLoading.value = false
  }
}

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

const metricValue = (item: EndpointStat): number =>
  toFiniteNumber(props.metric === 'actual_cost' ? item.actual_cost : item.total_tokens)

const sourceStats = computed(() => {
  return props.source === 'upstream'
    ? props.upstreamEndpointStats
    : props.source === 'path'
      ? props.endpointPathStats
      : props.endpointStats
})

const sortedStats = computed(() => {
  if (!sourceStats.value?.length) return []
  return [...sourceStats.value].sort((a, b) => metricValue(b) - metricValue(a))
})

const totalAll = computed(() => sortedStats.value.reduce((sum, item) => sum + metricValue(item), 0))

// Same as ModelDistributionChart: the bar list is the June design and renders
// any number of rows; TOP_N is compactness, so the tail expands in place rather
// than losing its drill-down.
const TOP_N = 6
const othersExpanded = ref(false)

interface DistRow {
  key: string
  label: string
  displayValue: string
  pct: number
  clickable: boolean
  isOther: boolean
  endpoint?: string
}

const displayValueFor = (raw: number): string =>
  props.metric === 'actual_cost' ? `$${formatCost(raw)}` : formatTokens(raw)

const rows = computed<DistRow[]>(() => {
  const total = totalAll.value
  const tail = sortedStats.value.slice(TOP_N)
  const top = othersExpanded.value ? sortedStats.value : sortedStats.value.slice(0, TOP_N)

  const result: DistRow[] = top.map((item) => {
    const raw = metricValue(item)
    return {
      key: `endpoint-${item.endpoint}`,
      label: item.endpoint,
      displayValue: displayValueFor(raw),
      pct: total > 0 ? (raw / total) * 100 : 0,
      clickable: props.enableBreakdown,
      isOther: false,
      endpoint: item.endpoint
    }
  })

  if (tail.length) {
    const raw = tail.reduce((sum, item) => sum + metricValue(item), 0)
    result.push({
      key: 'others',
      label: othersExpanded.value
        ? t('charts.distribution.othersCollapse')
        : t('charts.distribution.othersCount', { count: tail.length }),
      displayValue: othersExpanded.value ? '' : displayValueFor(raw),
      pct: othersExpanded.value ? 0 : total > 0 ? (raw / total) * 100 : 0,
      clickable: true,
      isOther: true
    })
  }

  return result
})

const subtitle = computed(() => {
  const total = totalAll.value
  const count = sortedStats.value.length
  const entity = t('charts.distribution.entityEndpoints')
  return props.metric === 'actual_cost'
    ? t('charts.distribution.subtitleCost', { value: `$${formatCost(total)}`, count, entity })
    : t('charts.distribution.subtitleTokens', { value: formatTokens(total), count, entity })
})

const onRowClick = (row: DistRow) => {
  if (row.isOther) {
    othersExpanded.value = !othersExpanded.value
    return
  }
  if (!row.clickable || row.endpoint === undefined) return
  toggleBreakdown(row.endpoint)
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatCost = (value: number): string => {
  if (value >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}
</script>

<template>
  <div class="dist2">
    <div class="dist2__head">
      <h3 class="dist2__title">{{ title || t('usage.endpointDistribution') }}</h3>
      <div v-if="showSourceToggle || showMetricToggle" class="dist2__controls">
        <Segmented
          v-if="showSourceToggle"
          :items="sourceItems"
          :model-value="source"
          :aria-label="t('usage.endpointDistribution')"
          @update:model-value="(v) => emit('update:source', v as EndpointSource)"
        />
        <Segmented
          v-if="showMetricToggle"
          :items="metricItems"
          :model-value="metric"
          :aria-label="t('admin.dashboard.metricTokens')"
          @update:model-value="(v) => emit('update:metric', v as DistributionMetric)"
        />
      </div>
    </div>

    <div v-if="loading" class="dist2__state">
      <LoadingSpinner />
    </div>
    <div v-else-if="!rows.length" class="dist2__state">
      <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
    </div>
    <div v-else class="dist2__body">
      <div class="dist2__subtitle">{{ subtitle }}</div>
      <div class="dist2__bars">
        <template v-for="row in rows" :key="row.key">
          <div
            class="dist2__row"
            :data-clickable="row.clickable || undefined"
            :data-expanded="expandedKey === row.key || undefined"
            @click="onRowClick(row)"
          >
            <span class="dist2__row-top">
              <!-- Identifier: mono, truncated from the start so the tail
                   (the part that actually distinguishes two paths) stays
                   visible. -->
              <span
                class="dist2__label"
                :class="!row.isOther && 'dist2__label--mono dist2__label--start'"
                :title="row.label"
              >{{ row.label }}</span>
              <span class="dist2__value" :class="row.isOther && 'dist2__value--muted'">{{ row.displayValue }}</span>
            </span>
            <span class="dist2__track">
              <span
                class="dist2__fill"
                :style="{ width: row.pct + '%', background: row.isOther ? 'var(--muted-foreground)' : 'var(--brand)' }"
              />
            </span>
          </div>
          <div v-if="expandedKey === row.key" class="dist2__expand">
            <UserBreakdownSubTable :items="breakdownItems" :loading="breakdownLoading" />
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dist2 {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  padding: 16px;
}

.dist2__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 12px;
}

.dist2__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.dist2__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.dist2__state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 176px;
}

.dist2__subtitle {
  margin-bottom: 12px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.dist2__bars {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.dist2__row {
  display: block;
  min-width: 0;
  padding: 4px 6px;
  margin: 0 -6px;
  border: 0;
  border-radius: var(--r-sm);
  background: transparent;
  text-align: left;
  transition: background var(--motion-hover);
}

.dist2__row[data-clickable] {
  cursor: pointer;
}

.dist2__row[data-clickable]:hover,
.dist2__row[data-expanded] {
  background: var(--sidebar-accent);
}

.dist2__row-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  min-width: 0;
  margin-bottom: 3px;
}

.dist2__label {
  overflow: hidden;
  min-width: 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dist2__label--mono {
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

/* Truncate from the start: direction:rtl flips which end the browser trims
   at, unicode-bidi:plaintext keeps the path's own characters in their
   natural (LTR) order so it still reads correctly, just clipped on the left. */
.dist2__label--start {
  direction: rtl;
  text-align: left;
  unicode-bidi: plaintext;
}

.dist2__value {
  flex-shrink: 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.dist2__value--muted {
  color: var(--muted-foreground);
}

.dist2__track {
  display: block;
  height: 6px;
  border-radius: 3px;
  background: var(--surface-subtle);
  overflow: hidden;
}

.dist2__fill {
  display: block;
  height: 100%;
  border-radius: 3px;
  transition: width var(--t-slow) var(--ease-out);
}
</style>
