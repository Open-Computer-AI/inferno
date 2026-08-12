<script setup lang="ts">
/**
 * Error distribution, as a tile ring.
 *
 * Converted from a Chart.js doughnut whose four categories were painted
 * #f59e0b / #3b82f6 / #ef4444 / #9ca3af -- orange, blue, red and grey. Four
 * unrelated hues for four parts of ONE total, which is the encoding part 09
 * removed everywhere else, and none of them Inferno's. The sequential brand
 * ramp replaces the lot: the slices are shares of a single number, so an
 * ordered ramp is what they want.
 *
 * The card chrome moved with it. `rounded-3xl bg-white shadow-sm
 * ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700` is legacy
 * Tailwind on a dead palette; it is now the same bordered --card surface every
 * converted panel uses, so this page stops looking like a different product
 * from the dashboard.
 */
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsErrorDistributionResponse } from '@/api/admin/ops'
import type { ChartState } from '../types'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherDonut, { type DonutSlice } from '@/components/charts/DitherDonut.vue'
import { useSequentialRamp } from '@/composables/useSequentialRamp'

interface Props {
  data: OpsErrorDistributionResponse | null
  loading: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'openDetails'): void
}>()
const { t } = useI18n()
const ramp = useSequentialRamp(4)
const hovered = ref<number | null>(null)

const totalSlaErrors = computed(() =>
  (props.data?.items ?? []).reduce((total, item) => total + Number(item.sla || 0), 0)
)

const hasData = computed(() => totalSlaErrors.value > 0)

const state = computed<ChartState>(() => {
  if (hasData.value) return 'ready'
  if (props.loading) return 'loading'
  return 'empty'
})

/*
 * Status codes collapse into four causes an operator can act on. Order is
 * fixed rather than sorted by size so a category keeps its ramp step -- a
 * slice that changed colour between refreshes would be unreadable.
 */
const categories = computed<DonutSlice[]>(() => {
  if (!props.data) return []

  let upstream = 0 // 502, 503, 504
  let client = 0 // 4xx
  let system = 0 // 500
  let other = 0

  for (const item of props.data.items || []) {
    const code = Number(item.status_code || 0)
    const count = Number(item.sla || 0)
    if (!Number.isFinite(code) || !Number.isFinite(count)) continue

    if ([502, 503, 504].includes(code)) upstream += count
    else if (code >= 400 && code < 500) client += count
    else if (code === 500) system += count
    else other += count
  }

  const rows: { key: string; label: string; value: number }[] = [
    { key: 'upstream', label: t('admin.ops.upstream'), value: upstream },
    { key: 'client', label: t('admin.ops.client'), value: client },
    { key: 'system', label: t('admin.ops.system'), value: system },
    { key: 'other', label: t('admin.ops.other'), value: other }
  ]

  return rows
    .filter((r) => r.value > 0)
    .map((r, i) => ({ ...r, color: ramp.value[i] ?? '' }))
})

const topReason = computed(() => {
  if (categories.value.length === 0) return null
  return categories.value.reduce((prev, cur) => (cur.value > prev.value ? cur : prev))
})
</script>

<template>
  <div class="opsdist">
    <div class="opsdist__head">
      <h3 class="opsdist__title">
        <i class="hgi-stroke hgi-alert-circle opsdist__title-icon" aria-hidden="true" />
        {{ t('admin.ops.errorDistribution') }}
        <HelpTooltip :content="t('admin.ops.tooltips.errorDistribution')" />
      </h3>
      <button
        type="button"
        class="opsdist__action"
        :disabled="state !== 'ready'"
        :title="t('admin.ops.errorTrend')"
        @click="emit('openDetails')"
      >
        {{ t('admin.ops.requestDetails.details') }}
      </button>
    </div>

    <div class="opsdist__body">
      <template v-if="state === 'ready' && categories.length">
        <div class="opsdist__ring">
          <DitherDonut
            :slices="categories"
            :hovered="hovered"
            @update:hovered="hovered = $event"
          />
        </div>
        <ul class="opsdist__legend">
          <li
            v-for="(item, i) in categories"
            :key="item.key"
            class="opsdist__row"
            :data-hot="hovered === i || undefined"
            :data-dim="(hovered !== null && hovered !== i) || undefined"
            @pointerenter="hovered = i"
            @pointerleave="hovered = null"
          >
            <span class="opsdist__swatch" :style="{ background: item.color }" />
            <span class="opsdist__label">{{ item.label }}</span>
            <span class="opsdist__value">{{ item.value }}</span>
          </li>
        </ul>
        <p v-if="topReason" class="opsdist__top">
          <!-- The locale string already ends in a colon ("Top:"), so adding
               one here rendered "Top:: Other". -->
          {{ t('admin.ops.top') }} <strong>{{ topReason.label }}</strong>
        </p>
      </template>

      <div v-else class="opsdist__state">
        <LoadingSpinner v-if="state === 'loading'" size="md" />
        <EmptyState v-else :title="t('common.noData')" :description="t('admin.ops.charts.emptyError')" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.opsdist {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.opsdist__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.opsdist__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

/* The old header used a red triangle glyph. Errors being present is not a
   fault -- some always are -- so the icon stays muted and the numbers carry
   the reading (ground rule 5). */
.opsdist__title-icon {
  font-size: 15px; /* june-lint-disable ground-rule-4: icon glyph, not text */
  color: var(--muted-foreground);
  line-height: 1;
}

.opsdist__action {
  flex: none;
  padding: 3px 9px;
  border: 1px solid var(--border);
  border-radius: var(--r-sm);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  cursor: pointer;
  transition: background var(--motion-hover);
}

.opsdist__action:hover:not(:disabled) {
  background: var(--sidebar-accent);
}

.opsdist__action:disabled {
  opacity: 0.5;
  cursor: default;
}

.opsdist__body {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 0;
}

.opsdist__ring {
  width: 100%;
  max-width: 168px;
}

.opsdist__legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 4px 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.opsdist__row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 7px;
  border-radius: var(--r-sm);
  cursor: default;
  transition:
    background var(--motion-hover),
    opacity var(--motion-hover);
}

.opsdist__row[data-hot] {
  background: var(--brand-tint);
}

.opsdist__row[data-dim] {
  opacity: 0.5;
}

.opsdist__swatch {
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

.opsdist__label {
  color: var(--body-copy);
  font-size: var(--fs-sm);
}

.opsdist__value {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}

.opsdist__top {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opsdist__top strong {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.opsdist__state {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
}
</style>
