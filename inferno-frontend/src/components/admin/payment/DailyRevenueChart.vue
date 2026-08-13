<script setup lang="ts">
/**
 * Revenue and order volume per day.
 *
 * WHY THIS DOES NOT STACK
 * Chart.js drew every currency plus the order count as overlaid `<Line>`
 * datasets, the count pinned to a second y-axis. Neither group can be stacked
 * honestly: USD + CNY + INR is not a number, and an order count shares no unit
 * with an amount at all. So this is strips, the same call the ops throughput
 * chart made for QPS and TPS, and the opposite of the one the user trend chart
 * made for per-user tokens, which genuinely do sum to a daily total.
 *
 * Each strip carries its own scale, which is what the dual y-axis was
 * approximating. Two axes on one plot is a well known way to imply a
 * correlation that the data does not contain -- the crossing point moves
 * wherever the two ranges happen to put it.
 *
 * COLOURS CAME FROM A FOUR ENTRY HEX RAINBOW
 * rgb(59,130,246) blue, rgb(168,85,247) purple, rgb(245,158,11) amber,
 * rgb(239,68,68) red, plus rgb(16,185,129) green for the count, on a page where
 * nothing else is any of those. Currencies take the categorical ramp now and
 * the count takes the brand hue.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DitherArea, { type AreaSeries } from '@/components/charts/DitherArea.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import { formatNumber } from '@/utils/format'
import type { DailyPaymentStats } from '@/types/payment'

const { t } = useI18n()
const { tokens } = useChartTokens()

const props = defineProps<{
  data: DailyPaymentStats[]
  loading?: boolean
}>()

const model = computed(() => {
  const rows = props.data ?? []
  if (rows.length === 0) return null

  const labels = rows.map((d) => d.date)
  const currencies = [...new Set(rows.flatMap((day) => Object.keys(day.amount ?? {})))].sort()

  const revenue = currencies.map((currency, i) => ({
    currency,
    total: rows.reduce((sum, day) => sum + (day.amount?.[currency] ?? 0), 0),
    color: tokens.ramp[i % tokens.ramp.length] ?? tokens.brand,
    series: [
      {
        key: currency,
        label: currency,
        values: rows.map((day) => day.amount?.[currency] ?? 0),
        color: tokens.ramp[i % tokens.ramp.length] ?? tokens.brand
      }
    ] satisfies AreaSeries[]
  }))

  const countSeries: AreaSeries[] = [
    {
      key: 'count',
      label: t('payment.admin.orderCount'),
      values: rows.map((d) => d.count ?? 0),
      color: tokens.brand
    }
  ]

  return {
    labels,
    revenue,
    countSeries,
    orderTotal: rows.reduce((sum, d) => sum + (d.count ?? 0), 0)
  }
})

/* Dates arrive as YYYY-MM-DD. Sliced, never through `new Date`, which would
   reinterpret a plain date in the browser's timezone and can shift the label a
   day either side of UTC. */
const axisLabels = computed(() => (model.value?.labels ?? []).map((d) => d.slice(5)))

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)

function formatAmount(v: number): string {
  return v.toLocaleString(undefined, { maximumFractionDigits: 2 })
}
</script>

<template>
  <section class="rev">
    <h3 class="rev__title">{{ t('payment.admin.dailyRevenue') }}</h3>

    <div v-if="loading" class="rev__state">
      <LoadingSpinner size="md" />
    </div>

    <p v-else-if="!model" class="rev__state rev__state--empty">
      {{ t('payment.admin.noData') }}
    </p>

    <template v-else>
      <!-- One strip per currency: amounts in different currencies do not sum,
           so a stack or a shared axis would be inventing a total. -->
      <div v-for="r in model.revenue" :key="r.currency" class="rev__strip">
        <div class="rev__strip-head">
          <span class="rev__legend">
            <span class="rev__swatch" :style="{ background: r.color }" aria-hidden="true" />
            {{ r.currency }} {{ t('payment.admin.revenue') }}
          </span>
          <span class="rev__total">{{ formatAmount(r.total) }}</span>
        </div>
        <div class="rev__plot">
          <DitherArea
            :series="r.series"
            :labels="axisLabels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>

      <!-- Order count is a different unit again, so it gets its own scale
           rather than the second y-axis it used to share with revenue. -->
      <div class="rev__strip">
        <div class="rev__strip-head">
          <span class="rev__legend">
            <span class="rev__swatch" :style="{ background: tokens.brand }" aria-hidden="true" />
            {{ t('payment.admin.orderCount') }}
          </span>
          <span class="rev__total">{{ formatNumber(model.orderTotal) }}</span>
        </div>
        <div class="rev__plot">
          <DitherArea
            :series="model.countSeries"
            :labels="axisLabels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.rev {
  display: flex;
  flex-direction: column;
  padding: 16px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.rev__title {
  margin: 0 0 12px;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.rev__state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 200px;
  margin: 0;
}

.rev__state--empty {
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.rev__strip + .rev__strip {
  margin-top: 14px;
  padding-top: 14px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.rev__strip-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 8px;
}

.rev__legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.rev__swatch {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

/* Serif at display size, like every other headline figure in the product. */
.rev__total {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-xl);
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
}

.rev__plot {
  position: relative;
  height: 148px;
  border-radius: 5px;
}
</style>
