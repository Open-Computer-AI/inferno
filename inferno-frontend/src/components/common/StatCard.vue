<script setup lang="ts">
import { computed } from 'vue'
import type { Component } from 'vue'

/**
 * StatCard — part 03, section 01.
 *
 * Migration note from the prototype:
 *   "Eight of these on the admin dashboard, each with a differently coloured
 *    icon tile, each in its own shadowed rounded-2xl card. Eight equally
 *    loud things is the same as no hierarchy at all. Drop the iconVariant
 *    tint tile and the per-card shadow. changeType maps to three colours
 *    instead of six. A number over six digits abbreviates and keeps the
 *    exact value in a title attribute."
 *
 * `icon` and `iconVariant` stay in the signature — the props table marks
 * both "accepted and ignored", so the existing call site keeps compiling —
 * but neither renders anything. A KPI does not need a decorative glyph, and
 * eight coloured tiles was the thing that read as generated in the first
 * place.
 *
 * This is the strip/grid-cell treatment: value at --font-serif --fs-2xl on
 * a plain --card fill, no border or radius of its own, because the
 * hairlines between cells belong to the container the caller composes
 * around a row of these (1px --border-subtle grid, gap:1px). The
 * one-or-two-card standalone treatment at --fs-display is a different
 * component, june/MeasureBlock.vue.
 */
type ChangeType = 'up' | 'down' | 'neutral'
type IconVariant = 'primary' | 'success' | 'warning' | 'danger'

interface Props {
  title: string
  value: number | string
  icon?: Component
  iconVariant?: IconVariant
  change?: number
  changeType?: ChangeType
  formatValue?: (value: number | string) => string
}

const props = withDefaults(defineProps<Props>(), {
  changeType: 'neutral',
  iconVariant: 'primary'
})

function trimTrailingZero(s: string): string {
  return s.replace(/\.0$/, '')
}

// Abbreviates past six digits, per "284.1B, exact on hover" in the
// overflow-state spec. The exact figure always lives in the title attribute.
function abbreviate(n: number): string {
  const abs = Math.abs(n)
  if (abs >= 1_000_000_000) return `${trimTrailingZero((n / 1_000_000_000).toFixed(1))}B`
  if (abs >= 1_000_000) return `${trimTrailingZero((n / 1_000_000).toFixed(1))}M`
  return n.toLocaleString()
}

const formattedValue = computed(() => {
  if (props.formatValue) return props.formatValue(props.value)
  if (typeof props.value === 'number') return abbreviate(props.value)
  return props.value
})

const exactValue = computed(() => {
  if (typeof props.value === 'number') return props.value.toLocaleString()
  return String(props.value)
})

const formattedChange = computed(() => {
  if (props.change === undefined) return ''
  return `${Math.abs(props.change)}%`
})

const arrowIcon = computed(() => (props.changeType === 'up' ? 'hgi-arrow-up-01' : 'hgi-arrow-down-01'))
</script>

<template>
  <div class="statcell">
    <p class="statcell__title">{{ title }}</p>
    <p class="statcell__value" :title="exactValue">{{ formattedValue }}</p>
    <p v-if="change !== undefined" class="statcell__change" :data-tone="changeType">
      <i
        v-if="changeType !== 'neutral'"
        class="hgi-stroke statcell__icon"
        :class="arrowIcon"
        aria-hidden="true"
      />
      <span>{{ formattedChange }}</span>
    </p>
  </div>
</template>

<style scoped>
/*
 * Class is `statcell`, NOT `stat-card`.
 *
 * `style.css` defines a legacy global `.stat-card { @apply card p-5; flex
 * items-start gap-4 }`. This component used the same name, so every converted
 * stat card silently inherited that card's border and radius AND its flex row
 * -- which is why the title and value sat side by side instead of stacked, and
 * why four of them in a StatStrip rendered as four separate cards rather than
 * one strip with dividers. A scoped attribute wins on the properties it
 * declares; it cannot un-declare the ones it does not.
 */
.statcell {
  padding: 13px 15px;
  background: var(--card);
  min-width: 0;
}

.statcell__title {
  margin: 0;
  font-size: var(--fs-xs);
  color: var(--muted-foreground);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.statcell__value {
  margin: 3px 0 0;
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  line-height: 1.1;
  color: var(--foreground);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.statcell__change {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 2px 0 0;
  font-size: var(--fs-sm);
  min-width: 0;
}
.statcell__change span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Three colours, not six: changeType is the only thing that ever paints
   this line, iconVariant no longer touches colour at all. */
.statcell__change[data-tone='up'] {
  color: var(--success);
}
.statcell__change[data-tone='down'] {
  color: var(--destructive);
}
.statcell__change[data-tone='neutral'] {
  color: var(--muted-foreground);
}

.statcell__icon {
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph */
  flex-shrink: 0;
}
</style>
