<script setup lang="ts">
/**
 * One quantity split into a handful of parts, as a single labelled bar.
 *
 * WHY THIS EXISTS RATHER THAN A DONUT
 * A donut is for a distribution across several categories where the ORDER does
 * not matter. This is for the two-or-three way split of one number where the
 * ratio itself is the reading -- "how much of my traffic succeeded" -- and a
 * bar answers that in one saccade because the eye compares lengths on a shared
 * baseline better than it compares angles.
 *
 * The specific thing it fixes: the ops header showed SLA at 88.050% and
 * request errors at 11.95% as two SEPARATE cards, when they are two halves of
 * one number. Two cards make the reader do the arithmetic to notice they sum;
 * one bar makes it impossible to miss.
 *
 * Segments carry a tone rather than a colour so the caller states MEANING and
 * the component owns the ink. That keeps ground rule 5 honest: success and
 * failure are states, so they take colour; a neutral remainder does not.
 */
import { computed } from 'vue'

export interface SplitSegment {
  key: string
  label: string
  value: number
  /** 'good' and 'bad' take state ink; 'neutral' stays muted. */
  tone: 'good' | 'bad' | 'neutral'
}

const props = withDefaults(
  defineProps<{
    segments: SplitSegment[]
    /** Sits under the bar, naming what the split is OF. */
    caption?: string
    /** Appended to each segment's count, e.g. "requests". */
    unit?: string
  }>(),
  { caption: '', unit: '' }
)

const total = computed(() =>
  props.segments.reduce((sum, s) => sum + Math.max(0, s.value), 0)
)

const parts = computed(() => {
  const sum = total.value
  return props.segments
    .filter((s) => s.value > 0)
    .map((s) => ({
      ...s,
      pct: sum > 0 ? (s.value / sum) * 100 : 0
    }))
})

/*
 * A segment under ~4% is narrower than its own rounded corners, so it renders
 * as a smudge rather than a block. Widening it distorts the ratio, which is
 * the one thing this chart exists to show -- so instead the FLOOR is applied
 * only to the drawn width while the label keeps the true percentage. A reader
 * comparing lengths is out by a hair; a reader reading numbers is not out at
 * all.
 */
const drawn = computed(() => {
  const rows = parts.value
  if (!rows.length) return []
  const FLOOR = 4
  const raised = rows.map((r) => ({ ...r, w: Math.max(FLOOR, r.pct) }))
  const over = raised.reduce((s, r) => s + r.w, 0)
  return raised.map((r) => ({ ...r, w: (r.w / over) * 100 }))
})
</script>

<template>
  <div class="split">
    <ul class="split__legend">
      <li v-for="part in parts" :key="part.key" class="split__item" :data-tone="part.tone">
        <span class="split__pct">{{ part.pct.toFixed(part.pct < 10 ? 2 : 1) }}%</span>
        <span class="split__label">{{ part.label }}</span>
        <span class="split__count">
          {{ part.value.toLocaleString() }}<template v-if="unit"> {{ unit }}</template>
        </span>
      </li>
    </ul>

    <div class="split__bar">
      <span
        v-for="part in drawn"
        :key="part.key"
        class="split__seg"
        :data-tone="part.tone"
        :style="{ width: `${part.w}%` }"
      />
    </div>

    <p v-if="caption" class="split__caption">{{ caption }}</p>
  </div>
</template>

<style scoped>
.split {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/*
 * Each part is a column with a rule on its leading edge, so the legend reads as
 * a split of one thing rather than as a list. The rule is a border on a static
 * element -- never transitioned (ground rule 6).
 */
.split__legend {
  display: flex;
  gap: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}

.split__item {
  flex: 1;
  min-width: 0;
  padding-left: 10px;
  border-left: 2px solid var(--border);
}

.split__item[data-tone='good'] {
  border-left-color: var(--brand);
}

.split__item[data-tone='bad'] {
  border-left-color: var(--s2a-attn);
}

.split__pct {
  display: block;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-xl);
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}

.split__label {
  display: block;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.split__count {
  display: block;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-variant-numeric: tabular-nums;
}

/* One track, no gaps between segments: a gap would read as a fourth category
   and break the "these sum to everything" claim the bar is making. */
.split__bar {
  display: flex;
  height: 8px;
  border-radius: var(--r-pill);
  background: var(--muted);
  overflow: hidden;
}

.split__seg {
  display: block;
  height: 100%;
  transition: width var(--t-slow) var(--ease-out);
}

.split__seg[data-tone='good'] {
  background: var(--brand);
}

.split__seg[data-tone='bad'] {
  background: var(--s2a-attn);
}

.split__seg[data-tone='neutral'] {
  background: var(--muted-foreground);
}

.split__caption {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}
</style>
