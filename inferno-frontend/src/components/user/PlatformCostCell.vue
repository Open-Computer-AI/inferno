<script setup lang="ts">
/**
 * PlatformCostCell — table cells part 08, kind B ("Quantity").
 *
 * Migration note from the prototype (restCells, "PlatformCostCell"):
 *   "Right aligned and tabular. The subtitle moves to the column header
 *    where it belongs."
 *
 * The literal specimen pairs a bare cost figure with a "platform, token
 * count" subtitle; `PlatformUsage` carries no token count (only
 * `today_actual_cost` / `total_actual_cost`), so that subtitle is not
 * fabricated here — see the report. What the migration note asks for
 * directly is honoured instead: the "Today" / "Last 30d" prefixes drop from
 * full inline labels to a small muted word, the two lines right align, and
 * the primary figure carries the weight instead of the label.
 *
 * Prop is unchanged (`usage?: PlatformUsage`).
 */
import { useI18n } from 'vue-i18n'
import type { PlatformUsage } from '@/api/admin/dashboard'

defineProps<{
  usage?: PlatformUsage
}>()

const { t } = useI18n()
</script>

<template>
  <div v-if="usage" class="pcc">
    <div class="pcc__row">
      <span class="pcc__label">{{ t('admin.users.today') }}</span>
      <span class="pcc__primary">${{ usage.today_actual_cost.toFixed(4) }}</span>
    </div>
    <div class="pcc__row pcc__row--secondary">
      <span class="pcc__label">{{ t('admin.users.total') }}</span>
      <span class="pcc__secondary">${{ usage.total_actual_cost.toFixed(4) }}</span>
    </div>
  </div>
  <span v-else class="pcc__empty">-</span>
</template>

<style scoped>
.pcc {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  min-width: 0;
}

.pcc__row {
  display: flex;
  align-items: baseline;
  gap: 5px;
  min-width: 0;
}

.pcc__label {
  flex-shrink: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.pcc__primary {
  overflow: hidden;
  color: var(--foreground);
  font-size: var(--fs-md);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pcc__row--secondary {
  margin-top: 1px;
}

.pcc__secondary {
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pcc__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
</style>
