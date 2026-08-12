<script setup lang="ts">
/**
 * Token volume over time, as a field of drifting tiles, with the cache hit
 * rate on a strip beneath it.
 *
 * WHY THIS ONE MOVED OFF CHART.JS
 * It renders on four routes -- both dashboards and both usage pages -- so it
 * is the single highest-leverage chart in the app, and leaving it as a
 * Chart.js line while the dashboard panels beside it were tile fields made the
 * page read as two products stitched together.
 *
 * STACKED, NOT THREE OVERLAID LINES
 * The old chart drew input filled, output as a line, and cache as a dashed
 * line -- three encodings for three series that actually SUM to the total
 * shown everywhere else on the page. Stacking says that: the envelope is total
 * tokens, and each band is its share. It also removes the dashed line, which
 * was carrying "this one is different" rather than any property of the data.
 *
 * The hover-sync machinery between the two charts is gone with it. DitherArea
 * owns its own scrubber, and two independent scrubbers on stacked plots would
 * fight; the strip is a readout, so it gets the value in its header instead of
 * a second interactive surface.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TrendDataPoint } from '@/types'
import DitherArea from './DitherArea.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import { useSequentialRamp } from '@/composables/useSequentialRamp'

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
}>()

const { t } = useI18n()
const { tokens } = useChartTokens()
const ramp = useSequentialRamp(3)

/*
 * Cache hit rate is measured against ALL prompt tokens (input + both halves of
 * cache), so Anthropic-style cache creation counts toward the denominator too.
 */
const rateFor = (d: TrendDataPoint): number => {
  const totalPromptTokens = d.input_tokens + d.cache_read_tokens + d.cache_creation_tokens
  return totalPromptTokens > 0 ? (d.cache_read_tokens / totalPromptTokens) * 100 : 0
}

const hasData = computed(() => (props.trendData?.length ?? 0) > 0)

/* Dates only, no time: the strip and the main field share these, and an hourly
   bucket's full stamp is too long to sit under a 34px strip. */
const labels = computed(() => props.trendData.map((d) => d.date.slice(0, 10)))

/* Bottom-first, so the deepest ramp step is the largest band and the stack
   reads dark to pale from the floor up. */
const mainSeries = computed(() => [
  {
    key: 'cache',
    label: t('admin.dashboard.cache'),
    values: props.trendData.map((d) => d.cache_creation_tokens + d.cache_read_tokens),
    color: ramp.value[0] ?? ''
  },
  {
    key: 'input',
    label: t('admin.dashboard.input'),
    values: props.trendData.map((d) => d.input_tokens),
    color: ramp.value[1] ?? ''
  },
  {
    key: 'output',
    label: t('admin.dashboard.output'),
    values: props.trendData.map((d) => d.output_tokens),
    color: ramp.value[2] ?? ''
  }
])

const rateSeries = computed(() => [
  {
    key: 'rate',
    label: t('usage.cacheHitRate'),
    values: props.trendData.map(rateFor),
    color: ramp.value[1] ?? ''
  }
])

const latestRateLabel = computed(() => {
  if (!hasData.value) return ''
  return `${rateFor(props.trendData[props.trendData.length - 1]).toFixed(1)}%`
})

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)
</script>

<template>
  <div class="trend2">
    <h3 class="trend2__title">{{ t('admin.dashboard.tokenUsageTrend') }}</h3>

    <div v-if="loading" class="trend2__state">
      <LoadingSpinner />
    </div>
    <div v-else-if="!hasData" class="trend2__state">
      <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
    </div>
    <div v-else class="trend2__body">
      <ul class="trend2__legend">
        <li v-for="entry in mainSeries" :key="entry.key" class="trend2__chip">
          <span class="trend2__swatch" :style="{ background: entry.color }" />
          {{ entry.label }}
        </li>
      </ul>

      <div class="trend2__main">
        <DitherArea
          :series="mainSeries"
          :labels="labels"
          :rest-color="restColor"
          :marker-color="tokens.brand"
        />
      </div>

      <div class="trend2__rate">
        <div class="trend2__rate-head">
          <span>{{ t('usage.cacheHitRate') }}</span>
          <span class="trend2__rate-value">{{ latestRateLabel }}</span>
        </div>
        <div class="trend2__rate-chart">
          <DitherArea
            :series="rateSeries"
            :labels="labels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.trend2 {
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  padding: 16px;
}

.trend2__title {
  margin: 0 0 12px;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.trend2__state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 192px;
}

/* The bands carry no shape of their own once stacked, so the legend is the
   only thing naming them -- it replaces the line styles the old chart used to
   distinguish the three series. */
.trend2__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin: 0 0 10px;
  padding: 0;
  list-style: none;
}

.trend2__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.trend2__swatch {
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.trend2__main {
  /* Anatomy spec: the volume panel, before the 34px rate strip below it. */
  height: 150px;
}

.trend2__rate {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}

.trend2__rate-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.trend2__rate-value {
  color: var(--body-copy);
  font-variant-numeric: tabular-nums;
}

.trend2__rate-chart {
  /* The exact 34px the anatomy spec gives the rate strip. */
  height: 34px;
}
</style>
