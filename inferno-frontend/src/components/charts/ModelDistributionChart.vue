<script setup lang="ts">
/**
 * ModelDistributionChart -- part 09, section 02 to 04.
 *
 * Same migration as GroupDistributionChart and EndpointDistributionChart
 * (see either for the full argument against the doughnut). This file's own
 * note from the prototype: "Has a second mode for ranking items with an
 * explicit other slice at slate, which is already the right instinct. Both
 * modes become the bar list." So both `model_distribution` and
 * `spending_ranking` render through the same `.dist2` bar-list markup below;
 * only the row shape (a model vs. a ranked user) and the click behaviour
 * (expand a breakdown vs. emit `ranking-click`) differ.
 *
 * Model names go mono, matching the prototype's own barList example (every
 * named row uses `--font-mono`): a model id like claude-sonnet-4-5 is an
 * identifier, not prose.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Segmented from '@/components/common/Segmented.vue'
import UserBreakdownSubTable from './UserBreakdownSubTable.vue'
import type { ModelStat, UserSpendingRankingItem, UserBreakdownItem } from '@/types'
import { getUserBreakdown } from '@/api/admin/dashboard'

const { t } = useI18n()

type DistributionMetric = 'tokens' | 'actual_cost'
type ModelSource = 'requested' | 'upstream' | 'mapping'
type RankingDisplayItem = UserSpendingRankingItem & { isOther?: boolean }

const props = withDefaults(defineProps<{
  modelStats: ModelStat[]
  upstreamModelStats?: ModelStat[]
  mappingModelStats?: ModelStat[]
  source?: ModelSource
  enableRankingView?: boolean
  rankingItems?: UserSpendingRankingItem[]
  rankingTotalActualCost?: number
  rankingTotalRequests?: number
  rankingTotalTokens?: number
  loading?: boolean
  metric?: DistributionMetric
  showSourceToggle?: boolean
  showMetricToggle?: boolean
  enableBreakdown?: boolean
  showAccountCost?: boolean
  rankingLoading?: boolean
  rankingError?: boolean
  startDate?: string
  endDate?: string
  filters?: Record<string, any>
}>(), {
  upstreamModelStats: () => [],
  mappingModelStats: () => [],
  source: 'requested',
  enableRankingView: false,
  rankingItems: () => [],
  rankingTotalActualCost: 0,
  rankingTotalRequests: 0,
  rankingTotalTokens: 0,
  loading: false,
  metric: 'tokens',
  showSourceToggle: false,
  showMetricToggle: false,
  enableBreakdown: true,
  showAccountCost: true,
  rankingLoading: false,
  rankingError: false
})

const emit = defineEmits<{
  'update:metric': [value: DistributionMetric]
  'update:source': [value: ModelSource]
  'ranking-click': [item: UserSpendingRankingItem]
}>()

const activeView = ref<'model_distribution' | 'spending_ranking'>('model_distribution')

const viewItems = computed(() => [
  { value: 'model_distribution', label: t('admin.dashboard.viewModelDistribution') },
  { value: 'spending_ranking', label: t('admin.dashboard.viewSpendingRanking') }
])

const sourceItems = computed(() => [
  { value: 'requested', label: t('usage.requestedModel') },
  { value: 'upstream', label: t('usage.upstreamModel') },
  { value: 'mapping', label: t('usage.mapping') }
])

const metricItems = computed(() => [
  { value: 'tokens', label: t('admin.dashboard.metricTokens') },
  { value: 'actual_cost', label: t('admin.dashboard.metricActualCost') }
])

const title = computed(() =>
  !props.enableRankingView || activeView.value === 'model_distribution'
    ? t('admin.dashboard.modelDistribution')
    : t('admin.dashboard.spendingRankingTitle')
)

const expandedKey = ref<string | null>(null)
const breakdownItems = ref<UserBreakdownItem[]>([])
const breakdownLoading = ref(false)

const toggleBreakdown = async (id: string) => {
  const key = `model-${id}`
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
      model: id,
      model_source: props.source,
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

// --- model distribution bar list ------------------------------------------

const sourceStats = computed(() => {
  return props.source === 'upstream'
    ? props.upstreamModelStats
    : props.source === 'mapping'
      ? props.mappingModelStats
      : props.modelStats
})

const metricValue = (m: ModelStat): number =>
  toFiniteNumber(props.metric === 'actual_cost' ? m.actual_cost : m.total_tokens)

const sortedModels = computed(() => {
  if (!sourceStats.value?.length) return []
  return [...sourceStats.value].sort((a, b) => metricValue(b) - metricValue(a))
})

const modelTotalAll = computed(() => sortedModels.value.reduce((sum, m) => sum + metricValue(m), 0))

const TOP_N = 6

interface DistRow {
  key: string
  label: string
  displayValue: string
  pct: number
  clickable: boolean
  isOther: boolean
  mono: boolean
  onClick?: () => void
}

const displayValueFor = (raw: number): string =>
  props.metric === 'actual_cost' ? `$${formatCost(raw)}` : formatTokens(raw)

const modelRows = computed<DistRow[]>(() => {
  const total = modelTotalAll.value
  const top = sortedModels.value.slice(0, TOP_N)
  const tail = sortedModels.value.slice(TOP_N)

  const result: DistRow[] = top.map((m) => {
    const raw = metricValue(m)
    return {
      key: `model-${m.model}`,
      label: m.model,
      displayValue: displayValueFor(raw),
      pct: total > 0 ? (raw / total) * 100 : 0,
      clickable: props.enableBreakdown,
      isOther: false,
      mono: true,
      onClick: () => toggleBreakdown(m.model)
    }
  })

  if (tail.length) {
    const raw = tail.reduce((sum, m) => sum + metricValue(m), 0)
    result.push({
      key: 'others',
      label: t('charts.distribution.othersCount', { count: tail.length }),
      displayValue: displayValueFor(raw),
      pct: total > 0 ? (raw / total) * 100 : 0,
      clickable: false,
      isOther: true,
      mono: false
    })
  }

  return result
})

const modelSubtitle = computed(() => {
  const total = modelTotalAll.value
  const count = sortedModels.value.length
  const entity = t('charts.distribution.entityModels')
  return props.metric === 'actual_cost'
    ? t('charts.distribution.subtitleCost', { value: `$${formatCost(total)}`, count, entity })
    : t('charts.distribution.subtitleTokens', { value: formatTokens(total), count, entity })
})

// --- spending ranking bar list ---------------------------------------------

const getRankingUserLabel = (item: UserSpendingRankingItem): string => {
  if (item.username?.trim()) return item.username.trim()
  if (item.email?.trim()) return item.email.trim()
  return t('admin.redeem.userPrefix', { id: item.user_id })
}

const otherRankingItem = computed<RankingDisplayItem | null>(() => {
  if (!props.rankingItems?.length) return null

  const rankedActualCost = props.rankingItems.reduce((sum, item) => sum + toFiniteNumber(item.actual_cost), 0)
  const rankedRequests = props.rankingItems.reduce((sum, item) => sum + toFiniteNumber(item.requests), 0)
  const rankedTokens = props.rankingItems.reduce((sum, item) => sum + toFiniteNumber(item.tokens), 0)

  const otherActualCost = Math.max((props.rankingTotalActualCost || 0) - rankedActualCost, 0)
  const otherRequests = Math.max((props.rankingTotalRequests || 0) - rankedRequests, 0)
  const otherTokens = Math.max((props.rankingTotalTokens || 0) - rankedTokens, 0)

  if (otherActualCost <= 0.000001 && otherRequests <= 0 && otherTokens <= 0) return null

  return {
    user_id: 0,
    email: '',
    username: '',
    actual_cost: otherActualCost,
    requests: otherRequests,
    tokens: otherTokens,
    isOther: true
  }
})

const rankingDisplayItems = computed<RankingDisplayItem[]>(() => {
  if (!props.rankingItems?.length) return []
  return otherRankingItem.value
    ? [...props.rankingItems, otherRankingItem.value]
    : [...props.rankingItems]
})

const rankingTotal = computed(() =>
  rankingDisplayItems.value.reduce((sum, item) => sum + toFiniteNumber(item.actual_cost), 0)
)

const rankingRows = computed<DistRow[]>(() =>
  rankingDisplayItems.value.map((item, index) => {
    const raw = toFiniteNumber(item.actual_cost)
    return {
      key: item.isOther ? 'others' : `user-${item.user_id}-${index}`,
      label: item.isOther ? t('admin.dashboard.spendingRankingOther') : `#${index + 1} ${getRankingUserLabel(item)}`,
      displayValue: `$${formatCost(raw)}`,
      pct: rankingTotal.value > 0 ? (raw / rankingTotal.value) * 100 : 0,
      clickable: !item.isOther,
      isOther: !!item.isOther,
      mono: false,
      onClick: item.isOther ? undefined : () => emit('ranking-click', item)
    }
  })
)

const rankingSubtitle = computed(() => {
  const entity = t('charts.distribution.entityUsers')
  return t('charts.distribution.subtitleCost', {
    value: `$${formatCost(rankingTotal.value)}`,
    count: rankingDisplayItems.value.length,
    entity
  })
})

const onRowClick = (row: DistRow) => {
  row.onClick?.()
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
      <h3 class="dist2__title">{{ title }}</h3>
      <div class="dist2__controls">
        <Segmented
          v-if="showSourceToggle"
          :items="sourceItems"
          :model-value="source"
          :aria-label="t('admin.dashboard.modelDistribution')"
          @update:model-value="(v) => emit('update:source', v as ModelSource)"
        />
        <Segmented
          v-if="showMetricToggle"
          :items="metricItems"
          :model-value="metric"
          :aria-label="t('admin.dashboard.metricTokens')"
          @update:model-value="(v) => emit('update:metric', v as DistributionMetric)"
        />
        <Segmented
          v-if="enableRankingView"
          :items="viewItems"
          :model-value="activeView"
          :aria-label="t('admin.dashboard.viewModelDistribution')"
          @update:model-value="(v) => (activeView = v as 'model_distribution' | 'spending_ranking')"
        />
      </div>
    </div>

    <template v-if="!enableRankingView || activeView === 'model_distribution'">
      <div v-if="loading" class="dist2__state">
        <LoadingSpinner />
      </div>
      <div v-else-if="!modelRows.length" class="dist2__state">
        <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
      </div>
      <div v-else class="dist2__body">
        <div class="dist2__subtitle">{{ modelSubtitle }}</div>
        <div class="dist2__bars">
          <template v-for="row in modelRows" :key="row.key">
            <div
              class="dist2__row"
              :data-clickable="row.clickable || undefined"
              :data-expanded="expandedKey === row.key || undefined"
              @click="onRowClick(row)"
            >
              <span class="dist2__row-top">
                <span class="dist2__label" :class="row.mono && 'dist2__label--mono'" :title="row.label">{{ row.label }}</span>
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
    </template>

    <template v-else>
      <div v-if="rankingLoading" class="dist2__state">
        <LoadingSpinner />
      </div>
      <div v-else-if="rankingError" class="dist2__state">
        <EmptyState :title="t('admin.dashboard.failedToLoad')" icon="hgi-alert-02" />
      </div>
      <div v-else-if="!rankingRows.length" class="dist2__state">
        <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
      </div>
      <div v-else class="dist2__body">
        <div class="dist2__subtitle">{{ rankingSubtitle }}</div>
        <div class="dist2__bars">
          <div
            v-for="row in rankingRows"
            :key="row.key"
            class="dist2__row"
            :data-clickable="row.clickable || undefined"
            @click="onRowClick(row)"
          >
            <span class="dist2__row-top">
              <span class="dist2__label" :title="row.label">{{ row.label }}</span>
              <span class="dist2__value" :class="row.isOther && 'dist2__value--muted'">{{ row.displayValue }}</span>
            </span>
            <span class="dist2__track">
              <span
                class="dist2__fill"
                :style="{ width: row.pct + '%', background: row.isOther ? 'var(--muted-foreground)' : 'var(--brand)' }"
              />
            </span>
          </div>
        </div>
      </div>
    </template>
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
