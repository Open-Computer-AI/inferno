<template>
  <div class="space-y-6">
    <!-- Date Range Filter -->
    <div class="surface-card p-4">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('dashboard.timeRange') }}:</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
          {{ t('common.refresh') }}
        </button>
        <div class="ml-auto flex items-center gap-2">
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('dashboard.granularity') }}:</span>
          <div class="w-28">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Model Distribution Chart -->
      <div class="surface-card relative overflow-hidden p-4">
        <div v-if="loading" class="chart-veil">
          <LoadingSpinner size="md" />
        </div>
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.modelDistribution') }}</h3>
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
      </div>

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

/*
 * The old doughnut carried eight hardcoded hues -- #3b82f6, #10b981, #f59e0b
 * and so on -- which is the twelve-hue collision part 09 removed from every
 * other distribution in the app, still sitting on the customer dashboard. The
 * sequential brand ramp replaces it, so the slices read as ordered parts of
 * one total and the card stops looking borrowed from another product.
 *
 * Capped at four models plus an explicit Other, for two reasons: the ramp is
 * five steps, and past five a slice is thinner than the gap between slices.
 * The table beside it still lists every model, so nothing is hidden -- the
 * donut is the shape and the table is the detail.
 */
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
      label: t('dashboard.noGroup'),
      value: tail.reduce((sum, m) => sum + (Number(m.total_tokens) || 0), 0),
      /* Deliberately off-ramp: Other is an aggregate, not a peer of the named
         slices, and giving it the next ramp step would invite reading it as
         one more model. */
      color: tokens.mutedForeground
    }
  ]
})

/** Table row index for the model a slice represents, or null for Other. */
const hoveredModel = computed(() =>
  hovered.value != null ? slices.value[hovered.value]?.key ?? null : null
)
</script>

<style scoped>
/*
 * The loading veil was bg-white/50 + backdrop-blur-sm -- glass, which ground
 * rule 7 bans, and which also hard-coded white so it inverted wrongly in dark
 * mode. An opaque-enough --card does the same job: the point is to mute what
 * is behind the spinner, and blurring it was never what achieved that.
 */
.chart-veil {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in oklch, var(--card) 78%, transparent);
}

/* 600, not Tailwind's font-medium (500): 500 is not one of the two weights
   the scale has, so it silently resolved to one of them anyway. */
.model-name {
  font-weight: var(--fw-medium);
}

/* The ring and the table are one control: hovering either marks the same
   model. Opacity and background only -- never border-color (ground rule 6).
   The row divider moves here off `border-gray-100 dark:border-dark-700`: the
   dark- palette is dead, and one token replaces the light/dark pair. */
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
</style>
