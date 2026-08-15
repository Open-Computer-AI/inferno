<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DitherDonut, { type DonutSlice } from '@/components/charts/DitherDonut.vue'
import Segmented from '@/components/common/Segmented.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import { useSequentialRamp } from '@/composables/useSequentialRamp'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'

export interface UsageDistributionRow {
  key: string
  label: string
  requests: number
  tokens: number
  actualCost: number
  standardCost: number
}

type DistributionMetric = 'tokens' | 'actual_cost'

const props = withDefaults(defineProps<{
  title: string
  entityLabel?: string
  rows: UsageDistributionRow[]
  loading?: boolean
  metric?: DistributionMetric
}>(), {
  loading: false,
  metric: 'tokens',
})

const emit = defineEmits<{ 'update:metric': [value: DistributionMetric] }>()
const { t } = useI18n()
const { tokens } = useChartTokens()
const ramp = useSequentialRamp(6)
const hovered = ref<number | null>(null)
const TOP_ROWS = 6

const metricItems = computed(() => [
  { value: 'tokens', label: t('admin.dashboard.metricTokens') },
  { value: 'actual_cost', label: t('admin.dashboard.metricActualCost') },
])

const metricValue = (row: UsageDistributionRow) =>
  props.metric === 'actual_cost' ? Number(row.actualCost) || 0 : Number(row.tokens) || 0

const sortedRows = computed(() =>
  [...(props.rows ?? [])].sort((a, b) => metricValue(b) - metricValue(a))
)

const visibleRows = computed(() => sortedRows.value.slice(0, TOP_ROWS))
const total = computed(() => sortedRows.value.reduce((sum, row) => sum + metricValue(row), 0))
const entityHeading = computed(() => props.entityLabel || t('dashboard.model'))

const slices = computed<DonutSlice[]>(() => {
  const head = visibleRows.value.map((row, index) => ({
    key: row.key,
    label: row.label,
    value: metricValue(row),
    color: ramp.value[index] ?? tokens.mutedForeground,
  }))
  const tail = sortedRows.value.slice(TOP_ROWS)
  if (!tail.length) return head

  return [
    ...head,
    {
      key: '__other',
      label: t('dashboard.noGroup'),
      value: tail.reduce((sum, row) => sum + metricValue(row), 0),
      color: tokens.mutedForeground,
    },
  ]
})

const hoveredKey = computed(() => hovered.value == null ? null : slices.value[hovered.value]?.key ?? null)

const displayMetric = (row: UsageDistributionRow) =>
  '$' + formatCost(row.standardCost)
</script>

<template>
  <section class="usage-dist">
    <header class="usage-dist__head">
      <h2 class="usage-dist__title">{{ title }}</h2>
      <Segmented
        :items="metricItems"
        :model-value="metric"
        :aria-label="title"
        @update:model-value="(value) => emit('update:metric', value as DistributionMetric)"
      />
    </header>

    <div v-if="loading" class="usage-dist__state"><LoadingSpinner /></div>
    <div v-else-if="!visibleRows.length" class="usage-dist__state">
      <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
    </div>
    <div v-else class="usage-dist__body">
      <div class="usage-dist__donut">
        <DitherDonut :slices="slices" :hovered="hovered" @update:hovered="hovered = $event" />
      </div>

      <div class="usage-dist__table-wrap">
        <table class="usage-dist__table">
          <thead>
            <tr>
              <th class="usage-dist__name">{{ entityHeading }}</th>
              <th>{{ t('dashboard.requests') }}</th>
              <th>{{ t('dashboard.tokens') }}</th>
              <th class="usage-dist__actual">{{ t('dashboard.actual') }}</th>
              <th class="usage-dist__cost">Cost</th>
              <th>{{ t('dashboard.standard') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(row, index) in visibleRows"
              :key="row.key"
              :data-hot="hoveredKey === row.key || undefined"
              :data-dim="hoveredKey !== null && hoveredKey !== row.key || undefined"
              @pointerenter="hovered = index"
              @pointerleave="hovered = null"
            >
              <td class="usage-dist__name" :title="row.label">{{ row.label }}</td>
              <td>{{ formatNumber(row.requests) }}</td>
              <td>{{ formatTokens(row.tokens) }}</td>
              <td class="usage-dist__actual">{{ '$' }}{{ formatCost(row.actualCost) }}</td>
              <td class="usage-dist__cost">{{ displayMetric(row) }}</td>
              <td>{{ '$' }}{{ formatCost(row.standardCost) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <p v-if="total > 0" class="usage-dist__caption">
      {{ props.metric === 'actual_cost' ? '$' + formatCost(total) : formatTokens(total) }}
      {{ props.metric === 'actual_cost' ? 'actual cost' : 'tokens' }}
    </p>
  </section>
</template>

<style scoped>
.usage-dist{min-width:0;border:1px solid var(--border-subtle);border-radius:var(--r-lg);background:var(--card);padding:16px}
.usage-dist__head{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:12px}
.usage-dist__title{margin:0;color:var(--foreground);font-size:var(--fs-lg);font-weight:var(--fw-medium)}
.usage-dist__state{display:flex;min-height:240px;align-items:center;justify-content:center}
.usage-dist__body{display:flex;align-items:center;gap:16px;min-width:0}
.usage-dist__donut{width:210px;height:210px;flex:none}
.usage-dist__table-wrap{min-width:0;flex:1;overflow-x:auto}
.usage-dist__table{width:100%;border-collapse:collapse;color:var(--muted-foreground);font-size:var(--fs-2xs);font-variant-numeric:tabular-nums;white-space:nowrap}
.usage-dist__table th{padding:0 4px 8px;text-align:right;font-weight:var(--fw-medium)}
.usage-dist__table td{border-top:1px solid var(--border-subtle);padding:8px 4px;text-align:right;transition:background var(--motion-hover),opacity var(--motion-hover)}
.usage-dist__table .usage-dist__name{text-align:left;color:var(--foreground);overflow:hidden;max-width:150px;text-overflow:ellipsis}
.usage-dist__table tbody tr[data-hot] td{background:var(--brand-tint)}
.usage-dist__table tbody tr[data-dim] td{opacity:.5}
.usage-dist__actual{color:var(--success)!important}
.usage-dist__cost{color:var(--warm-strong)!important}
.usage-dist__caption{margin:10px 0 0;color:var(--muted-foreground);font-size:var(--fs-2xs);text-align:right}
@media (max-width:1100px){.usage-dist__body{align-items:flex-start;flex-direction:column}.usage-dist__donut{width:180px;height:180px;align-self:center}.usage-dist__table-wrap{width:100%}}
</style>
