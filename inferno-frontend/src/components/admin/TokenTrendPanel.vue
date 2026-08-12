<script setup lang="ts">
/**
 * "New members", forked onto our data: tokens per day, as a field of tiles.
 *
 * IT CAN SHOW A REAL DELTA, UNLIKE THE OTHER TWO
 * A year of daily buckets is fetched once and every range is a slice of it --
 * which means the PREVIOUS equal-length window is already in memory too. So
 * the "up 14%" badge here is a genuine like-for-like comparison computed
 * client side, not a second query and not a number stood in for. Both windows
 * are whole days, so none of the partial-day trouble the fold's delta pills
 * have to defend against applies.
 */
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { TrendDataPoint } from '@/types'
import DitherArea from '@/components/charts/DitherArea.vue'
import NumberTicker from '@/components/common/NumberTicker.vue'
import Segmented from '@/components/common/Segmented.vue'
import { useChartTokens } from '@/composables/useChartTokens'

const { t } = useI18n()
const { tokens } = useChartTokens()

const RANGES = [7, 14, 30, 90] as const
type RangeKey = '7' | '14' | '30' | '90'

const range = ref<RangeKey>('30')
const trend = ref<TrendDataPoint[]>([])
const loading = ref(true)

const rangeItems = computed(() =>
  RANGES.map((d) => ({ value: String(d), label: `${d}D` }))
)

function formatLocalDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(
    date.getDate()
  ).padStart(2, '0')}`
}

/* Sorted here, once, rather than trusted: the field draws whatever sequence it
   is handed, so an unordered response would produce a plausible scribble
   rather than an obvious failure. */
const daily = computed(() =>
  [...trend.value]
    .filter((p) => typeof p.date === 'string')
    .sort((a, b) => a.date.localeCompare(b.date))
)

const days = computed(() => Number(range.value))

const windowed = computed(() => daily.value.slice(-days.value))

/** The equal-length window immediately before the visible one. */
const previous = computed(() =>
  daily.value.slice(-days.value * 2, -days.value)
)

const values = computed(() =>
  windowed.value.map((p) => Number(p.total_tokens) || 0)
)

const labels = computed(() =>
  windowed.value.map((p) => {
    const [, m, d] = p.date.slice(0, 10).split('-')
    return `${m}/${d}`
  })
)

const total = computed(() => values.value.reduce((a, b) => a + b, 0))

const delta = computed(() => {
  const prior = previous.value.reduce((sum, p) => sum + (Number(p.total_tokens) || 0), 0)
  if (prior <= 0) return null
  const change = ((total.value - prior) / prior) * 100
  if (!Number.isFinite(change) || Math.abs(change) < 0.5) return null
  return { pct: Math.round(Math.abs(change)), up: change > 0 }
})

function abbreviate(n: number): string {
  if (n >= 1_000_000_000) return `${(n / 1_000_000_000).toFixed(1)}B`
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}k`
  return String(Math.round(n))
}

/* Four ticks at 1, 0.66, 0.33 and 0 of the peak. Not a nice-rounded axis like
   the bars: this scale re-zooms continuously as the range changes, and
   snapping it to round numbers would make the labels jump while the field
   flowed. */
const peak = computed(() => Math.max(1, ...values.value))
const ticks = computed(() =>
  [1, 0.66, 0.33, 0].map((f) => ({ f, label: abbreviate(peak.value * f) }))
)

/* Five evenly spaced date labels, first and last always included. */
const dateLabels = computed(() => {
  const n = labels.value.length
  if (n === 0) return []
  if (n <= 5) return labels.value
  return [0, 0.25, 0.5, 0.75, 1].map((f) => labels.value[Math.round(f * (n - 1))])
})

const restColor = computed(() =>
  `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)

onMounted(async () => {
  try {
    const now = new Date()
    const response = await adminAPI.dashboard.getSnapshotV2({
      /* 180 days, not 90: the longest range needs an equal-length window
         BEFORE it to compute the delta, so the fetch has to cover twice what
         it ever displays. */
      start_date: formatLocalDate(new Date(now.getTime() - 180 * 86400000)),
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
})
</script>

<template>
  <div class="trend">
    <div class="trend__card trend__head">
      <span class="trend__title">
        <i class="hgi-stroke hgi-chart-line-data-01 trend__title-icon" aria-hidden="true" />
        {{ t('admin.dashboard.tokenTrend.title') }}
      </span>
      <p class="trend__hero">
        <NumberTicker :value="total" />
        <span v-if="delta" class="trend__delta" :data-up="delta.up || undefined">
          <i
            class="hgi-stroke trend__delta-icon"
            :class="delta.up ? 'hgi-arrow-up-right-01' : 'hgi-arrow-down-right-01'"
            aria-hidden="true"
          />{{ delta.pct }}%
        </span>
      </p>
    </div>

    <div class="trend__card trend__body">
      <div v-if="!loading && !values.length" class="trend__empty">
        {{ t('admin.dashboard.noDataAvailable') }}
      </div>
      <template v-else>
        <div class="trend__chart">
          <div class="trend__axis">
            <span v-for="tick in ticks" :key="tick.f" class="trend__tick">{{ tick.label }}</span>
          </div>
          <div class="trend__plot">
            <span
              v-for="tick in ticks"
              :key="tick.f"
              class="trend__grid"
              :data-base="tick.f === 0 || undefined"
              :style="{ top: `${(1 - tick.f) * 100}%` }"
            />
            <DitherArea
              :values="values"
              :labels="labels"
              :color="tokens.brand"
              :rest-color="restColor"
              :marker-color="tokens.brand"
            />
          </div>
        </div>
        <div class="trend__dates">
          <span v-for="(label, i) in dateLabels" :key="`${label}-${i}`" class="trend__date">{{
            label
          }}</span>
        </div>
      </template>
    </div>

    <Segmented
      v-model="range"
      :items="rangeItems"
      :aria-label="t('admin.dashboard.tokenTrend.title')"
      class="trend__ranges"
    />
  </div>
</template>

<style scoped>
.trend {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 6px;
  border-radius: 14px;
  background: color-mix(in oklch, var(--card) 96%, var(--foreground));
  box-shadow: 0 24px 50px -34px rgb(15 23 42 / 0.3);
}

.trend__card {
  border-radius: 10px;
  background: var(--card);
  box-shadow:
    0 0 1.76px rgb(0 0 0 / 0.08),
    0 1px 1.76px rgb(25 28 33 / 0.06),
    0 0 0 1px rgb(25 28 33 / 0.04);
}

.trend__head {
  padding: 14px 16px 16px;
}

.trend__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.trend__title-icon {
  font-size: 14px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.trend__hero {
  display: flex;
  align-items: baseline;
  gap: 10px;
  margin: 6px 0 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
}

/* Tokens rising is growth, so up is brand and down is muted -- the same
   sentiment-not-direction rule the fold's pills follow, with the polarity
   fixed because this metric only has one reading. */
.trend__delta {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.trend__delta[data-up] {
  color: var(--brand);
}

.trend__delta-icon {
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  line-height: 1;
}

.trend__body {
  padding: 14px 16px 16px;
}

.trend__empty {
  display: grid;
  place-items: center;
  height: 176px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.trend__chart {
  display: flex;
  gap: 8px;
}

.trend__axis {
  display: flex;
  flex: none;
  flex-direction: column;
  justify-content: space-between;
  width: 34px;
  height: 176px;
  text-align: right;
}

.trend__tick {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
  transform: translateY(-50%);
}

.trend__plot {
  position: relative;
  flex: 1;
  height: 176px;
  min-width: 0;
  border-radius: 5px;
  background-image: repeating-linear-gradient(-45deg, color-mix(in oklch, var(--foreground) 5%, transparent) 0 1px, transparent 1px 7px); /* june-lint-disable ground-rule-7: hard-stop hatch, not a colour ramp */
}

.trend__grid {
  position: absolute;
  left: 0;
  right: 0;
  height: 0;
  border-top: 1px dashed color-mix(in oklch, var(--foreground) 14%, transparent);
}

.trend__grid[data-base] {
  border-top: 1.5px solid var(--border);
}

.trend__dates {
  display: flex;
  justify-content: space-between;
  margin-top: 8px;
  padding-left: 42px;
}

.trend__date {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

.trend__ranges {
  align-self: center;
}
</style>
