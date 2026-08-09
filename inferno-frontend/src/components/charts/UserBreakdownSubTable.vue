<script setup lang="ts">
/**
 * UserBreakdownSubTable -- part 09, "the pattern for every row expansion in
 * the product, including the usage log tooltip replacement".
 *
 * Not a bespoke nested <table>: a DataTable at density="compact" (28px rows,
 * "a table nested in a card or modal" is that density's own documented
 * case), so every row-expansion panel in the product shares one empty
 * state, one loading skeleton and one mobile fallback instead of each
 * inventing its own. The only local decisions are the column set and the
 * indent that reads as "nested under the row above."
 *
 * Reuse: a caller that wants this same pattern -- the usage log tooltip
 * replacement the spec names, or any other row that expands into a related
 * list -- does not extend this component. It copies the shape: a
 * `.ubs2`-style wrapper (surface-subtle tint, indented, no card border of
 * its own) around a compact-density DataTable, with its own columns and
 * data. This file stays the user-breakdown instance of that shape, because
 * its columns (requests, tokens, cost) are specific to that data; forcing
 * an unrelated breakdown through THIS component's props would be the wrong
 * kind of reuse. Toggling it in place is the callers' job too: all three
 * (Endpoint/Group/ModelDistributionChart) render it directly under the row
 * that expanded, keyed off their own `expandedKey`, not through a prop here.
 *
 * Props, emits: unchanged from before the rewrite.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import type { UserBreakdownItem } from '@/types'

const { t } = useI18n()

const props = withDefaults(defineProps<{
  items: UserBreakdownItem[]
  loading?: boolean
  showAccountCost?: boolean
}>(), {
  loading: false,
  showAccountCost: true
})

const columns = computed<Column[]>(() => {
  const cols: Column[] = [
    { key: 'email', label: t('admin.dashboard.spendingRankingUser') },
    { key: 'requests', label: t('admin.dashboard.requests') },
    { key: 'total_tokens', label: t('admin.dashboard.tokens') },
    { key: 'actual_cost', label: t('admin.dashboard.actual') }
  ]
  if (props.showAccountCost) cols.push({ key: 'account_cost', label: t('admin.dashboard.accountCost') })
  cols.push({ key: 'cost', label: t('admin.dashboard.standard') })
  return cols
})

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatCost = (value: number | undefined | null): string => {
  if (value == null) return '0.0000'
  if (value >= 1000) return (value / 1000).toFixed(2) + 'K'
  if (value >= 1) return value.toFixed(2)
  if (value >= 0.01) return value.toFixed(3)
  return value.toFixed(4)
}
</script>

<template>
  <div class="ubs2">
    <DataTable
      :columns="columns"
      :data="items"
      :loading="loading"
      density="compact"
      row-key="user_id"
      :sticky-first-column="false"
      :sticky-actions-column="false"
    >
      <template #cell-email="{ row }">
        <span class="ubs2__user" :title="row.email">{{ row.email || t('admin.redeem.userPrefix', { id: row.user_id }) }}</span>
      </template>
      <template #cell-requests="{ value }">
        <span class="ubs2__num">{{ Number(value).toLocaleString() }}</span>
      </template>
      <template #cell-total_tokens="{ value }">
        <span class="ubs2__num">{{ formatTokens(Number(value)) }}</span>
      </template>
      <template #cell-actual_cost="{ value }">
        <span class="ubs2__num ubs2__num--actual">${{ formatCost(Number(value)) }}</span>
      </template>
      <template #cell-account_cost="{ value }">
        <span class="ubs2__num ubs2__num--account">${{ formatCost(Number(value)) }}</span>
      </template>
      <template #cell-cost="{ value }">
        <span class="ubs2__num ubs2__num--muted">${{ formatCost(Number(value)) }}</span>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
/* Reads as "nested under the row above": a tint instead of a card border
   (it is not a card, it is a detail panel), indented so the relationship to
   the row that opened it is legible without a connecting line. */
.ubs2 {
  padding: 2px 4px 2px 24px;
  background: var(--surface-subtle);
  border-top: 1px solid var(--border-subtle);
}

/* One size down from the parent table: a drill-down reads as subordinate
   detail, not a second table of equal weight. */
.ubs2 :deep(.dt-th),
.ubs2 :deep(.dt-td) {
  font-size: var(--fs-sm);
}

.ubs2__user {
  display: block;
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--foreground);
}

.ubs2__num {
  display: block;
  text-align: right;
  font-variant-numeric: tabular-nums;
  color: var(--body-copy);
}

.ubs2__num--actual {
  color: var(--success);
}

.ubs2__num--account {
  color: var(--s2a-attn);
}

.ubs2__num--muted {
  color: var(--muted-foreground);
}
</style>
