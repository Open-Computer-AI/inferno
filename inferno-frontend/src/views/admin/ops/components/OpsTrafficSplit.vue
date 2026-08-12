<script setup lang="ts">
/**
 * How this window's requests actually landed, as one split bar.
 *
 * WHAT THE TWO OLD CARDS HID
 * The header showed "SLA 88.050%" and "Request errors 11.95%" side by side as
 * if they were unrelated readings, and left a third number -- business limited
 * -- in small print under one of them. They are three parts of ONE total, and
 * crucially the first two do NOT sum to 100% whenever the third is non-zero,
 * because business limits are deliberately excluded from the SLA denominator.
 *
 * Two cards make a reader do that arithmetic to notice. One bar makes it
 * impossible to miss: the three segments are the whole of the traffic, and the
 * neutral one is visibly carved out of the total rather than hidden in a
 * footnote.
 *
 * Business limits take the neutral tone on purpose. They are not failures --
 * they are the gateway doing its job, refusing a request that exceeded a quota
 * -- so painting them with the error colour would report a working system as a
 * broken one.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import SplitBar, { type SplitSegment } from '@/components/charts/SplitBar.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

const props = defineProps<{
  successCount: number
  errorCountSla: number
  businessLimitedCount: number
  /** 0 to 1, as the API reports it. */
  sla: number | null
  fullscreen?: boolean
}>()

const emit = defineEmits<{ (e: 'openDetails'): void }>()

const { t } = useI18n()

const segments = computed<SplitSegment[]>(() => [
  {
    key: 'success',
    label: t('admin.ops.traffic.succeeded'),
    value: Math.max(0, props.successCount),
    tone: 'good'
  },
  {
    key: 'failed',
    label: t('admin.ops.traffic.failed'),
    value: Math.max(0, props.errorCountSla),
    tone: 'bad'
  },
  {
    key: 'limited',
    label: t('admin.ops.traffic.limited'),
    value: Math.max(0, props.businessLimitedCount),
    tone: 'neutral'
  }
])

const hasTraffic = computed(() => segments.value.some((s) => s.value > 0))

/* Kept as the headline because SLA is the number people are held to, and it is
   NOT simply the success share once business limits are excluded. */
const slaDisplay = computed(() =>
  props.sla == null ? '-' : `${(props.sla * 100).toFixed(3)}%`
)
</script>

<template>
  <div class="traffic">
    <div class="traffic__head">
      <span class="traffic__label">
        {{ t('admin.ops.traffic.title') }}
        <HelpTooltip v-if="!fullscreen" :content="t('admin.ops.tooltips.sla')" />
      </span>
      <button v-if="!fullscreen" class="traffic__action" type="button" @click="emit('openDetails')">
        {{ t('admin.ops.requestDetails.details') }}
      </button>
    </div>

    <p class="traffic__sla">
      {{ slaDisplay }}
      <span class="traffic__sla-name">{{ t('admin.ops.traffic.slaCaption') }}</span>
    </p>

    <SplitBar
      v-if="hasTraffic"
      :segments="segments"
      :unit="t('admin.ops.traffic.unit')"
      :caption="t('admin.ops.traffic.caption')"
    />
    <p v-else class="traffic__empty">{{ t('admin.ops.noData') }}</p>
  </div>
</template>

<style scoped>
.traffic {
  padding: 16px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.traffic__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.traffic__label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.traffic__action {
  border: 0;
  background: none;
  color: var(--brand);
  font-size: var(--fs-sm);
  cursor: pointer;
}

.traffic__action:hover {
  text-decoration: underline;
}

.traffic__sla {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 4px 0 14px;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-display);
  font-weight: 400;
  line-height: 1.1;
}

.traffic__sla-name {
  color: var(--muted-foreground);
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
}

.traffic__empty {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}
</style>
