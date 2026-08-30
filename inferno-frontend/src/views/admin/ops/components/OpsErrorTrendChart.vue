<script setup lang="ts">
/**
 * Error trend, as a stacked tile field.
 *
 * Converted from three overlaid Chart.js lines painted red, purple and grey.
 * They are STACKABLE -- SLA errors, upstream errors and business limits are
 * disjoint counts of requests in the same buckets -- so stacking them says
 * something the overlay did not: the envelope is every failed or refused
 * request in that minute, and each band is its share.
 *
 * Ordered by how much they demand attention, floor upward: SLA errors are what
 * the operator is held to, so they sit on the baseline where the eye lands
 * first and where a band's height is easiest to judge. Business limits go on
 * top because they are the least alarming -- the gateway refusing a
 * quota-exceeded request is it working, not failing.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsErrorTrendPoint } from '@/api/admin/ops'
import type { ChartState } from '../types'
import { formatHistoryLabel } from '../utils/opsFormatters'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherArea from '@/components/charts/DitherArea.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import { useSequentialRamp } from '@/composables/useSequentialRamp'

interface Props {
  points: OpsErrorTrendPoint[]
  loading: boolean
  timeRange: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'openRequestErrors'): void
  (e: 'openUpstreamErrors'): void
}>()

const { t } = useI18n()
const { tokens } = useChartTokens()
const ramp = useSequentialRamp(3)

const totalRequestErrors = computed(() =>
  props.points.reduce((sum, p) => sum + (p.error_count_sla ?? 0), 0)
)
// Two totals on purpose. The plotted series excludes 429/529, but they are
// still real upstream errors -- gating the drill-down on the plotted total
// alone hides it during an outage that is entirely rate-limit/overload.
const totalUpstreamErrorsPlotted = computed(() =>
  props.points.reduce((sum, p) => sum + (p.upstream_error_count_excl_429_529 ?? 0), 0)
)
const totalUpstreamErrors = computed(() =>
  props.points.reduce(
    (sum, p) =>
      sum + (p.upstream_error_count_excl_429_529 ?? 0) + (p.upstream_429_count ?? 0) + (p.upstream_529_count ?? 0),
    0
  )
)
const totalLimited = computed(() =>
  props.points.reduce((sum, p) => sum + (p.business_limited_count ?? 0), 0)
)

const hasRequestErrors = computed(() => totalRequestErrors.value > 0)
const hasUpstreamErrors = computed(() => totalUpstreamErrors.value > 0)

const totalDisplayed = computed(
  () => totalRequestErrors.value + totalUpstreamErrorsPlotted.value + totalLimited.value
)

const labels = computed(() =>
  props.points.map((p) => formatHistoryLabel(p.bucket_start, props.timeRange))
)

const series = computed(() => [
  {
    key: 'sla',
    label: t('admin.ops.errorsSla'),
    values: props.points.map((p) => p.error_count_sla ?? 0),
    color: ramp.value[0] ?? ''
  },
  {
    key: 'upstream',
    label: t('admin.ops.upstreamExcl429529'),
    values: props.points.map((p) => p.upstream_error_count_excl_429_529 ?? 0),
    color: ramp.value[1] ?? ''
  },
  {
    key: 'limited',
    label: t('admin.ops.businessLimited'),
    values: props.points.map((p) => p.business_limited_count ?? 0),
    color: ramp.value[2] ?? ''
  }
])

const state = computed<ChartState>(() => {
  if (props.points.length && totalDisplayed.value > 0) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)
</script>

<template>
  <div class="opstrend">
    <div class="opstrend__head">
      <h3 class="opstrend__title">
        <i class="hgi-stroke hgi-alert-02 opstrend__title-icon" aria-hidden="true" />
        {{ t('admin.ops.errorTrend') }}
        <HelpTooltip :content="t('admin.ops.tooltips.errorTrend')" />
      </h3>
      <div class="opstrend__actions">
        <button
          type="button"
          class="opstrend__action"
          :disabled="!hasRequestErrors"
          @click="emit('openRequestErrors')"
        >
          {{ t('admin.ops.errorDetails.requestErrors') }}
        </button>
        <button
          type="button"
          class="opstrend__action"
          :disabled="!hasUpstreamErrors"
          @click="emit('openUpstreamErrors')"
        >
          {{ t('admin.ops.errorDetails.upstreamErrors') }}
        </button>
      </div>
    </div>

    <template v-if="state === 'ready'">
      <ul class="opstrend__legend">
        <li v-for="entry in series" :key="entry.key" class="opstrend__chip">
          <span class="opstrend__swatch" :style="{ background: entry.color }" />
          {{ entry.label }}
        </li>
      </ul>
      <div class="opstrend__plot">
        <DitherArea
          :series="series"
          :labels="labels"
          :rest-color="restColor"
          :marker-color="tokens.brand"
        />
      </div>
    </template>

    <div v-else class="opstrend__state">
      <LoadingSpinner v-if="state === 'loading'" size="md" />
      <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.charts.emptyError')" />
    </div>
  </div>
</template>

<style scoped>
.opstrend {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.opstrend__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.opstrend__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

/* Muted, not rose: errors trending is what this panel is FOR, so a coloured
   glyph would be permanently alarmed (ground rule 5). */
.opstrend__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opstrend__actions {
  display: flex;
  flex: none;
  gap: 6px;
}

.opstrend__action {
  padding: 3px 9px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opstrend__action:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.opstrend__action:disabled {
  opacity: 0.5;
  cursor: default;
}

.opstrend__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin: 0 0 10px;
  padding: 0;
  list-style: none;
}

.opstrend__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.opstrend__swatch {
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.opstrend__plot {
  position: relative;
  flex: 1;
  min-height: 150px;
  border-radius: 5px;
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.opstrend__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}
</style>
