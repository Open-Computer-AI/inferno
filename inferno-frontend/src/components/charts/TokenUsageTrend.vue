<script setup lang="ts">
/**
 * TokenUsageTrend -- part 09, section 05.
 *
 * Migration note from the prototype:
 *   "Five series on one canvas, a percentage and four volumes on the same
 *    chart, means lines cross for reasons that have no meaning. Three volume
 *    series and the rate pulled into its own strip below, sharing the x
 *    axis, no right hand axis. Cache creation and cache read become one
 *    cache series -- the question is how much traffic is cached, not which
 *    half of the mechanism it went through. Both stay in the tooltip."
 *
 * Two Chart.js Line instances instead of one dual-axis chart: `mainChart`
 * (Input, Output, Cache -- Input alone carries the area fill) and `rateChart`
 * (Cache hit rate, its own 0-100 scale, 34px per the anatomy spec). They
 * share x labels and are kept in sync on hover -- see `syncHover` below --
 * rather than merged back into one canvas, so neither series has to fight
 * the other's scale again.
 *
 * All five June colors below (--sr-1 input, --sr-3 output/rate, --sr-8
 * dashed cache, --brand-tint fill) come through `useChartTokens()`: Chart.js
 * draws to a <canvas>, which cannot read a CSS custom property, so every one
 * of them is resolved to a literal and re-resolved on a theme or brand
 * change. See src/composables/useChartTokens.ts for the mechanism.
 *
 * "The dark mode text and grid colours come from tokens, so the isDarkMode
 * computed can go" -- it does; nothing here reads document.documentElement
 * directly any more.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type { ChartComponentRef } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const { t } = useI18n()

const props = defineProps<{
  trendData: TrendDataPoint[]
  loading?: boolean
}>()

const { tokens, prefersReducedMotion } = useChartTokens()

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

// Same formula as before the rewrite: cache reads over every prompt token
// (input + both halves of cache), so Anthropic-style cache creation counts
// toward the denominator too.
const rateFor = (d: TrendDataPoint): number => {
  const totalPromptTokens = d.input_tokens + d.cache_read_tokens + d.cache_creation_tokens
  return totalPromptTokens > 0 ? (d.cache_read_tokens / totalPromptTokens) * 100 : 0
}

const cacheLabel = computed(() => t('admin.dashboard.cache'))

const mainChartData = computed(() => {
  if (!props.trendData?.length) return null
  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: t('admin.dashboard.input'),
        data: props.trendData.map((d) => d.input_tokens),
        borderColor: tokens.ramp[0],
        backgroundColor: tokens.brandTint,
        borderWidth: 1.75,
        fill: true,
        tension: 0.3,
        pointRadius: 0,
        pointHoverRadius: 3
      },
      {
        label: t('admin.dashboard.output'),
        data: props.trendData.map((d) => d.output_tokens),
        borderColor: tokens.ramp[2],
        backgroundColor: 'transparent',
        borderWidth: 1.75,
        fill: false,
        tension: 0.3,
        pointRadius: 0,
        pointHoverRadius: 3
      },
      {
        label: cacheLabel.value,
        // Cache creation and cache read merged into one series (migration
        // note above); both numbers still surface in the tooltip callback.
        data: props.trendData.map((d) => d.cache_creation_tokens + d.cache_read_tokens),
        borderColor: tokens.ramp[7],
        backgroundColor: 'transparent',
        borderWidth: 1.5,
        borderDash: [5, 5],
        fill: false,
        tension: 0.3,
        pointRadius: 0,
        pointHoverRadius: 3
      }
    ]
  }
})

const rateChartData = computed(() => {
  if (!props.trendData?.length) return null
  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: t('dashboard.cacheHitRate'),
        data: props.trendData.map(rateFor),
        borderColor: tokens.ramp[2],
        backgroundColor: 'transparent',
        borderWidth: 1.75,
        fill: false,
        tension: 0.3,
        pointRadius: 0,
        pointHoverRadius: 3
      }
    ]
  }
})

const latestRateLabel = computed(() => {
  if (!props.trendData?.length) return ''
  return `${rateFor(props.trendData[props.trendData.length - 1]).toFixed(1)}%`
})

const mainChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: prefersReducedMotion.value ? (false as const) : undefined,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: tokens.bodyCopy,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 14,
        font: { size: 11 }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          if (context.dataset.label === cacheLabel.value) {
            const point = props.trendData[context.dataIndex]
            const breakdown = t('admin.dashboard.cacheBreakdownTooltip', {
              total: formatTokens(context.raw),
              created: formatTokens(point?.cache_creation_tokens ?? 0),
              read: formatTokens(point?.cache_read_tokens ?? 0)
            })
            return `${context.dataset.label}: ${breakdown}`
          }
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
        },
        footer: (tooltipItems: any) => {
          const dataIndex = tooltipItems[0]?.dataIndex
          const data = dataIndex !== undefined ? props.trendData[dataIndex] : undefined
          if (!data) return ''
          return t('admin.dashboard.trendTooltipFooter', {
            actual: formatCost(data.actual_cost),
            standard: formatCost(data.cost)
          })
        }
      }
    }
  },
  scales: {
    x: {
      grid: { color: tokens.borderSubtle },
      ticks: { color: tokens.mutedForeground, maxTicksLimit: 6, font: { size: 10 } }
    },
    y: {
      beginAtZero: true,
      grid: { color: tokens.borderSubtle },
      ticks: {
        color: tokens.mutedForeground,
        maxTicksLimit: 5,
        font: { size: 10 },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    }
  }
}))

const rateChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: prefersReducedMotion.value ? (false as const) : undefined,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.dataset.label}: ${Number(context.raw).toFixed(1)}%`
      }
    }
  },
  // No right hand axis (the defect the "Today" panel illustrates): this
  // strip has its own 0-100 scale, not a second axis sharing the main chart.
  scales: {
    x: { display: false },
    y: { display: false, min: 0, max: 100 }
  }
}))

// --- one shared crosshair across both canvases ----------------------------
//
// Two separate Chart.js instances, so Chart.js's own `interaction.mode:
// 'index'` only ever syncs a chart with itself. This mirrors the active
// index (and its tooltip) onto both charts on every pointer move over
// either one, so hovering the volume chart also lights up the rate strip
// and vice versa -- "same hover, one shared crosshair" per the anatomy spec.
// No custom Chart.js plugin: just the two public instances Chart.js already
// exposes, kept in step.
const mainChartRef = ref<ChartComponentRef<'line'> | null>(null)
const rateChartRef = ref<ChartComponentRef<'line'> | null>(null)

const liveCharts = () =>
  [mainChartRef.value?.chart, rateChartRef.value?.chart].filter(
    (c): c is NonNullable<typeof c> => !!c
  )

const syncHover = (event: MouseEvent) => {
  const charts = liveCharts()
  const [primary] = charts
  if (!primary) return
  const elements = primary.getElementsAtEventForMode(event, 'index', { intersect: false }, false)
  if (!elements.length) return
  const index = elements[0].index
  for (const chart of charts) {
    const active = chart.data.datasets.map((_, datasetIndex) => ({ datasetIndex, index }))
    chart.setActiveElements(active)
    chart.tooltip?.setActiveElements(active, { x: 0, y: 0 })
    chart.update('none')
  }
}

const clearHover = () => {
  for (const chart of liveCharts()) {
    chart.setActiveElements([])
    chart.tooltip?.setActiveElements([], { x: 0, y: 0 })
    chart.update('none')
  }
}
</script>

<template>
  <div class="trend2">
    <h3 class="trend2__title">{{ t('admin.dashboard.tokenUsageTrend') }}</h3>

    <div v-if="loading" class="trend2__state">
      <LoadingSpinner />
    </div>
    <div v-else-if="!trendData.length || !mainChartData || !rateChartData" class="trend2__state">
      <EmptyState :title="t('admin.dashboard.noDataAvailable')" icon="hgi-chart-bar-big" />
    </div>
    <div v-else class="trend2__body" @mousemove="syncHover" @mouseleave="clearHover">
      <div class="trend2__main">
        <Line ref="mainChartRef" :data="mainChartData" :options="mainChartOptions" />
      </div>
      <div class="trend2__rate">
        <div class="trend2__rate-head">
          <span>{{ t('dashboard.cacheHitRate') }}</span>
          <span class="trend2__rate-value">{{ latestRateLabel }}</span>
        </div>
        <div class="trend2__rate-chart">
          <Line ref="rateChartRef" :data="rateChartData" :options="rateChartOptions" />
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
