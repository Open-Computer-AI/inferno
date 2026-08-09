<script setup lang="ts">
/**
 * UserConcurrencyCell — table cells part 08, kind C ("Ratio").
 *
 * Migration note from the prototype (restCells, "UserConcurrencyCell"):
 *   "Two plain numbers today. Gains the bar, because 8 of 10 matters and 8
 *    alone does not."
 *
 * The pre-June version already coloured the chip by state (full/in-use/idle
 * at its own cutoffs); this keeps the same spirit but moves onto the one
 * shared threshold rule every ratio cell in this part uses (brand under 80,
 * attention from 80, destructive over 100), composing CapacityBar for the
 * bar itself instead of a colour-only chip.
 *
 * Props are unchanged (`current`, `max`).
 */
import { computed } from 'vue'
import CapacityBar from '@/components/common/CapacityBar.vue'

const props = defineProps<{
  current: number
  max: number
}>()

const percent = computed(() => (props.max > 0 ? (props.current / props.max) * 100 : 0))
const unset = computed(() => props.max <= 0)

const ink = computed(() => {
  if (unset.value) return 'var(--muted-foreground)'
  if (percent.value > 100) return 'var(--destructive)'
  if (percent.value >= 80) return 'var(--s2a-attn)'
  return 'var(--foreground)'
})
</script>

<template>
  <div class="ucc">
    <CapacityBar :percent="percent" :unset="unset" size="sm" class="ucc__bar" />
    <span class="ucc__value" :style="{ color: ink }">
      {{ current }}<span class="ucc__sep">/</span>{{ max }}
    </span>
  </div>
</template>

<style scoped>
.ucc {
  display: flex;
  align-items: center;
  gap: 6px;
}

.ucc__bar {
  width: 56px;
  flex-shrink: 0;
}

.ucc__value {
  flex-shrink: 0;
  font-size: var(--fs-sm);
}

.ucc__sep {
  margin: 0 2px;
  color: var(--muted-foreground);
}
</style>
