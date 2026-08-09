<script setup lang="ts">
/**
 * AccountTodayStatsCell — table cells part 08, kind B ("Quantity").
 *
 * Migration note from the prototype (restCells, "AccountTodayStatsCell"):
 *   "Three stacked numbers today, each with its own pulsing skeleton.
 *    Becomes two, and the skeleton goes flat."
 * Kind B anatomy (part 08, section 01):
 *   "Right aligned, tabular, unit stated once in the column header and never
 *    repeated in the cell. An absent value is an em rule in muted, never a
 *    zero."
 *
 * Two lines instead of three: requests (the scannable volume figure) as the
 * primary line, cost as the secondary line. Tokens and the separate
 * user-billed figure are not dropped, only demoted — they move into the
 * primary line's title tooltip, one hover away, rather than a third stacked
 * line that would grow past the 36px row.
 *
 * No tabular-nums here: these are per-render values in a table cell, not a
 * live-ticking figure in a fixed-width slot, so the numeric feature stays
 * off per the brief's own rule ("not by reflex").
 *
 * Props are unchanged.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatNumber, formatCurrency } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    stats?: WindowStats | null
    loading?: boolean
    error?: string | null
  }>(),
  {
    stats: null,
    loading: false,
    error: null
  }
)

const { t } = useI18n()

const formatTokens = (tokens: number): string => {
  if (tokens >= 1000000) return `${(tokens / 1000000).toFixed(2)}M`
  if (tokens >= 1000) return `${(tokens / 1000).toFixed(1)}K`
  return tokens.toString()
}

// Full breakdown, one hover away, instead of a third stacked line.
const detailTitle = computed(() => {
  const stats = props.stats
  if (!stats) return ''
  const lines = [
    `${t('admin.accounts.stats.requests')}: ${formatNumber(stats.requests)}`,
    `${t('admin.accounts.stats.tokens')}: ${formatTokens(stats.tokens)}`,
    `${t('usage.accountBilled')}: ${formatCurrency(stats.cost)}`
  ]
  if (stats.user_cost != null) {
    lines.push(`${t('usage.userBilled')}: ${formatCurrency(stats.user_cost)}`)
  }
  return lines.join('\n')
})
</script>

<template>
  <div class="tsc">
    <div v-if="loading && !stats" class="tsc__skel-group">
      <span class="tsc__skel" style="width: 60%" />
      <span class="tsc__skel" style="width: 42%" />
    </div>

    <div v-else-if="error && !stats" class="tsc__error" :title="error">
      {{ t('admin.accounts.todayStatsCell.loadFailed') }}
    </div>

    <div v-else-if="stats" class="tsc__stack" :title="detailTitle">
      <span class="tsc__primary">{{ t('admin.accounts.todayStatsCell.requests', { count: formatNumber(stats.requests) }) }}</span>
      <span class="tsc__secondary">{{ t('admin.accounts.todayStatsCell.costToday', { amount: formatCurrency(stats.cost) }) }}</span>
    </div>

    <span v-else class="tsc__empty">-</span>
  </div>
</template>

<style scoped>
.tsc {
  min-width: 0;
  text-align: right;
}

.tsc__skel-group {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.tsc__skel {
  display: block;
  height: 8px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
}

.tsc__error {
  overflow: hidden;
  font-size: var(--fs-sm);
  color: var(--destructive);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tsc__stack {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.tsc__primary {
  overflow: hidden;
  color: var(--foreground);
  font-size: var(--fs-md);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tsc__secondary {
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tsc__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
</style>
