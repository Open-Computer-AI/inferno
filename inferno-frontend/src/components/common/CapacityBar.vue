<script setup lang="ts">
import { computed } from 'vue'

/**
 * CapacityBar — part 03, section 07 to 09 ("Capacity, quota, usage bar"),
 * geometry superseded by part 15, section 03 ("Two calls for you").
 *
 * Migration note from part 15:
 *   "The segmented bar decided here supersedes the continuous bar in part
 *    three and the usage cell in part eight. It is one shared component at
 *    three sizes: 13px on this page, 11px in a settings row, 6px in a table
 *    cell where there is no room for ticks and the continuous bar stays."
 *
 * Ticks over one filled track: at 5% a continuous bar is a sliver too small
 * to read, three lit ticks out of forty four is a quantity you can count
 * without the label. A segmented bar also degrades honestly, because one
 * tick still shows at 1% where a continuous bar rounds a sliver away to
 * nothing. `sm` (6px, a table cell) is the one size with no room for ticks,
 * so it keeps the continuous fill part 03 originally specified.
 *
 * Colour is risk, not category (part 03, "Capacity, quota, usage bar"): fill
 * and ink share one threshold — neutral under 80%, `--s2a-attn` to 100%,
 * `--destructive` over. `unset` (no cap configured) is a fourth, always
 * neutral state distinct from "0% used", matching the CapacityBadge
 * "2 / infinity" case in the same section.
 */

type Size = 'lg' | 'md' | 'sm'

interface Props {
  /** Utilization, 0 to 100+. The bar clamps at 100; the number does not have to. */
  percent: number
  /** 13px ticks (default), 11px ticks for a settings row, or 6px continuous for a table cell. */
  size?: Size
  /** Tick count at lg/md. Ignored at sm, which is a continuous fill. */
  segments?: number
  /** Row label, e.g. "This month" or "5 hour window". */
  label?: string
  /** Right-aligned text on the label row: a percent, a reset note, whatever the caller has. */
  trailing?: string
  /** No cap configured. Renders fully neutral regardless of percent. */
  unset?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  size: 'lg',
  unset: false
})

const clamped = computed(() => Math.min(Math.max(props.percent, 0), 100))

// 44 ticks matches the settings-row demo, 60 matches the account-overview
// screen; both are decorative counts rather than a data-driven granularity.
const segmentCount = computed(() => props.segments ?? (props.size === 'md' ? 44 : 60))

// A segmented bar degrades honestly: any usage above zero lights at least
// one tick, where a continuous bar would round a sliver away to nothing.
const litCount = computed(() => {
  if (props.unset || props.percent <= 0) return 0
  return Math.max(1, Math.round((clamped.value / 100) * segmentCount.value))
})

const fillColor = computed(() => {
  if (props.unset) return 'var(--muted)'
  if (props.percent > 100) return 'var(--destructive)'
  if (props.percent >= 80) return 'var(--s2a-attn-bg)'
  return 'var(--brand)'
})

const inkColor = computed(() => {
  if (props.unset) return 'var(--muted-foreground)'
  if (props.percent > 100) return 'var(--destructive)'
  if (props.percent >= 80) return 'var(--s2a-attn)'
  return 'var(--body-copy)'
})

const ticks = computed(() =>
  Array.from({ length: segmentCount.value }, (_, i) => (i < litCount.value ? fillColor.value : 'var(--muted)'))
)
</script>

<template>
  <div class="cap-bar" :data-size="size">
    <div v-if="label || trailing" class="cap-bar__head">
      <span v-if="label" class="cap-bar__label">{{ label }}</span>
      <span v-if="trailing" class="cap-bar__trailing" :style="{ color: inkColor }">{{ trailing }}</span>
    </div>

    <div
      class="cap-bar__meter"
      role="progressbar"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-valuenow="Math.round(clamped)"
      :aria-label="label || undefined"
    >
      <div v-if="size === 'sm'" class="cap-bar__track">
        <div class="cap-bar__fill" :style="{ width: clamped + '%', background: fillColor }" />
      </div>
      <div v-else class="cap-bar__ticks">
        <span v-for="(color, i) in ticks" :key="i" class="cap-bar__tick" :style="{ background: color }" />
      </div>
    </div>

    <div v-if="$slots.default" class="cap-bar__foot">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.cap-bar {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.cap-bar__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 6px;
}

.cap-bar__label {
  font-size: var(--fs-md);
  color: var(--foreground);
}

.cap-bar__trailing {
  font-size: var(--fs-sm);
}

.cap-bar__meter {
  min-width: 0;
}

/* Ticks: 2px gaps, each an equal flex share, 1px corners. Matches the
   ticks/tick idiom already used for platform breakdown bars elsewhere in
   the app rather than inventing a second one. */
.cap-bar__ticks {
  display: flex;
  gap: 2px;
  align-items: stretch;
}
.cap-bar[data-size='lg'] .cap-bar__ticks {
  height: 13px;
}
.cap-bar[data-size='md'] .cap-bar__ticks {
  height: 11px;
}

.cap-bar__tick {
  flex: 1 1 0;
  min-width: 0;
  border-radius: 1px;
}

/* sm is the one size with no room for ticks: a 6px continuous fill instead,
   the treatment part 03 originally specified for every size. */
.cap-bar__track {
  height: 6px;
  border-radius: 999px;
  background: var(--muted);
  overflow: hidden;
}
.cap-bar__fill {
  height: 100%;
  border-radius: 999px;
  /* Width only, never border-color. --t-slow collapses to 0ms under
     prefers-reduced-motion at the token level, so no extra media query. */
  transition: width var(--t-slow) var(--ease-out);
}

.cap-bar__foot {
  margin-top: 8px;
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
</style>
