<script setup lang="ts">
/**
 * GroupDistributionChart -- part 09, section 02 to 04.
 *
 * Migration note from the prototype:
 *   "A ranked horizontal bar list answers 'how is this total split up' with
 *    length, which is the encoding people read most accurately. It also
 *    needs no palette at all, which means the twelve-hue collision in
 *    section 01 disappears rather than being managed." And, on this file
 *    specifically: "Ten colours for what is usually three or four groups.
 *    The unit becomes requests or spend, stated in the header."
 *
 * The doughnut, its ArcElement registration, and the ten-colour chartColors
 * array are gone. Every bar shares one fill (--brand, the plain-CSS
 * equivalent of the ramp's sr-1 step -- see useChartTokens.ts for why the
 * ramp itself only exists as literals inside that composable and not as a
 * CSS custom property here); only the "N others" tail row is deliberately
 * off-brand, in --muted-foreground.
 *
 * The old side-by-side table (group, requests, tokens, actual, account,
 * standard) is not carried over one for one: the bar list is now the only
 * summary view, and it only ever shows the one metric the toggle has
 * selected, per the mockup's own row ("Reading a value takes one saccade").
 * The other numbers are not lost -- they are one click away in
 * UserBreakdownSubTable, which still shows the full column set for whatever
 * group a caller expands.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Segmented from '@/components/common/Segmented.vue'
import UserBreakdownSubTable from './UserBreakdownSubTable.vue'
import type { GroupStat, UserBreakdownItem } from '@/types'
import { getUserBreakdown } from '@/api/admin/dashboard'

const { t } = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'

const props = withDefaults(defineProps<{
  groupStats: GroupStat[]
  loading?: boolean
  metric?: DistributionMetric
  showMetricToggle?: boolean
  enableBreakdown?: boolean
  showAccountCost?: boolean
  startDate?: string
  endDate?: string
  filters?: Record<string, any>
}>(), {
  loading: false,
  metric: 'tokens',
  showMetricToggle: false,
  enableBreakdown: true,
  showAccountCost: true,
})

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
}>()

const metricItems = computed(() => [
  { value: 'tokens', label: t('admin.dashboard.metricTokens') },
  { value: 'actual_cost', label: t('admin.dashboard.metricActualCost') }
])

const expandedKey = ref<string | null>(null)
const breakdownItems = ref<UserBreakdownItem[]>([])
const breakdownLoading = ref(false)

const toggleBreakdown = async (id: number) => {
  const key = `group-${id}`
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
      group_id: Number(id),
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

const metricValue = (g: GroupStat): number =>
  toFiniteNumber(props.metric === 'actual_cost' ? g.actual_cost : g.total_tokens)

const sortedGroups = computed(() => {
  if (!props.groupStats?.length) return []
  return [...props.groupStats].sort((a, b) => metricValue(b) - metricValue(a))
})

const totalAll = computed(() => sortedGroups.value.reduce((sum, g) => sum + metricValue(g), 0))

const TOP_N = 6

interface DistRow {
  key: string
  label: string
  displayValue: string
  pct: number
  clickable: boolean
  isOther: boolean
  groupId?: number
}

const displayValueFor = (raw: number): string =>
  props.metric === 'actual_cost' ? `$${formatCost(raw)}` : formatTokens(raw)

const rows = computed<DistRow[]>(() => {
  const total = totalAll.value
  const top = sortedGroups.value.slice(0, TOP_N)
  const tail = sortedGroups.value.slice(TOP_N)

  const result: DistRow[] = top.map((g) => {
    const raw = metricValue(g)
    return {
      key: `group-${g.group_id}`,
      label: g.group_name || t('admin.dashboard.noGroup'),
      displayValue: displayValueFor(raw),
      pct: total > 0 ? (raw / total) * 100 : 0,
      clickable: props.enableBreakdown && g.group_id > 0,
      isOther: false,
      groupId: g.group_id
    }
  })

  if (tail.length) {
    const raw = tail.reduce((sum, g) => sum + metricValue(g), 0)
    result.push({
      key: 'others',
      label: t('charts.distribution.othersCount', { count: tail.length }),
      displayValue: displayValueFor(raw),
      pct: total > 0 ? (raw / total) * 100 : 0,
      clickable: false,
      isOther: true
    })
  }

  return result
})

const subtitle = computed(() => {
  const total = totalAll.value
  const count = sortedGroups.value.length
  const entity = t('charts.distribution.entityGroups')
  return props.metric === 'actual_cost'
    ? t('charts.distribution.subtitleCost', { value: `$${formatCost(total)}`, count, entity })
    : t('charts.distribution.subtitleTokens', { value: formatTokens(total), count, entity })
})

const onRowClick = (row: DistRow) => {
  if (!row.clickable || row.groupId === undefined) return
  toggleBreakdown(row.groupId)
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatCost = (value: number | null | undefined): string => {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1000) return (safeValue / 1000).toFixed(2) + 'K'
  if (safeValue >= 1) return safeValue.toFixed(2)
  if (safeValue >= 0.01) return safeValue.toFixed(3)
  return safeValue.toFixed(4)
}
</script>

<template>
  <div class="dist2">
    <div class="dist2__head">
      <h3 class="dist2__title">{{ t('admin.dashboard.groupDistribution') }}</h3>
      <div v-if="showMetricToggle" class="dist2__controls">
        <Segmented
          :items="metricItems"
          :model-value="metric"
          :aria-label="t('admin.dashboard.groupDistribution')"
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
              <span class="dist2__label">{{ row.label }}</span>
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
            <UserBreakdownSubTable :items="breakdownItems" :loading="breakdownLoading" :show-account-cost="showAccountCost" />
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
  /* Background only, never border-color (ground rule 6). */
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
  /* Width only. --t-slow collapses to 0ms under prefers-reduced-motion at
     the token level (motion.css), so no extra media query is needed. */
  transition: width var(--t-slow) var(--ease-out);
}
</style>
