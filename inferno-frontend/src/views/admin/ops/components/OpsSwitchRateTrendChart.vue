<script setup lang="ts">
/**
 * Average account switches per request, as a tile field.
 *
 * Was a single teal Chart.js line (#14b8a6) under a teal glyph -- the last of
 * the page's borrowed palette. Single series, so DitherArea takes it as a
 * one-entry stack.
 *
 * The headline is the WINDOW's rate rather than the latest bucket's. A
 * per-minute switch rate is noisy enough that the last point is often an
 * outlier, and the number people act on is "how much are we churning accounts
 * right now", which is the aggregate.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsThroughputTrendPoint } from '@/api/admin/ops'
import type { ChartState } from '../types'
import { formatHistoryLabel, sumNumbers } from '../utils/opsFormatters'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherArea from '@/components/charts/DitherArea.vue'
import { useChartTokens } from '@/composables/useChartTokens'

interface Props {
  points: OpsThroughputTrendPoint[]
  loading: boolean
  timeRange: string
  fullscreen?: boolean
}

const props = defineProps<Props>()
const { t } = useI18n()
const { tokens } = useChartTokens()

const totalRequests = computed(() => sumNumbers(props.points.map((p) => p.request_count)))
const totalSwitches = computed(() => sumNumbers(props.points.map((p) => p.switch_count)))

const state = computed<ChartState>(() => {
  if (props.points.length && totalRequests.value > 0) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

const labels = computed(() =>
  props.points.map((p) => formatHistoryLabel(p.bucket_start, props.timeRange))
)

const series = computed(() => [
  {
    key: 'switchRate',
    label: t('admin.ops.switchRate'),
    values: props.points.map((p) => {
      const requests = p.request_count ?? 0
      const switches = p.switch_count ?? 0
      return requests > 0 ? switches / requests : 0
    }),
    color: tokens.brand
  }
])

const windowRate = computed(() =>
  totalRequests.value > 0 ? totalSwitches.value / totalRequests.value : 0
)

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)
</script>

<template>
  <div class="opsswitch">
    <div class="opsswitch__head">
      <h3 class="opsswitch__title">
        <i class="hgi-stroke hgi-exchange-01 opsswitch__title-icon" aria-hidden="true" />
        {{ t('admin.ops.switchRateTrend') }}
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.switchRateTrend')" />
      </h3>
    </div>

    <template v-if="state === 'ready'">
      <p class="opsswitch__value">{{ windowRate.toFixed(2) }}</p>
      <p class="opsswitch__sub">{{ t('admin.ops.switchRateCaption') }}</p>
      <div class="opsswitch__plot">
        <DitherArea
          :series="series"
          :labels="labels"
          :rest-color="restColor"
          :marker-color="tokens.brand"
        />
      </div>
    </template>

    <div v-else class="opsswitch__state">
      <LoadingSpinner v-if="state === 'loading'" size="md" />
      <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.charts.emptyRequest')" />
    </div>
  </div>
</template>

<style scoped>
.opsswitch {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.opsswitch__head {
  margin-bottom: 8px;
}

.opsswitch__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.opsswitch__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opsswitch__value {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.opsswitch__sub {
  margin: 2px 0 12px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opsswitch__plot {
  position: relative;
  flex: 1;
  min-height: 120px;
  border-radius: 5px;
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.opsswitch__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}
</style>
