<template>
  <div class="space-y-6">
    <!-- Date Range Filter -->
    <div class="dashboard-panel dashboard-panel--controls">
      <div class="dashboard-controls">
        <div class="dashboard-control-group">
          <span class="dashboard-control-label">{{ t('dashboard.timeRange') }}:</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="primary-action">
          {{ t('common.refresh') }}
        </button>
        <div class="dashboard-control-group dashboard-control-group--end">
          <span class="dashboard-control-label">{{ t('dashboard.granularity') }}:</span>
          <div class="dashboard-select-wrap">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="dashboard-panels">
      <!-- Model Distribution Chart -->
      <section class="dashboard-panel relative overflow-hidden">
        <div v-if="loading" class="chart-veil">
          <LoadingSpinner size="md" />
        </div>
        <h3 class="dashboard-panel__title mb-4">{{ t('dashboard.modelDistribution') }}</h3>
        <div class="flex flex-col items-center gap-4 sm:flex-row sm:gap-6">
          <div class="h-48 w-48 shrink-0">
            <DitherDonut
              v-if="slices.length"
              :slices="slices"
              :hovered="hovered"
              @update:hovered="hovered = $event"
            />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('dashboard.noDataAvailable') }}</div>
          </div>
          <div class="max-h-48 w-full min-w-0 flex-1 overflow-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="text-gray-500 dark:text-gray-400">
                  <th class="pb-2 text-left">{{ t('dashboard.model') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.requests') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.tokens') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.actual') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="model in models"
                  :key="model.model"
                  class="model-row"
                  :data-hot="hoveredModel === model.model || undefined"
                  :data-dim="hoveredModel !== null && hoveredModel !== model.model || undefined"
                  @pointerenter="hovered = slices.findIndex((s) => s.key === model.model) >= 0 ? slices.findIndex((s) => s.key === model.model) : null"
                  @pointerleave="hovered = null"
                >
                  <td class="model-name max-w-[100px] truncate py-1.5 text-gray-900 dark:text-white" :title="model.model">{{ model.model }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatNumber(model.requests) }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatTokens(model.total_tokens) }}</td>
                  <td class="py-1.5 text-right text-green-600 dark:text-green-400">${{ formatCost(model.actual_cost) }}</td>
                  <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">${{ formatCost(model.cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <!-- Token Usage Trend Chart -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import DitherDonut, { type DonutSlice } from '@/components/charts/DitherDonut.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import { useSequentialRamp } from '@/composables/useSequentialRamp'
import { useChartTokens } from '@/composables/useChartTokens'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'

const props = defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()
const ramp = useSequentialRamp(5)
const { tokens } = useChartTokens()
const hovered = ref<number | null>(null)

const TOP_SLICES = 4

const slices = computed<DonutSlice[]>(() => {
  const sorted = [...(props.models ?? [])]
    .filter((m) => (Number(m.total_tokens) || 0) > 0)
    .sort((a, b) => (Number(b.total_tokens) || 0) - (Number(a.total_tokens) || 0))
  if (!sorted.length) return []

  const head = sorted.slice(0, TOP_SLICES).map((m, i) => ({
    key: m.model,
    label: m.model,
    value: Number(m.total_tokens) || 0,
    color: ramp.value[i] ?? ''
  }))
  const tail = sorted.slice(TOP_SLICES)
  if (!tail.length) return head

  return [
    ...head,
    {
      key: '__other',
      // Not 'No Group' -- that means a key with no group assigned. This row is
      // the rolled-up tail of the model ranking.
      label: t('charts.distribution.othersCount', { count: tail.length }),
      value: tail.reduce((sum, m) => sum + (Number(m.total_tokens) || 0), 0),
      color: tokens.mutedForeground
    }
  ]
})

const hoveredModel = computed(() =>
  hovered.value != null ? slices.value[hovered.value]?.key ?? null : null
)
</script>

<style scoped>
/* Keep the panel readable while the customer-scoped chart request is pending. */
.chart-veil {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in oklch, var(--card) 78%, transparent);
}

.model-name {
  font-weight: var(--fw-medium);
}

.model-row {
  border-top: 1px solid var(--border-subtle);
  transition:
    background var(--motion-hover),
    opacity var(--motion-hover);
}

.model-row[data-hot] {
  background: var(--brand-tint);
}

.model-row[data-dim] {
  opacity: 0.5;
}

.dashboard-controls{display:flex;align-items:center;flex-wrap:wrap;gap:12px}
.dashboard-control-group{display:flex;align-items:center;gap:8px;min-width:0}
.dashboard-control-group--end{margin-left:auto}
.dashboard-control-label{color:var(--muted-foreground);font-size:var(--fs-sm);white-space:nowrap}
.dashboard-select-wrap{width:112px}

@media (max-width:720px){
  .dashboard-control-group--end{margin-left:0}
}
</style>
