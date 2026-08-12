<script setup lang="ts">
/**
 * Request duration histogram, as tile columns.
 *
 * Converted from a Chart.js bar chart painted #3b82f6 with a purple clock
 * glyph in its header -- two more hues that were not Inferno's. Single series,
 * so each column carries one band; DitherBars handles that shape without
 * change, which is the point of having built it as a stack in the first place.
 *
 * A histogram is the one chart here where the x axis is NOT time and NOT a
 * category you could reorder -- the buckets are an ordered range, and reading
 * where the mass sits along it is the whole job. So the columns stay in
 * bucket order and are never sorted by height.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsLatencyHistogramResponse } from '@/api/admin/ops'
import type { ChartState } from '../types'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherBars, { type BarColumn, type BarHover } from '@/components/charts/DitherBars.vue'
import { useChartTokens } from '@/composables/useChartTokens'

interface Props {
  latencyData: OpsLatencyHistogramResponse | null
  loading: boolean
}

const props = defineProps<Props>()
const { t } = useI18n()
const { tokens } = useChartTokens()
const hovered = ref<BarHover | null>(null)

const hasData = computed(() => (props.latencyData?.total_requests ?? 0) > 0)

const state = computed<ChartState>(() => {
  if (hasData.value) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

const columns = computed<BarColumn[]>(() =>
  (props.latencyData?.buckets ?? []).map((b) => ({
    key: b.range,
    label: b.range,
    bands: [
      {
        key: 'count',
        label: t('admin.ops.requests'),
        value: Number(b.count) || 0,
        color: tokens.brand
      }
    ]
  }))
)

const counts = computed(() => columns.value.map((c) => c.bands[0].value))

/* Same nice-rounding as the model bars: without it the gridline labels land on
   values like 137 and the axis stops being readable at a glance. */
const axisMax = computed(() => {
  const peak = Math.max(0, ...counts.value) * 1.05
  if (peak <= 0) return 1
  const magnitude = Math.pow(10, Math.floor(Math.log10(peak)))
  const norm = peak / magnitude
  const step = norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10
  return step * magnitude
})

function abbreviate(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}k`
  return String(Math.round(n))
}

const ticks = computed(() =>
  [1, 0.75, 0.5, 0.25, 0].map((f) => ({ f, label: abbreviate(axisMax.value * f) }))
)

const hoveredColumn = computed(() =>
  hovered.value ? columns.value[hovered.value.column] : null
)

/* The header reports whichever bucket is under the pointer, and the total
   otherwise -- so pointing at a column answers "how many landed here" without
   a tooltip that vanishes the moment you look away from it. */
const headline = computed(() => {
  if (hoveredColumn.value) return hoveredColumn.value.bands[0].value.toLocaleString()
  return (props.latencyData?.total_requests ?? 0).toLocaleString()
})

const subline = computed(() =>
  hoveredColumn.value
    ? t('admin.ops.latency.inBucket', { bucket: hoveredColumn.value.label })
    : t('admin.ops.latency.totalCaption')
)
</script>

<template>
  <div class="opslat">
    <div class="opslat__head">
      <h3 class="opslat__title">
        <i class="hgi-stroke hgi-timer-02 opslat__title-icon" aria-hidden="true" />
        {{ t('admin.ops.latencyHistogram') }}
        <HelpTooltip :content="t('admin.ops.tooltips.latencyHistogram')" />
      </h3>
    </div>

    <template v-if="state === 'ready' && columns.length">
      <p class="opslat__value">{{ headline }}</p>
      <p class="opslat__sub">{{ subline }}</p>

      <div class="opslat__chart">
        <div class="opslat__axis">
          <span v-for="tick in ticks" :key="tick.f" class="opslat__tick">{{ tick.label }}</span>
        </div>
        <div class="opslat__plot">
          <span
            v-for="tick in ticks"
            :key="tick.f"
            class="opslat__grid"
            :data-base="tick.f === 0 || undefined"
            :style="{ top: `${(1 - tick.f) * 100}%` }"
          />
          <DitherBars
            :columns="columns"
            :axis-max="axisMax"
            :hovered="hovered"
            :highlight="tokens.brand"
            @update:hovered="hovered = $event"
          />
        </div>
      </div>

      <div class="opslat__labels">
        <span
          v-for="(col, i) in columns"
          :key="col.key"
          class="opslat__label"
          :data-hot="hovered?.column === i || undefined"
          :title="col.label"
          >{{ col.label }}</span
        >
      </div>
    </template>

    <div v-else class="opslat__state">
      <LoadingSpinner v-if="state === 'loading'" size="md" />
      <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.charts.emptyRequest')" />
    </div>
  </div>
</template>

<style scoped>
.opslat {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.opslat__head {
  margin-bottom: 8px;
}

.opslat__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.opslat__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opslat__value {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.opslat__sub {
  margin: 2px 0 12px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opslat__chart {
  display: flex;
  flex: 1;
  gap: 8px;
  min-height: 140px;
}

.opslat__axis {
  display: flex;
  flex: none;
  flex-direction: column;
  justify-content: space-between;
  width: 30px;
  text-align: right;
}

.opslat__tick {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
  transform: translateY(-50%);
}

.opslat__plot {
  position: relative;
  flex: 1;
  min-width: 0;
  border-radius: 5px;
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.opslat__grid {
  position: absolute;
  left: 0;
  right: 0;
  height: 0;
  border-top: 1px dashed color-mix(in oklch, var(--foreground) 14%, transparent);
}

.opslat__grid[data-base] {
  border-top: 1.5px solid var(--border);
}

.opslat__labels {
  display: flex;
  margin-top: 8px;
  padding-left: 38px;
}

.opslat__label {
  flex: 1;
  min-width: 0;
  padding: 0 2px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-align: center;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color var(--motion-hover);
}

.opslat__label[data-hot] {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.opslat__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}
</style>
