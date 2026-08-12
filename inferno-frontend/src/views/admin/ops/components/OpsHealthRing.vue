<script setup lang="ts">
/**
 * The health score, as a ring.
 *
 * Extracted from OpsDashboardHeader, where it was ~60 lines of inline SVG with
 * four hardcoded hexes (#10b981, #f59e0b, #ef4444, #9ca3af) and a parallel set
 * of Tailwind text classes that had to be kept in sync with them by hand. One
 * tone now drives the arc, the number and the condition line together, so they
 * cannot disagree.
 *
 * HEALTHY IS BRAND, NOT GREEN
 * Conventional would be green, but this page already establishes "good is
 * brand" -- the resource meters fill brand when normal, the traffic split
 * paints succeeded in brand. A lone green ring would read as a different
 * system's status light. Warning and critical keep the real state colours,
 * because those are the ones that need to be unmistakable.
 *
 * The arc is drawn from a dash offset rather than an arc path: a stroked
 * circle with stroke-linecap round gives the rounded ends for free, and
 * animating one number is cheaper and smoother than rebuilding a path.
 *
 * THE BLOCK IS `hring`, NOT `ring`
 * `ring` is a Tailwind utility, and Tailwind is still in this build. Naming
 * the block `ring` meant every instance also matched `.ring`, which sets
 * `box-shadow: var(--tw-ring-shadow)` -- and with no ring colour set, the
 * default is blue-500/50. The result was a 3px blue box painted around the
 * health score on every page load, in a design system that has no blue.
 * Scoped CSS does not protect against this: the scope attribute narrows OUR
 * rule, it does not stop a global utility of the same name from matching.
 * june-lint now rejects bare Tailwind utility names as block names.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

const props = withDefaults(
  defineProps<{
    score: number | null
    idle?: boolean
    fullscreen?: boolean
  }>(),
  { idle: false, fullscreen: false }
)

const { t } = useI18n()

type Tone = 'idle' | 'ok' | 'warning' | 'critical'

const tone = computed<Tone>(() => {
  if (props.idle || props.score == null) return 'idle'
  if (props.score >= 90) return 'ok'
  if (props.score >= 60) return 'warning'
  return 'critical'
})

const size = computed(() => (props.fullscreen ? 140 : 100))
const strokeWidth = computed(() => (props.fullscreen ? 10 : 8))
const radius = computed(() => (size.value - strokeWidth.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)

/* An idle system shows an empty track rather than a full ring: nothing has
   happened, which is not the same as everything being fine. */
const dashOffset = computed(() => {
  if (props.idle || props.score == null) return circumference.value
  const clamped = Math.max(0, Math.min(100, props.score))
  return circumference.value - (clamped / 100) * circumference.value
})

const display = computed(() => {
  if (props.idle) return t('admin.ops.idleStatus')
  return props.score == null ? '--' : String(props.score)
})

const conditionLabel = computed(() => {
  if (props.idle) return t('admin.ops.idleStatus')
  if (props.score != null && props.score >= 90) return t('admin.ops.healthyStatus')
  return t('admin.ops.riskyStatus')
})
</script>

<template>
  <div class="hring" :data-tone="tone">
    <div class="hring__dial" :style="{ width: `${size}px`, height: `${size}px` }">
      <svg :width="size" :height="size" class="hring__svg" aria-hidden="true">
        <circle
          class="hring__track"
          :cx="size / 2"
          :cy="size / 2"
          :r="radius"
          :stroke-width="strokeWidth"
          fill="transparent"
        />
        <circle
          class="hring__arc"
          :cx="size / 2"
          :cy="size / 2"
          :r="radius"
          :stroke-width="strokeWidth"
          fill="transparent"
          stroke-linecap="round"
          :stroke-dasharray="circumference"
          :stroke-dashoffset="dashOffset"
        />
      </svg>
      <div class="hring__centre">
        <span class="hring__score" :data-large="fullscreen || undefined">{{ display }}</span>
        <span class="hring__caption">{{ t('admin.ops.health') }}</span>
      </div>
    </div>

    <div v-if="!fullscreen" class="hring__condition">
      <span class="hring__condition-label">
        {{ t('admin.ops.healthCondition') }}
        <HelpTooltip :content="t('admin.ops.healthHelp')" />
      </span>
      <span class="hring__condition-value">{{ conditionLabel }}</span>
    </div>
  </div>
</template>

<style scoped>
.hring {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.hring__dial {
  position: relative;
  display: grid;
  place-items: center;
}

/* Rotated so the arc starts at 12 o'clock rather than 3. */
.hring__svg {
  transform: rotate(-90deg);
}

.hring__track {
  stroke: var(--muted);
}

/*
 * One tone drives the arc, the number and the condition line. They were three
 * separate colour expressions before, kept in sync by hand.
 */
.hring[data-tone='idle'] .hring__arc {
  stroke: var(--muted-foreground);
}
.hring[data-tone='ok'] .hring__arc {
  stroke: var(--brand);
}
.hring[data-tone='warning'] .hring__arc {
  stroke: var(--s2a-attn);
}
.hring[data-tone='critical'] .hring__arc {
  stroke: var(--destructive);
}

.hring__arc {
  /* Only the offset animates -- never a colour or a width. */
  transition: stroke-dashoffset var(--t-slow) var(--ease-out);
}

.hring__centre {
  position: absolute;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.hring__score {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.hring__score[data-large] {
  font-size: calc(var(--fs-display) * 1.6);
}

.hring[data-tone='idle'] .hring__score {
  color: var(--muted-foreground);
}
.hring[data-tone='warning'] .hring__score {
  color: var(--s2a-attn);
}
.hring[data-tone='critical'] .hring__score {
  color: var(--destructive);
}

/* Sentence case: "HEALTH" was shouting a label (ground rule 1). */
.hring__caption {
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.hring__condition {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  text-align: center;
}

.hring__condition-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.hring__condition-value {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.hring[data-tone='idle'] .hring__condition-value {
  color: var(--muted-foreground);
}
.hring[data-tone='warning'] .hring__condition-value {
  color: var(--s2a-attn);
}
.hring[data-tone='critical'] .hring__condition-value {
  color: var(--destructive);
}
</style>
