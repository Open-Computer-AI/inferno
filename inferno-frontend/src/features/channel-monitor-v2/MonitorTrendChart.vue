<script setup lang="ts">
/**
 * Channel health over time: error rate, cache rate, TTFT p50.
 *
 * THREE OVERLAID LINES BECOME THREE STRIPS
 * Chart.js drew all three on one plot with percentages on a left axis and
 * milliseconds on a right one. They cannot share a scale and they cannot
 * stack: error rate and cache rate are independent rates, so "5% + 80% = 85%"
 * is not a quantity, and TTFT is a different unit entirely. Each series gets
 * its own strip and its own scale, the same call the ops throughput chart and
 * the payment revenue chart made.
 *
 * A second y-axis also implies a correlation the data does not contain -- where
 * the error line crosses the TTFT line depends only on how the two ranges
 * happened to be scaled.
 *
 * THE ZOOM IS UNCHANGED, BECAUSE IT NEVER TOUCHED CHART.JS
 * `onChartWheel` updates a ZoomState that `sliceByZoom` uses to window the data
 * array before it reaches any renderer. It is pure data windowing, so it works
 * against DitherArea exactly as it did against Chart.js. The wheel handler
 * moved to the element wrapping all three strips, since they share one time
 * axis and must zoom together.
 *
 * COLOURS
 * #ef4444 / #10b981 / #0ea5e9 became the categorical ramp, matching
 * OpsErrorTrendChart: a rate series is categorical data, not a state, so it
 * takes the ramp rather than the destructive tone. Colour-as-severity stays
 * reserved for chips and badges.
 */
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import DitherArea, { type AreaSeries } from '@/components/charts/DitherArea.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import type { MonitorCoverage, MonitorMetric, MonitorHealth } from '@/api/channelMonitorV2'
import { formatMonitorMs, formatMonitorPercent } from '@/features/channel-monitor-v2/monitorFormat'
import {
  applyWheelZoom,
  clientXRatio,
  isZoomed,
  resetZoom,
  sliceByZoom,
  type ZoomState,
} from '@/features/channel-monitor-v2/monitorZoom'

const { t, locale } = useI18n()
const { tokens } = useChartTokens()

const props = defineProps<{
  trend: Array<{ bucket_start: string; metrics: MonitorMetric; health: MonitorHealth }>
  coverage: MonitorCoverage | null
  loading?: boolean
}>()

const chartRef = ref<HTMLElement | null>(null)
const zoom = ref<ZoomState>(resetZoom())
const zoomed = computed(() => isZoomed(zoom.value))

const bucketLabel = computed(() => {
  const seconds = props.coverage?.bucket_seconds || 60
  const minutes = seconds / 60
  if (minutes < 60) return t('channelMonitorV2.bucket.minutes', { count: minutes })
  const hours = minutes / 60
  if (hours < 24) return t('channelMonitorV2.bucket.hours', { count: hours })
  return t('channelMonitorV2.bucket.days', { count: hours / 24 })
})

/** Window the series by zoom state around the cursor — not always the last N points. */
const visibleTrend = computed(() => sliceByZoom(props.trend || [], zoom.value))

function smoothTrend(values: Array<number | null>): Array<number | null> {
  if (values.length <= 2) return values
  return values.map((value, index) => {
    if (value == null) return null
    const neighbors = values.slice(Math.max(0, index - 1), Math.min(values.length, index + 2))
      .filter((item): item is number => item != null)
    if (!neighbors.length) return value
    return neighbors.reduce((sum, item) => sum + item, 0) / neighbors.length
  })
}

/*
 * DitherArea takes number[] and coerces anything else with `Number(v) || 0`, so
 * a null TTFT bucket would render as 0ms -- a claim that the channel answered
 * instantly, which is the opposite of "we have no sample". The Chart.js dataset
 * carried `spanGaps: true`, which bridges a gap rather than dropping to zero,
 * so this reproduces that: interior gaps interpolate between known neighbours,
 * and leading/trailing gaps hold the nearest known value.
 */
function bridgeGaps(values: Array<number | null>): number[] {
  const known = values.map((v, i) => (v == null ? -1 : i)).filter((i) => i >= 0)
  if (known.length === 0) return values.map(() => 0)

  return values.map((v, i) => {
    if (v != null) return v
    const before = known.filter((k) => k < i).pop()
    const after = known.find((k) => k > i)
    if (before == null) return values[after as number] as number
    if (after == null) return values[before] as number
    const lo = values[before] as number
    const hi = values[after] as number
    return lo + ((hi - lo) * (i - before)) / (after - before)
  })
}

const model = computed(() => {
  const points = visibleTrend.value
  if (!points.length) return null

  const labels = points.map((p) =>
    new Intl.DateTimeFormat(locale.value || undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    }).format(new Date(p.bucket_start))
  )

  const errorRates = bridgeGaps(smoothTrend(points.map((p) => (p.metrics.error_rate || 0) * 100)))
  const cacheRates = bridgeGaps(smoothTrend(points.map((p) => (p.metrics.cache_rate || 0) * 100)))
  const ttftP50 = bridgeGaps(smoothTrend(points.map((p) => p.metrics.ttft?.p50_ms ?? null)))

  const peak = (xs: number[]) => (xs.length ? Math.max(...xs) : 0)

  return {
    labels,
    strips: [
      {
        key: 'error',
        label: t('channelMonitorV2.chart.errorDataset'),
        color: tokens.ramp[0] ?? tokens.brand,
        peak: formatMonitorPercent(peak(errorRates) / 100),
        series: [
          { key: 'error', label: t('channelMonitorV2.chart.errorDataset'), values: errorRates, color: tokens.ramp[0] ?? tokens.brand }
        ] satisfies AreaSeries[],
      },
      {
        key: 'cache',
        label: t('channelMonitorV2.chart.cacheDataset'),
        color: tokens.ramp[1] ?? tokens.brand,
        peak: formatMonitorPercent(peak(cacheRates) / 100),
        series: [
          { key: 'cache', label: t('channelMonitorV2.chart.cacheDataset'), values: cacheRates, color: tokens.ramp[1] ?? tokens.brand }
        ] satisfies AreaSeries[],
      },
      {
        key: 'ttft',
        label: t('channelMonitorV2.chart.ttftDataset'),
        color: tokens.ramp[2] ?? tokens.brand,
        peak: formatMonitorMs(peak(ttftP50)),
        series: [
          { key: 'ttft', label: t('channelMonitorV2.chart.ttftDataset'), values: ttftP50, color: tokens.ramp[2] ?? tokens.brand }
        ] satisfies AreaSeries[],
      },
    ],
  }
})

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)

function onChartWheel(event: WheelEvent) {
  // Plain vertical wheel zooms X (narrower time range); shift/horizontal pans.
  event.preventDefault()
  const ratio = clientXRatio(event.clientX, chartRef.value)
  zoom.value = applyWheelZoom(zoom.value, event, ratio)
}

function resetChartZoom() {
  zoom.value = resetZoom()
}

watch(() => props.trend, () => {
  zoom.value = resetZoom()
})
</script>

<template>
  <section class="mtrend">
    <div class="mtrend__head">
      <div class="mtrend__heading">
        <h2 class="mtrend__title">
          <Icon name="chart" size="sm" class="mtrend__title-icon" aria-hidden="true" />
          {{ t('channelMonitorV2.chart.title') }}
        </h2>
        <p class="mtrend__desc">{{ t('channelMonitorV2.chart.description') }}</p>
      </div>

      <div class="mtrend__controls">
        <span class="mtrend__bucket">{{ bucketLabel }}</span>
        <button type="button" class="mtrend__reset" :disabled="!zoomed" @click="resetChartZoom">
          {{ t('channelMonitorV2.chart.resetZoom') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="mtrend__state">
      {{ t('common.loading') }}
    </div>

    <!-- One wheel target for all three strips: they share a time axis, so they
         have to zoom together or the comparison breaks. -->
    <div v-else-if="model" ref="chartRef" class="mtrend__plots" @wheel="onChartWheel">
      <div v-for="strip in model.strips" :key="strip.key" class="mtrend__strip">
        <div class="mtrend__strip-head">
          <span class="mtrend__legend">
            <span class="mtrend__swatch" :style="{ background: strip.color }" aria-hidden="true" />
            {{ strip.label }}
          </span>
          <span class="mtrend__peak">{{ strip.peak }} {{ t('admin.ops.charts.peak') }}</span>
        </div>
        <div class="mtrend__plot">
          <DitherArea
            :series="strip.series"
            :labels="model.labels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>
    </div>

    <div v-else class="mtrend__state">
      <EmptyState
        :title="t('channelMonitorV2.chart.emptyTitle')"
        :description="t('channelMonitorV2.empty.description')"
      />
    </div>
  </section>
</template>

<style scoped>
.mtrend {
  display: flex;
  flex-direction: column;
  min-height: 360px;
  padding: 16px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.mtrend__head {
  display: flex;
  flex-shrink: 0;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}

.mtrend__heading {
  min-width: 0;
}

.mtrend__title {
  display: flex;
  align-items: center;
  gap: 7px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.mtrend__title-icon {
  color: var(--muted-foreground);
}

.mtrend__desc {
  margin: 3px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.mtrend__controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.mtrend__bucket {
  padding: 2px 8px;
  border-radius: var(--r-pill);
  background: var(--surface-subtle);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  white-space: nowrap;
}

.mtrend__reset {
  padding: 4px 10px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.mtrend__reset:hover:not(:disabled) {
  background: var(--surface-hover);
  color: var(--foreground);
}

.mtrend__reset:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}

.mtrend__reset:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.mtrend__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  min-height: 280px;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.mtrend__plots {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
}

.mtrend__strip + .mtrend__strip {
  margin-top: 12px;
  padding-top: 12px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.mtrend__strip-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

/* The old top-of-card legend listed the same three names as coloured dots.
   With one strip each, the swatch belongs beside its own plot. */
.mtrend__legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.mtrend__swatch {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.mtrend__peak {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.mtrend__plot {
  position: relative;
  height: 96px;
  border-radius: 5px;
}
</style>
