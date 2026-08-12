<script setup lang="ts">
/**
 * "Members by plan", forked onto our data: the token mix.
 *
 * WHY THIS METRIC CARRIES THE DONUT
 * The reference donut wants a genuine part-of-whole with a handful of
 * categories and a total worth reading. Token mix is exactly that -- input,
 * output, cache write and cache read sum to total tokens by definition, and
 * the split is the single most load-bearing fact on this dashboard: it is what
 * explains the cost tile. A donut of "models" or "users" would be a ranking
 * wearing a circle, which is the thing part 09 removed doughnuts for.
 *
 * ONE FETCH, FOUR PERIODS
 * A year of daily buckets is pulled once and sliced client side. Refetching
 * per period would put a network round trip between the click and the morph,
 * and the morph IS the interaction -- a 300ms wait before the wedges start
 * moving reads as a bug rather than as an animation. 365 rows is a small
 * payload to make every period switch instant.
 *
 * SLICE ORDER IS FIXED, NOT SORTED BY SIZE
 * The ramp is sequential, so sorting by value would look tidier -- but a slice
 * would then change COLOUR when the data shifts, and the morph animates
 * angles, not colours. A wedge that slides and recolours at the same time is
 * unreadable. Fixed order means a slice keeps its identity across every period
 * change.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { TrendDataPoint } from '@/types'
import DitherDonut, { type DonutSlice } from '@/components/charts/DitherDonut.vue'
import NumberTicker from '@/components/common/NumberTicker.vue'
import Segmented from '@/components/common/Segmented.vue'
import { useSequentialRamp } from '@/composables/useSequentialRamp'

const { t } = useI18n()
const ramp = useSequentialRamp(4)

type PeriodKey = 'week' | 'month' | 'quarter' | 'year'
const PERIOD_DAYS: Record<PeriodKey, number> = {
  week: 7,
  month: 30,
  quarter: 90,
  year: 365
}

const period = ref<PeriodKey>('month')
const hovered = ref<number | null>(null)
const trend = ref<TrendDataPoint[]>([])
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

/* Daily buckets render as "YYYY-MM-DD" (backend dateFormatWhitelist), so the
   window is selected by string comparison -- no Date parsing, no timezone
   drift across the cutoff. Same reasoning as utils/dayOverDay.ts. */
const windowed = computed(() => {
  const days = PERIOD_DAYS[period.value]
  const cutoff = formatLocalDate(new Date(Date.now() - (days - 1) * 86400000))
  return trend.value.filter((p) => typeof p.date === 'string' && p.date.slice(0, 10) >= cutoff)
})

const totals = computed(() => {
  let input = 0
  let output = 0
  let cacheWrite = 0
  let cacheRead = 0
  for (const p of windowed.value) {
    input += Number(p.input_tokens) || 0
    output += Number(p.output_tokens) || 0
    cacheWrite += Number(p.cache_creation_tokens) || 0
    cacheRead += Number(p.cache_read_tokens) || 0
  }
  return { input, output, cacheWrite, cacheRead }
})

const slices = computed<DonutSlice[]>(() => [
  { key: 'cacheRead', label: t('admin.dashboard.mix.cacheRead'), value: totals.value.cacheRead, color: ramp.value[0] ?? '' },
  { key: 'input', label: t('admin.dashboard.mix.input'), value: totals.value.input, color: ramp.value[1] ?? '' },
  { key: 'cacheWrite', label: t('admin.dashboard.mix.cacheWrite'), value: totals.value.cacheWrite, color: ramp.value[2] ?? '' },
  { key: 'output', label: t('admin.dashboard.mix.output'), value: totals.value.output, color: ramp.value[3] ?? '' }
])

const grandTotal = computed(() => slices.value.reduce((sum, s) => sum + s.value, 0))

/* The header number follows the PERIOD, never the hover -- the spec is
   explicit, and it is right: a total that changes when you point at a slice
   stops being a total. */
const periodLabel = computed(() => t('admin.dashboard.period.toDate', {
  period: t(`admin.dashboard.period.${period.value}`)
}))

async function load() {
  loading.value = true
  try {
    const now = new Date()
    const response = await adminAPI.dashboard.getSnapshotV2({
      start_date: formatLocalDate(new Date(now.getTime() - 364 * 86400000)),
      end_date: formatLocalDate(now),
      granularity: 'day',
      include_stats: false,
      include_trend: true,
      include_model_stats: false,
      include_group_stats: false,
      include_users_trend: false
    })
    trend.value = response.trend || []
  } catch {
    trend.value = []
  } finally {
    loading.value = false
  }
}

/* Clicking a period clears the hover: the wedge under the cursor is about to
   be a different size and probably a different slice, so keeping the old index
   highlighted would explode the wrong one. */
watch(period, () => {
  hovered.value = null
})

onMounted(load)
</script>

<template>
  <div class="mix">
    <!-- Header card: label, period pill, hero total. -->
    <div class="mix__card mix__head">
      <div class="mix__head-row">
        <span class="mix__title">
          <i class="hgi-stroke hgi-database-02 mix__title-icon" aria-hidden="true" />
          {{ t('admin.dashboard.mix.title') }}
        </span>
        <span class="mix__period">
          <Transition name="mix-period">
            <span :key="period" class="mix__period-text">{{ periodLabel }}</span>
          </Transition>
        </span>
      </div>
      <p class="mix__total">
        <NumberTicker :value="grandTotal" />
      </p>
      <p class="mix__caption">{{ t('admin.dashboard.mix.caption') }}</p>
    </div>

    <!-- Chart card: ring plus legend. -->
    <div class="mix__card mix__body">
      <DitherDonut :slices="slices" :hovered="hovered" @update:hovered="hovered = $event" />
      <ul class="mix__legend">
        <li
          v-for="(slice, i) in slices"
          :key="slice.key"
          class="mix__row"
          :data-hot="hovered === i || undefined"
          :data-dim="hovered !== null && hovered !== i || undefined"
          @pointerenter="hovered = i"
          @pointerleave="hovered = null"
        >
          <span class="mix__swatch" :style="{ background: slice.color }" />
          <span class="mix__label">{{ slice.label }}</span>
          <span class="mix__value"><NumberTicker :value="slice.value" /></span>
        </li>
      </ul>
    </div>

    <Segmented
      v-model="period"
      :items="periodItems"
      :aria-label="t('admin.dashboard.mix.title')"
      class="mix__periods"
    />
  </div>
</template>

<style scoped>
/*
 * The tray and its cards follow the reference deck's own spec rather than the
 * StatTile treatment: 14px tray, 10px cards, 6px padding and gap, with the
 * three stacked shadows. This is the one surface where the fork is deliberately
 * literal -- owner call, and the panel reads as its own object because of it.
 */
.mix {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: 14px;
  background: color-mix(in oklch, var(--card) 96%, var(--foreground));
  box-shadow: 0 24px 50px -34px rgb(15 23 42 / 0.3);
}

.mix__card {
  border-radius: 10px;
  background: var(--card);
  box-shadow:
    0 0 1.76px rgb(0 0 0 / 0.08),
    0 1px 1.76px rgb(25 28 33 / 0.06),
    0 0 0 1px rgb(25 28 33 / 0.04);
}

.mix__head {
  padding: 14px 16px 16px;
}

.mix__head-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.mix__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.mix__title-icon {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

/* Fixed height and relative positioning so the two crossfading labels stack
   instead of reflowing the row as one leaves and the other arrives. */
.mix__period {
  position: relative;
  display: block;
  min-width: 110px;
  height: 1.2em;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  text-align: right;
}

.mix__period-text {
  position: absolute;
  inset: 0;
  white-space: nowrap;
}

.mix-period-enter-active,
.mix-period-leave-active {
  transition:
    opacity 0.18s cubic-bezier(0.19, 1, 0.22, 1),
    transform 0.18s cubic-bezier(0.19, 1, 0.22, 1),
    filter 0.18s cubic-bezier(0.19, 1, 0.22, 1);
}

.mix-period-enter-from {
  opacity: 0;
  transform: translateY(5px);
  filter: blur(3px);
}

.mix-period-leave-to {
  opacity: 0;
  transform: translateY(-5px);
  filter: blur(3px);
}

.mix__total {
  margin: 6px 0 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
}

.mix__caption {
  margin: 2px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.mix__body {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 18px 16px;
}

.mix__legend {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  min-width: 0;
}

.mix__row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 5px 8px;
  border-radius: var(--r-sm);
  cursor: default;
  /* Background and opacity only -- never border-color (ground rule 6). */
  transition:
    background 0.16s var(--ease-out),
    opacity 0.16s var(--ease-out);
}

.mix__row[data-hot] {
  background: var(--brand-tint);
}

.mix__row[data-dim] {
  opacity: 0.5;
}

.mix__swatch {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.mix__label {
  flex: 1;
  min-width: 0;
  color: var(--body-copy);
  font-size: var(--fs-md);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mix__row[data-hot] .mix__label,
.mix__row[data-hot] .mix__value {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.mix__value {
  flex: none;
  color: var(--foreground);
  font-size: var(--fs-md);
}

.mix__periods {
  align-self: center;
}

@media (max-width: 520px) {
  .mix__body {
    flex-direction: column;
    align-items: stretch;
    gap: 16px;
  }
}
</style>
