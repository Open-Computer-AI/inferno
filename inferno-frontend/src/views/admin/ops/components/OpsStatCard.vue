<script setup lang="ts">
/**
 * One reading and its supporting numbers, for the ops header row.
 *
 * The four cards this replaces were the same object written four times in
 * legacy Tailwind -- `rounded-2xl bg-gray-50 p-4 dark:bg-dark-900`, a
 * `text-[10px] font-bold uppercase` title, an optional `text-3xl font-black`
 * value and a stack of label/value rows. Four copies meant four places for the
 * treatment to drift, and it had: two of them coloured their headline by
 * threshold and two did not.
 *
 * WHAT CHANGED BEYOND THE TOKENS
 *
 *   ALL CAPS titles are gone (ground rule 1). "REQUEST DURATION" was shouting
 *   a label, not encoding anything.
 *
 *   The headline keeps its threshold colour, because that IS state -- a P99 of
 *   6s is a different fact from a P99 of 200ms. But the supporting rows lost
 *   theirs: they were `font-bold text-gray-900` on every row regardless of
 *   value, which is weight spent on nothing.
 *
 *   Numbers are tabular. These are columns of figures that update on a timer,
 *   and proportional digits make the column shimmy on every refresh.
 */
import HelpTooltip from '@/components/common/HelpTooltip.vue'

export interface StatRow {
  label: string
  value: string
}

withDefaults(
  defineProps<{
    title: string
    tooltip?: string
    /** The headline. Omit for cards that are only a list of rows. */
    value?: string
    /** Sits after the value in muted text, e.g. "ms (P99)". */
    unit?: string
    /** Colours the headline only. 'none' leaves it as foreground. */
    tone?: 'none' | 'ok' | 'warning' | 'critical'
    rows?: StatRow[]
    detailsLabel?: string
    showDetails?: boolean
  }>(),
  { tone: 'none', rows: () => [], showDetails: false }
)

const emit = defineEmits<{ (e: 'openDetails'): void }>()
</script>

<template>
  <div class="opscard">
    <div class="opscard__head">
      <span class="opscard__title">
        {{ title }}
        <HelpTooltip v-if="tooltip" :content="tooltip" />
      </span>
      <button v-if="showDetails" type="button" class="opscard__action" @click="emit('openDetails')">
        {{ detailsLabel }}
      </button>
    </div>

    <p v-if="value" class="opscard__value" :data-tone="tone">
      {{ value }}<span v-if="unit" class="opscard__unit">{{ unit }}</span>
    </p>

    <dl v-if="rows.length" class="opscard__rows" :data-tight="!!value || undefined">
      <div v-for="row in rows" :key="row.label" class="opscard__row">
        <dt class="opscard__label">{{ row.label }}</dt>
        <dd class="opscard__figure">{{ row.value }}</dd>
      </div>
    </dl>
  </div>
</template>

<style scoped>
.opscard {
  padding: 14px 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
  min-width: 0;
}

.opscard__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

/* Sentence case, not the old uppercase: a label shouting is not encoding
   anything (ground rule 1). */
.opscard__title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.opscard__action {
  flex: none;
  border: 0;
  background: none;
  color: var(--brand);
  font-size: var(--fs-sm);
  cursor: pointer;
}

.opscard__action:hover {
  text-decoration: underline;
}

.opscard__value {
  margin: 6px 0 0;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

/* Threshold colour stays on the headline because it IS state -- a P99 of 6s
   is a different fact from 200ms, not a different category. */
.opscard__value[data-tone='ok'] {
  color: var(--success);
}
.opscard__value[data-tone='warning'] {
  color: var(--s2a-attn);
}
.opscard__value[data-tone='critical'] {
  color: var(--destructive);
}

.opscard__unit {
  margin-left: 6px;
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

.opscard__rows {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 8px 0 0;
}

.opscard__rows[data-tight] {
  gap: 2px;
}

.opscard__row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
}

.opscard__label {
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* No bold. Every row was font-bold before, which is weight spent on nothing --
   if all of them are emphasised, none of them is. */
.opscard__figure {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
</style>
