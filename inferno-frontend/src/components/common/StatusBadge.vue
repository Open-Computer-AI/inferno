<script setup lang="ts">
/**
 * StatusBadge — part 03, section 06.
 *
 * The prototype's own section 06 is account/AccountStatusIndicator.vue, a
 * 350-line component deriving state from six timestamps with a field/reason
 * table and a live countdown, which is out of scope here. This file is the
 * smaller, generic dot-plus-label primitive; the migration note's underlying
 * rule still applies to it directly:
 *   "Keep every derivation. The change is that expired and disabled become
 *    neutral rather than red, since neither is a failure... dots --success,
 *    --s2a-attn-bg, --destructive, --muted-foreground."
 *
 * This primitive has no separate expired/disabled distinction in its status
 * enum, just `disabled` / `inactive`, so both take the same neutral
 * treatment: an off switch is not a failure.
 *
 * Prop surface (`status`, `label`) is unchanged.
 */
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
}>()

const ink = computed(() => {
  switch (props.status) {
    case 'active':
    case 'success':
      return 'var(--success)'
    case 'warning':
      return 'var(--s2a-attn-bg)'
    case 'error':
    case 'danger':
      return 'var(--destructive)'
    case 'disabled':
    case 'inactive':
    default:
      return 'var(--muted-foreground)'
  }
})
</script>

<template>
  <span class="stbadge">
    <span class="stbadge__dot" :style="{ background: ink }" aria-hidden="true" />
    <span class="stbadge__label" :style="{ color: ink }">{{ label }}</span>
  </span>
</template>

<style scoped>
.stbadge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.stbadge__dot {
  flex-shrink: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.stbadge__label {
  font-size: var(--fs-sm);
}
</style>
