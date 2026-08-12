<script setup lang="ts">
/**
 * Throughput, as two stacked strips rather than one dual-axis chart.
 *
 * WHY NOT ONE STACKED FIELD
 * QPS and TPS are different units. DitherArea stacks, and stacking is only
 * honest for series that ADD UP -- requests per second and tokens per second
 * do not sum to anything. The old chart handled that with a second y axis,
 * which is worse than it looks: two axes let a reader compare the HEIGHTS of
 * two lines that share no scale, and any apparent crossover between them is an
 * artefact of where the axes were placed.
 *
 * Two strips with a shared x axis says the true thing. Each has its own scale
 * because it must, and stacking them vertically makes that explicit rather
 * than hiding it behind a right-hand axis. It is the same treatment the token
 * trend uses for its cache-rate strip.
 *
 * WHAT WAS LOST, DELIBERATELY
 * Pinch and drag zoom went with Chart.js -- it came from chartjs-plugin-zoom
 * and there is nothing to reset without it. Owner call: the range selector
 * above the page already does that job, and zooming a 60-point one-hour window
 * was rarely the tool anyone reached for. Download survives, because a canvas
 * exports itself with toDataURL and does not need a plugin.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  OpsThroughputGroupBreakdownItem,
  OpsThroughputPlatformBreakdownItem,
  OpsThroughputTrendPoint
} from '@/api/admin/ops'
import type { ChartState } from '../types'
import { formatHistoryLabel, sumNumbers } from '../utils/opsFormatters'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherArea from '@/components/charts/DitherArea.vue'
import { formatNumber } from '@/utils/format'
import { useChartTokens } from '@/composables/useChartTokens'
import { useSequentialRamp } from '@/composables/useSequentialRamp'

interface Props {
  points: OpsThroughputTrendPoint[]
  loading: boolean
  timeRange: string
  byPlatform?: OpsThroughputPlatformBreakdownItem[]
  topGroups?: OpsThroughputGroupBreakdownItem[]
  fullscreen?: boolean
}

const props = defineProps<Props>()
const { t } = useI18n()
const emit = defineEmits<{
  (e: 'selectPlatform', platform: string): void
  (e: 'selectGroup', groupId: number): void
  (e: 'openDetails'): void
}>()

const { tokens } = useChartTokens()
const ramp = useSequentialRamp(2)
const root = ref<HTMLElement | null>(null)

const totalRequests = computed(() => sumNumbers(props.points.map((p) => p.request_count)))

const state = computed<ChartState>(() => {
  if (props.points.length && totalRequests.value > 0) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

const labels = computed(() =>
  props.points.map((p) => formatHistoryLabel(p.bucket_start, props.timeRange))
)

const qpsSeries = computed(() => [
  {
    key: 'qps',
    label: 'QPS',
    values: props.points.map((p) => p.qps ?? 0),
    color: ramp.value[0] ?? ''
  }
])

const tpsSeries = computed(() => [
  {
    key: 'tps',
    label: t('admin.ops.tpsK'),
    values: props.points.map((p) => (p.tps ?? 0) / 1000),
    color: ramp.value[1] ?? ''
  }
])

const peakQps = computed(() => Math.max(0, ...props.points.map((p) => p.qps ?? 0)))
const peakTps = computed(() => Math.max(0, ...props.points.map((p) => (p.tps ?? 0) / 1000)))

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)

/* Both strips onto one image, stacked as they appear, so the export matches
   what is on screen rather than being two unrelated files. */
function downloadChart() {
  const canvases = Array.from(root.value?.querySelectorAll('canvas') ?? [])
  if (!canvases.length) return
  const width = Math.max(...canvases.map((c) => c.width))
  const height = canvases.reduce((sum, c) => sum + c.height, 0)
  const out = document.createElement('canvas')
  out.width = width
  out.height = height
  const ctx = out.getContext('2d')
  if (!ctx) return
  ctx.fillStyle = tokens.card
  ctx.fillRect(0, 0, width, height)
  let y = 0
  for (const c of canvases) {
    ctx.drawImage(c, 0, y)
    y += c.height
  }
  const link = document.createElement('a')
  link.download = 'throughput.png'
  link.href = out.toDataURL('image/png')
  link.click()
}
</script>

<template>
  <div ref="root" class="opsthru">
    <div class="opsthru__head">
      <h3 class="opsthru__title">
        <i class="hgi-stroke hgi-activity-01 opsthru__title-icon" aria-hidden="true" />
        {{ t('admin.ops.throughputTrend') }}
        <HelpTooltip v-if="!props.fullscreen" :content="t('admin.ops.tooltips.throughputTrend')" />
      </h3>
      <div data-testid="throughput-chart-toolbar" class="opsthru__actions">
        <template v-if="!props.fullscreen">
          <button
            type="button"
            class="opsthru__action"
            :disabled="state !== 'ready'"
            :title="t('admin.ops.requestDetails.title')"
            @click="emit('openDetails')"
          >
            {{ t('admin.ops.requestDetails.details') }}
          </button>
          <button
            type="button"
            class="opsthru__action"
            :disabled="state !== 'ready'"
            :title="t('admin.ops.charts.downloadChartHint')"
            @click="downloadChart"
          >
            {{ t('admin.ops.charts.downloadChart') }}
          </button>
        </template>
      </div>
    </div>

    <!-- Drilldown chips (baseline interaction: click to set global filter) -->
    <div v-if="(props.topGroups?.length ?? 0) > 0" class="opsthru__chips">
      <button
        v-for="g in props.topGroups"
        :key="g.group_id"
        type="button"
        class="opsthru__chip"
        @click="emit('selectGroup', g.group_id)"
      >
        <span class="opsthru__chip-name">{{ g.group_name || `#${g.group_id}` }}</span>
        <span class="opsthru__chip-count">{{ formatNumber(g.request_count) }}</span>
      </button>
    </div>

    <div v-else-if="(props.byPlatform?.length ?? 0) > 0" class="opsthru__chips">
      <button
        v-for="p in props.byPlatform"
        :key="p.platform"
        type="button"
        class="opsthru__chip"
        @click="emit('selectPlatform', p.platform)"
      >
        <span class="opsthru__chip-name opsthru__chip-name--upper">{{ p.platform }}</span>
        <span class="opsthru__chip-count">{{ formatNumber(p.request_count) }}</span>
      </button>
    </div>

    <template v-if="state === 'ready'">
      <div class="opsthru__strip">
        <div class="opsthru__strip-head">
          <span><span class="opsthru__swatch" :style="{ background: ramp[0] }" />QPS</span>
          <span class="opsthru__peak">{{ peakQps.toFixed(2) }} {{ t('admin.ops.charts.peak') }}</span>
        </div>
        <div class="opsthru__plot">
          <DitherArea
            :series="qpsSeries"
            :labels="labels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>

      <div class="opsthru__strip">
        <div class="opsthru__strip-head">
          <span><span class="opsthru__swatch" :style="{ background: ramp[1] }" />{{ t('admin.ops.tpsK') }}</span>
          <span class="opsthru__peak">{{ peakTps.toFixed(1) }} {{ t('admin.ops.charts.peak') }}</span>
        </div>
        <div class="opsthru__plot">
          <DitherArea
            :series="tpsSeries"
            :labels="labels"
            :rest-color="restColor"
            :marker-color="tokens.brand"
          />
        </div>
      </div>
    </template>

    <div v-else class="opsthru__state">
      <LoadingSpinner v-if="state === 'loading'" size="md" />
      <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.charts.emptyRequest')" />
    </div>
  </div>
</template>

<style scoped>
.opsthru {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.opsthru__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.opsthru__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.opsthru__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opsthru__actions {
  display: flex;
  flex: none;
  gap: 6px;
}

.opsthru__action {
  padding: 3px 9px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opsthru__action:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.opsthru__action:disabled {
  opacity: 0.5;
  cursor: default;
}

.opsthru__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
}

.opsthru__chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 3px 10px;
  border: 1px solid var(--border);
  border-radius: var(--r-pill);
  background: var(--card);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opsthru__chip:hover {
  background: var(--sidebar-accent);
}

.opsthru__chip-name {
  max-width: 180px;
  color: var(--foreground);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Was uppercase, which ground rule 1 bans. Platform values arrive lowercase
   ("anthropic"), so capitalising reads as a name without shouting. */
.opsthru__chip-name--upper {
  text-transform: capitalize;
}

.opsthru__chip-count {
  color: var(--muted-foreground);
  font-variant-numeric: tabular-nums;
}

/* Two strips, each with its own scale, stacked so the shared x axis is
   obvious and the different y scales are not pretending to be one. */
.opsthru__strip {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 84px;
}

.opsthru__strip + .opsthru__strip {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border-subtle);
}

.opsthru__strip-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.opsthru__swatch {
  display: inline-block;
  width: 9px;
  height: 9px;
  margin-right: 6px;
  border-radius: 3px;
  vertical-align: baseline;
}

.opsthru__peak {
  color: var(--body-copy);
  font-variant-numeric: tabular-nums;
}

.opsthru__plot {
  position: relative;
  flex: 1;
  min-height: 60px;
  border-radius: 5px;
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.opsthru__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}
</style>
