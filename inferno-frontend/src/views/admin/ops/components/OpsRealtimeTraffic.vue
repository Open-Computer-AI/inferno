<script setup lang="ts">
/**
 * Live QPS and TPS, with the window selector.
 *
 * Extracted from OpsDashboardHeader and converted. Three things changed beyond
 * the tokens.
 *
 * THE FAKE EKG IS GONE
 * The block ended with an "Animated Pulse Line": a <path> with an SMIL
 * <animate> cycling three hardcoded `d` values. It represented nothing. On a
 * monitoring dashboard that is worse than ordinary decoration -- it looks
 * exactly like a live traffic waveform and sat directly under the real QPS
 * figure, so it invited being read as data. It also ran forever with no way to
 * honour prefers-reduced-motion, since SMIL cannot be gated by a media query.
 *
 * THE WINDOW SELECTOR IS THE REAL CONTROL
 * It was six hand-rolled buttons toggling `bg-blue-500 text-white` against
 * `bg-gray-200`. Segmented already does that, with a sliding indicator,
 * roving-tabindex keyboard support and radiogroup semantics -- none of which
 * the hand-rolled version had.
 *
 * THE LIVE DOT NO LONGER PINGS
 * `animate-ping` is an infinite CSS animation with no reduced-motion guard.
 * A static dot says "live" just as well; the numbers updating is the actual
 * evidence.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Segmented from '@/components/common/Segmented.vue'

const props = defineProps<{
  window: string
  windows: readonly string[]
  qps: number
  tps: number
  qpsPeak: string
  tpsPeak: string
  qpsAvg: string
  tpsAvg: string
  fullscreen?: boolean
}>()

const emit = defineEmits<{ (e: 'update:window', value: string): void }>()

const { t } = useI18n()

const windowItems = computed(() => props.windows.map((w) => ({ value: w, label: w })))
</script>

<template>
  <div class="rt" :data-large="fullscreen || undefined">
    <div class="rt__head">
      <span class="rt__title">
        <span class="rt__dot" aria-hidden="true" />
        {{ t('admin.ops.realtime.title') }}
        <HelpTooltip v-if="!fullscreen" :content="t('admin.ops.tooltips.qps')" />
      </span>
      <Segmented
        :model-value="window"
        :items="windowItems"
        :aria-label="t('admin.ops.realtime.title')"
        @update:model-value="emit('update:window', $event)"
      />
    </div>

    <div class="rt__now">
      <p class="rt__label">{{ t('admin.ops.current') }}</p>
      <div class="rt__pair">
        <span class="rt__figure">{{ qps.toFixed(1) }}<span class="rt__unit">QPS</span></span>
        <span class="rt__figure">{{ tps.toFixed(1) }}<span class="rt__unit">{{ t('admin.ops.tps') }}</span></span>
      </div>
    </div>

    <div class="rt__cols">
      <div>
        <p class="rt__label">{{ t('admin.ops.peak') }}</p>
        <div class="rt__rows">
          <span class="rt__row">{{ qpsPeak }}<span class="rt__unit">QPS</span></span>
          <span class="rt__row">{{ tpsPeak }}<span class="rt__unit">{{ t('admin.ops.tps') }}</span></span>
        </div>
      </div>
      <div>
        <p class="rt__label">{{ t('admin.ops.average') }}</p>
        <div class="rt__rows">
          <span class="rt__row">{{ qpsAvg }}<span class="rt__unit">QPS</span></span>
          <span class="rt__row">{{ tpsAvg }}<span class="rt__unit">{{ t('admin.ops.tps') }}</span></span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.rt {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: 100%;
  justify-content: center;
  padding: 8px 0;
}

.rt__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.rt__title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

/* Static. The old one was animate-ping -- an infinite animation with no
   reduced-motion guard, saying "live" less convincingly than the numbers
   updating do. */
.rt__dot {
  width: 8px;
  height: 8px;
  border-radius: var(--r-pill);
  background: var(--brand);
}

.rt__label {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.rt__pair {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 20px;
  margin-top: 4px;
}

.rt__figure {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.rt[data-large] .rt__figure {
  font-size: var(--fs-display);
}

.rt__unit {
  margin-left: 6px;
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

.rt__cols {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.rt__rows {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 4px;
}

/* No bold. Every figure here was font-black before, which is weight spent on
   nothing -- if all of them shout, none of them is emphasised. */
.rt__row {
  color: var(--foreground);
  font-size: var(--fs-md);
  font-variant-numeric: tabular-nums;
}
</style>
