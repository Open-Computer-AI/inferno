<script setup lang="ts">
/**
 * "Payments by branch", forked onto our data: tokens by model, stacked by
 * type.
 *
 * WHY THIS DATA
 * The form needs categories on the x-axis and a small number of additive
 * series inside each. ModelStat already carries input, output, cache creation
 * and cache read per model, and those four sum to that model's total by
 * definition -- so the stack is real arithmetic rather than three numbers
 * placed on top of each other. Cache write and cache read are folded into one
 * Cache band to land on the reference's three, and because the pair is one
 * idea at this altitude.
 *
 * The chart's own finding is that cache dwarfs everything, which is the same
 * story the fold's tiles tell. That is not a flaw in the choice of data; it is
 * the shape of this workload, and a chart that hides it would be worse.
 *
 * PREFETCH RATHER THAN FOUR CALLS ON MOUNT
 * Model stats are aggregated over a range, not bucketed, so unlike the donut
 * they cannot be sliced client side -- each period is genuinely a different
 * query. Firing all four on mount would put three unused requests in the
 * dashboard's critical path; fetching on click would put a round trip between
 * the click and the morph. So: the active period loads on mount, the other
 * three are fetched quietly once it lands, and every switch after that is
 * instant against the cache.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ModelStat } from '@/types'
import DitherBars, { type BarColumn, type BarHover } from '@/components/charts/DitherBars.vue'
import NumberTicker from '@/components/common/NumberTicker.vue'
import Segmented from '@/components/common/Segmented.vue'
import { useSequentialRamp } from '@/composables/useSequentialRamp'
import { useChartTokens } from '@/composables/useChartTokens'

const { t } = useI18n()
const ramp = useSequentialRamp(3)
const { tokens } = useChartTokens()

type PeriodKey = 'week' | 'month' | 'quarter' | 'year'
const PERIOD_DAYS: Record<PeriodKey, number> = { week: 7, month: 30, quarter: 90, year: 365 }
const MAX_COLUMNS = 4

const period = ref<PeriodKey>('month')
const hovered = ref<BarHover | null>(null)
const cache = ref<Partial<Record<PeriodKey, ModelStat[]>>>({})
const loading = ref(true)

const periodItems = computed(() =>
  (Object.keys(PERIOD_DAYS) as PeriodKey[]).map((key) => ({
    value: key,
    label: t(`admin.dashboard.period.${key}`)
  }))
)

function formatLocalDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(
    date.getDate()
  ).padStart(2, '0')}`
}

async function fetchPeriod(key: PeriodKey): Promise<void> {
  if (cache.value[key]) return
  const now = new Date()
  const response = await adminAPI.dashboard.getSnapshotV2({
    start_date: formatLocalDate(new Date(now.getTime() - (PERIOD_DAYS[key] - 1) * 86400000)),
    end_date: formatLocalDate(now),
    granularity: 'day',
    include_stats: false,
    include_trend: false,
    include_model_stats: true,
    include_group_stats: false,
    include_users_trend: false
  })
  cache.value = { ...cache.value, [key]: response.models || [] }
}

const models = computed(() => cache.value[period.value] ?? [])

const BAND_KEYS = ['cache', 'input', 'output'] as const

/** Top models by total tokens. Anything past the cap is dropped rather than
 *  rolled into an "other" column: an aggregate bar next to real ones invites
 *  comparing it to them, and it is not a peer. */
const columns = computed<BarColumn[]>(() =>
  [...models.value]
    .map((m) => ({
      model: m,
      total:
        (Number(m.input_tokens) || 0) +
        (Number(m.output_tokens) || 0) +
        (Number(m.cache_creation_tokens) || 0) +
        (Number(m.cache_read_tokens) || 0)
    }))
    .filter((row) => row.total > 0)
    .sort((a, b) => b.total - a.total)
    .slice(0, MAX_COLUMNS)
    .map(({ model }) => ({
      key: model.model,
      label: model.model,
      bands: [
        {
          key: 'cache',
          label: t('admin.dashboard.cache'),
          value:
            (Number(model.cache_creation_tokens) || 0) + (Number(model.cache_read_tokens) || 0),
          color: ramp.value[0] ?? ''
        },
        {
          key: 'input',
          label: t('admin.dashboard.input'),
          value: Number(model.input_tokens) || 0,
          color: ramp.value[1] ?? ''
        },
        {
          key: 'output',
          label: t('admin.dashboard.output'),
          value: Number(model.output_tokens) || 0,
          color: ramp.value[2] ?? ''
        }
      ]
    }))
)

const columnTotals = computed(() =>
  columns.value.map((c) => c.bands.reduce((sum, b) => sum + b.value, 0))
)

const grandTotal = computed(() => columnTotals.value.reduce((a, b) => a + b, 0))

/**
 * A "nice" axis maximum: 5% headroom, then rounded UP to 1, 2, 5 or 10 times a
 * power of ten. Without the rounding the gridline labels land on values like
 * 137,412 and the axis stops being readable at a glance; without the headroom
 * the tallest bar touches the top line and reads as clipped.
 */
const axisMax = computed(() => {
  const peak = Math.max(0, ...columnTotals.value) * 1.05
  if (peak <= 0) return 1
  const magnitude = Math.pow(10, Math.floor(Math.log10(peak)))
  const norm = peak / magnitude
  const step = norm <= 1 ? 1 : norm <= 2 ? 2 : norm <= 5 ? 5 : 10
  return step * magnitude
})

function abbreviate(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}k`
  return String(Math.round(n))
}

/* Top to bottom, so the array order matches the visual order of the rows. */
const ticks = computed(() =>
  [1, 0.75, 0.5, 0.25, 0].map((f) => ({ f, label: abbreviate(axisMax.value * f) }))
)

/* Labels come from i18n directly, not from column 0's bands: with no models
   the fallback printed the raw keys ("cache", "input") into the legend. The
   legend describes the SERIES, which exist whether or not any data does. */
const BAND_LABEL_KEYS: Record<(typeof BAND_KEYS)[number], string> = {
  cache: 'admin.dashboard.cache',
  input: 'admin.dashboard.input',
  output: 'admin.dashboard.output'
}

const legend = computed(() =>
  BAND_KEYS.map((key, i) => ({
    key,
    label: t(BAND_LABEL_KEYS[key]),
    color: ramp.value[i] ?? ''
  }))
)

/* The badge follows the hover, and falls back to the reading this dashboard
   cares about most rather than to a delta we would need another query for. */
const cacheShare = computed(() => {
  if (grandTotal.value <= 0) return 0
  const cacheTotal = columns.value.reduce((sum, c) => sum + (c.bands[0]?.value ?? 0), 0)
  return Math.round((cacheTotal / grandTotal.value) * 100)
})

const hoveredColumn = computed(() =>
  hovered.value ? columns.value[hovered.value.column] : null
)

const hoveredBand = computed(() =>
  hovered.value?.band != null ? hoveredColumn.value?.bands[hovered.value.band] ?? null : null
)

const badge = computed(() => {
  if (!hoveredColumn.value) return t('admin.dashboard.modelMix.cacheBadge', { pct: cacheShare.value })
  if (hoveredBand.value) return `${hoveredColumn.value.label} · ${hoveredBand.value.label}`
  return hoveredColumn.value.label
})

const subline = computed(() => {
  const periodLabel = t(`admin.dashboard.period.${period.value}`).toLowerCase()
  if (hoveredBand.value) {
    return t('admin.dashboard.modelMix.subBand', {
      band: hoveredBand.value.label.toLowerCase(),
      period: periodLabel
    })
  }
  if (hoveredColumn.value) {
    return t('admin.dashboard.modelMix.subModel', {
      model: hoveredColumn.value.label,
      period: periodLabel
    })
  }
  return t('admin.dashboard.modelMix.subDefault', { period: periodLabel })
})

/* The hero number follows the HOVER here, unlike the donut's centre total: a
   stacked bar's point is comparing parts, so drilling into one and watching
   the number resolve to it is the interaction. The donut's centre is a total
   with no such drill. */
const heroValue = computed(() => {
  if (hoveredBand.value) return hoveredBand.value.value
  if (hoveredColumn.value) return columnTotals.value[hovered.value!.column] ?? 0
  return grandTotal.value
})

const periodLabel = computed(() =>
  t('admin.dashboard.period.thisPeriod', { period: t(`admin.dashboard.period.${period.value}`) })
)

watch(period, async (next) => {
  hovered.value = null
  if (!cache.value[next]) {
    try {
      await fetchPeriod(next)
    } catch {
      cache.value = { ...cache.value, [next]: [] }
    }
  }
})

onMounted(async () => {
  try {
    await fetchPeriod(period.value)
  } catch {
    cache.value = { ...cache.value, [period.value]: [] }
  } finally {
    loading.value = false
  }
  /* Quietly warm the rest so the first switch morphs immediately. Sequential,
     not parallel: these are background work and should not contend with
     whatever the user is actually looking at. */
  for (const key of Object.keys(PERIOD_DAYS) as PeriodKey[]) {
    if (key === period.value) continue
    try {
      await fetchPeriod(key)
    } catch {
      /* A warm that fails just means that period pays for its own fetch. */
    }
  }
})
</script>

<template>
  <div class="mmix">
    <div class="mmix__card mmix__head">
      <div class="mmix__head-row">
        <span class="mmix__title">
          <i class="hgi-stroke hgi-chart-bar-big mmix__title-icon" aria-hidden="true" />
          {{ t('admin.dashboard.modelMix.title') }}
        </span>
        <span class="mmix__period">
          <Transition name="mmix-period">
            <span :key="period" class="mmix__period-text">{{ periodLabel }}</span>
          </Transition>
        </span>
      </div>
      <p class="mmix__hero">
        <NumberTicker :value="heroValue" />
        <span class="mmix__badge" :data-hot="hovered ? true : undefined">{{ badge }}</span>
      </p>
      <p class="mmix__sub">{{ subline }}</p>
    </div>

    <div class="mmix__card mmix__body">
      <ul class="mmix__legend">
        <li
          v-for="(item, i) in legend"
          :key="item.key"
          class="mmix__chip"
          :data-dim="hovered?.band != null && hovered.band !== i || undefined"
        >
          <span class="mmix__swatch" :style="{ background: item.color }" />
          {{ item.label }}
        </li>
      </ul>

      <div v-if="!loading && !columns.length" class="mmix__empty">
        {{ t('admin.dashboard.noDataAvailable') }}
      </div>
      <div v-else class="mmix__chart">
        <div class="mmix__axis">
          <span v-for="tick in ticks" :key="tick.f" class="mmix__tick">{{ tick.label }}</span>
        </div>
        <div class="mmix__plot">
          <!-- Gridlines are DOM, not canvas: they are static, and redrawing
               them every frame beside the tiles would be pure waste. -->
          <span
            v-for="tick in ticks"
            :key="tick.f"
            class="mmix__grid"
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

      <div v-if="columns.length" class="mmix__labels">
        <span
          v-for="(col, i) in columns"
          :key="col.key"
          class="mmix__label"
          :data-hot="hovered?.column === i || undefined"
          :title="col.label"
          >{{ col.label }}</span
        >
      </div>
    </div>

    <Segmented
      v-model="period"
      :items="periodItems"
      :aria-label="t('admin.dashboard.modelMix.title')"
      class="mmix__periods"
    />
  </div>
</template>

<style scoped>
.mmix {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: 14px;
  background: color-mix(in oklch, var(--card) 96%, var(--foreground));
  box-shadow: 0 24px 50px -34px rgb(15 23 42 / 0.3);
}

.mmix__card {
  border-radius: 10px;
  background: var(--card);
  box-shadow:
    0 0 1.76px rgb(0 0 0 / 0.08),
    0 1px 1.76px rgb(25 28 33 / 0.06),
    0 0 0 1px rgb(25 28 33 / 0.04);
}

.mmix__head {
  padding: 14px 16px 16px;
}

.mmix__head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.mmix__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.mmix__title-icon {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.mmix__period {
  position: relative;
  display: block;
  min-width: 96px;
  height: 1.2em;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: right;
}

.mmix__period-text {
  position: absolute;
  inset: 0;
  white-space: nowrap;
}

.mmix-period-enter-active,
.mmix-period-leave-active {
  transition:
    opacity 0.18s cubic-bezier(0.19, 1, 0.22, 1),
    transform 0.18s cubic-bezier(0.19, 1, 0.22, 1),
    filter 0.18s cubic-bezier(0.19, 1, 0.22, 1);
}

.mmix-period-enter-from {
  opacity: 0;
  transform: translateY(5px);
  filter: blur(3px);
}

.mmix-period-leave-to {
  opacity: 0;
  transform: translateY(-5px);
  filter: blur(3px);
}

.mmix__hero {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin: 6px 0 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  min-width: 0;
}

/* Brand while it reports the standing figure, muted once it is echoing
   whatever the pointer is on -- the colour marks which of the two it is. */
.mmix__badge {
  flex: 1;
  min-width: 0;
  color: var(--brand);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mmix__badge[data-hot] {
  color: var(--muted-foreground);
}

.mmix__sub {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.mmix__body {
  padding: 14px 16px 16px;
}

.mmix__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
  margin: 0 0 12px;
  padding: 0;
  list-style: none;
}

.mmix__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  transition: opacity 0.16s var(--ease-out);
}

.mmix__chip[data-dim] {
  opacity: 0.4;
}

.mmix__swatch {
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.mmix__chart {
  display: flex;
  gap: 8px;
}

/* Same height as the plot, so an instance with no model data does not change
   the panel's size and shuffle the grid around it. */
.mmix__empty {
  display: grid;
  place-items: center;
  height: 168px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.mmix__axis {
  display: flex;
  flex: none;
  flex-direction: column;
  justify-content: space-between;
  width: 34px;
  height: 168px;
  text-align: right;
}

.mmix__tick {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
  /* Each label is centred on its gridline rather than sitting above it. */
  transform: translateY(-50%);
}

.mmix__plot {
  position: relative;
  flex: 1;
  height: 168px;
  min-width: 0;
  border-radius: 5px;
  /*
   * The hatch is the reference's, at our ink so it survives a theme flip.
   *
   * Flagged by ground rule 7, and worth the disable rather than a workaround:
   * the rule bans gradients as in a colour RAMP -- an interpolation used for
   * decoration. This is a hard-stop pattern, one colour and transparent with
   * no interpolation between them, which happens to be spelled with the
   * gradient function because CSS offers no other way to tile a diagonal
   * hairline. An SVG data URI would satisfy the linter and change nothing on
   * screen, which is the definition of working around a check rather than
   * answering it.
   */
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.mmix__grid {
  position: absolute;
  left: 0;
  right: 0;
  height: 0;
  border-top: 1px dashed color-mix(in oklch, var(--foreground) 14%, transparent);
}

.mmix__grid[data-base] {
  border-top: 1.5px solid var(--border);
}

.mmix__labels {
  display: flex;
  margin-top: 8px;
  padding-left: 42px;
}

.mmix__label {
  flex: 1;
  min-width: 0;
  padding: 0 4px;
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  text-align: center;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition: color 0.18s var(--ease-out);
}

.mmix__label[data-hot] {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.mmix__periods {
  align-self: center;
}
</style>
