<script setup lang="ts">
/**
 * Tokens per day, split by the users who spent them.
 *
 * The last Chart.js instance on the admin dashboard. It drew twelve
 * `<Line>` datasets over one another, each coloured from a hardcoded twelve
 * entry hex rainbow -- #3b82f6, #10b981, #f59e0b, #ef4444, #8b5cf6, ... -- on
 * a page where every other chart is a dither field on the brand hue. It was
 * the only blue thing on the screen.
 *
 * LINES BECOME A STACK, AND THAT IS A CORRECTNESS FIX
 * Twelve overlapping lines is spaghetti: you cannot read one user's share off
 * it, and you cannot read the total either, because lines do not add. These
 * series DO add -- per-user tokens on a given day sum to that day's total --
 * so stacking is the honest treatment here, and it answers both questions at
 * once: band thickness is one user's usage, total height is the day.
 *
 * (Worth contrasting with the ops throughput chart, which refused to stack:
 * QPS and TPS are different units and do not sum to anything, so that one got
 * two strips instead. Same component, opposite call, because the data differs.)
 *
 * TWELVE SERIES BECOMES EIGHT
 * June's categorical ramp is eight steps, and that is a deliberate ceiling --
 * past about eight bands nobody can match a colour to a legend entry anyway.
 * Users are ranked by total tokens; the top seven keep their own band and the
 * tail folds into "Other", the same call the token-mix donut already makes.
 * Nothing is dropped: "Other" carries the remainder, so the stack still sums
 * to the true daily total.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserUsageTrendPoint } from '@/types'
import DitherArea, { type AreaSeries } from '@/components/charts/DitherArea.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useChartTokens } from '@/composables/useChartTokens'
import { formatNumber } from '@/utils/format'

const props = defineProps<{
  points: UserUsageTrendPoint[]
  loading?: boolean
}>()

const { t } = useI18n()
const { tokens } = useChartTokens()

/** Eight steps in the ramp, so seven named users plus a remainder. */
const MAX_BANDS = 8

function displayName(point: UserUsageTrendPoint): string {
  return (
    point.username?.trim() ||
    point.email?.trim() ||
    t('admin.redeem.userPrefix', { id: point.user_id })
  )
}

/* Grouped by user_id, never by display name: two users can share a username or
   have none at all, and merging them would silently add their usage together. */
const model = computed(() => {
  const dates = new Set<string>()
  const users = new Map<number, { name: string; byDate: Map<string, number>; total: number }>()

  for (const point of props.points ?? []) {
    dates.add(point.date)
    let user = users.get(point.user_id)
    if (!user) {
      user = { name: displayName(point), byDate: new Map(), total: 0 }
      users.set(point.user_id, user)
    }
    const tokensOnDay = Number.isFinite(point.tokens) ? point.tokens : 0
    user.byDate.set(point.date, (user.byDate.get(point.date) ?? 0) + tokensOnDay)
    user.total += tokensOnDay
  }

  const labels = Array.from(dates).sort()
  const ranked = Array.from(users.values()).sort((a, b) => b.total - a.total)

  const named = ranked.slice(0, MAX_BANDS - 1)
  const tail = ranked.slice(MAX_BANDS - 1)

  const series: AreaSeries[] = named.map((user, i) => ({
    key: `u${i}`,
    label: user.name,
    values: labels.map((d) => user.byDate.get(d) ?? 0),
    color: tokens.ramp[i] ?? tokens.brand
  }))

  /* The tail is summed rather than discarded, so the stack height stays the
     true daily total and the chart cannot quietly understate usage. */
  if (tail.length) {
    series.push({
      key: 'other',
      label: t('admin.dashboard.otherUsers', { count: tail.length }),
      values: labels.map((d) => tail.reduce((sum, u) => sum + (u.byDate.get(d) ?? 0), 0)),
      color: tokens.ramp[MAX_BANDS - 1] ?? tokens.mutedForeground
    })
  }

  const total = ranked.reduce((sum, u) => sum + u.total, 0)
  return { labels, series, total, userCount: users.size }
})

const hasData = computed(() => model.value.series.length > 0 && model.value.total > 0)

/* Dates arrive as YYYY-MM-DD. Sliced, never passed through `new Date`: that
   would reinterpret a plain date in the browser's timezone and can shift the
   label by a day either side of UTC. */
const axisLabels = computed(() => model.value.labels.map((d) => d.slice(5)))

const restColor = computed(
  () => `color-mix(in oklch, ${tokens.mutedForeground} 26%, ${tokens.card})`
)
</script>

<template>
  <div class="utrend">
    <div class="utrend__head">
      <div>
        <h3 class="utrend__title">
          <i class="hgi-stroke hgi-chart-line-data-01 utrend__title-icon" aria-hidden="true" />
          {{ t('admin.dashboard.recentUsage') }}
        </h3>
        <p v-if="hasData" class="utrend__sub">
          {{ t('admin.dashboard.acrossUsers', model.userCount, { named: { count: model.userCount } }) }}
        </p>
      </div>
      <p v-if="hasData" class="utrend__total">
        {{ formatNumber(model.total) }}
        <span class="utrend__total-unit">{{ t('admin.dashboard.tokensUnit') }}</span>
      </p>
    </div>

    <div v-if="loading" class="utrend__state">
      <LoadingSpinner size="md" />
    </div>

    <div v-else-if="!hasData" class="utrend__state utrend__state--empty">
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>

    <template v-else>
      <div class="utrend__legend">
        <span v-for="s in model.series" :key="s.key" class="utrend__legend-item">
          <span class="utrend__swatch" :style="{ background: s.color }" aria-hidden="true" />
          <span class="utrend__legend-label">{{ s.label }}</span>
        </span>
      </div>

      <div class="utrend__plot">
        <DitherArea
          :series="model.series"
          :labels="axisLabels"
          :rest-color="restColor"
          :marker-color="tokens.brand"
        />
      </div>
    </template>
  </div>
</template>

<style scoped>
.utrend {
  display: flex;
  flex-direction: column;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.utrend__head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}

.utrend__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.utrend__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.utrend__sub {
  margin: 3px 0 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* Serif at display size, like every other headline figure in the product. */
.utrend__total {
  margin: 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.utrend__total-unit {
  margin-left: 6px;
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

.utrend__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 14px;
  margin-bottom: 10px;
}

.utrend__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.utrend__swatch {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

/* A username can be an email; it truncates rather than pushing the legend into
   a fourth row and shoving the plot down. */
.utrend__legend-label {
  max-width: 180px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.utrend__plot {
  position: relative;
  height: 256px;
  border-radius: 5px;
}

.utrend__state {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 256px;
}

.utrend__state--empty {
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}
</style>
