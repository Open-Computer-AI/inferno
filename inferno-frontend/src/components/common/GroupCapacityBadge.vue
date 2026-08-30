<script setup lang="ts">
/**
 * GroupCapacityBadge — part 03, section 07 to 09 (capacity, quota, usage bar).
 *
 * Migration note from the prototype:
 *   "Four components measuring how full something is, in four visual
 *    languages. They keep their props and their separate jobs, but share one
 *    threshold rule: accent under 80 percent, attention to 100, destructive
 *    over."
 *   "Group capacity, three dimensions in one 104px cell" — three 4px bars
 *    keyed C, S and R, replacing the three separate coloured pills.
 *   "GroupCapacityBadge stops rendering three separate coloured pills."
 *
 * CapacityBadge, QuotaBadge and UsageProgressBar carry the same threshold
 * rule but are not owned by this pass (only GroupCapacityBadge.vue is), so
 * the threshold function is duplicated here rather than centralized in a
 * shared util. See the report for that gap.
 *
 * Prop surface (concurrencyUsed/Max, sessionsUsed/Max, rpmUsed/Max) is
 * unchanged; sessions and RPM stay hidden when their max is 0, same as
 * before.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

interface Props {
  concurrencyUsed: number
  concurrencyMax: number
  sessionsUsed: number
  sessionsMax: number
  rpmUsed: number
  rpmMax: number
}

const props = withDefaults(defineProps<Props>(), {
  concurrencyUsed: 0,
  concurrencyMax: 0,
  sessionsUsed: 0,
  sessionsMax: 0,
  rpmUsed: 0,
  rpmMax: 0
})

const { t } = useI18n()

// Accent under 80%, attention approaching the cap, destructive AT the cap --
// used >= max is saturation, and an enforced cap never exceeds 100%. No max (never seen
// today, since a group always has a concurrency cap) falls back to neutral.
function pctOf(used: number, max: number): number | null {
  return max > 0 ? (used / max) * 100 : null
}

function barColor(pct: number | null): string {
  if (pct === null) return 'var(--muted)'
  if (pct >= 100) return 'var(--destructive)'
  if (pct >= 80) return 'var(--s2a-attn-bg)'
  return 'var(--brand)'
}

function inkColor(pct: number | null): string {
  if (pct === null) return 'var(--muted-foreground)'
  if (pct >= 100) return 'var(--destructive)'
  if (pct >= 80) return 'var(--s2a-attn)'
  return 'var(--body-copy)'
}

interface Dim {
  key: string
  widthPct: number
  color: string
  ink: string
  text: string
}

function buildDim(key: string, used: number, max: number): Dim {
  const pct = pctOf(used, max)
  return {
    key,
    widthPct: pct === null ? 0 : Math.min(pct, 100),
    color: barColor(pct),
    ink: inkColor(pct),
    text: `${used}/${max}`
  }
}

const dims = computed<Dim[]>(() => {
  const list: Dim[] = [
    buildDim(t('common.groupCapacity.concurrencyAbbr'), props.concurrencyUsed, props.concurrencyMax)
  ]
  if (props.sessionsMax > 0) {
    list.push(buildDim(t('common.groupCapacity.sessionsAbbr'), props.sessionsUsed, props.sessionsMax))
  }
  if (props.rpmMax > 0) {
    list.push(buildDim(t('common.groupCapacity.rpmAbbr'), props.rpmUsed, props.rpmMax))
  }
  return list
})
</script>

<template>
  <div class="gcap">
    <div v-for="dim in dims" :key="dim.key" class="gcap__row">
      <span class="gcap__key">{{ dim.key }}</span>
      <span class="gcap__track"><span class="gcap__fill" :style="{ width: dim.widthPct + '%', background: dim.color }" /></span>
      <span class="gcap__val" :style="{ color: dim.ink }">{{ dim.text }}</span>
    </div>
  </div>
</template>

<style scoped>
/* "Three dimensions in one 104px cell", so it drops into a table column
   without reflowing it. */
.gcap {
  display: flex;
  flex-direction: column;
  width: 104px;
}

.gcap__row {
  display: grid;
  grid-template-columns: 14px minmax(0, 1fr) auto;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}
.gcap__row:last-child {
  margin-bottom: 0;
}

.gcap__key {
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.gcap__track {
  display: block;
  height: 4px;
  border-radius: 2px;
  background: var(--muted);
  overflow: hidden;
}

.gcap__fill {
  display: block;
  height: 100%;
  border-radius: 2px;
  transition: width var(--t-med) var(--ease-out);
}

.gcap__val {
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

@media (prefers-reduced-motion: reduce) {
  .gcap__fill {
    transition: none;
  }
}
</style>
