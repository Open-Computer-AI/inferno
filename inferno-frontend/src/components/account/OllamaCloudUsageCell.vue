<script setup lang="ts">
/**
 * OllamaCloudUsageCell — table cells part 08, kind C ("Ratio").
 *
 * Migration note from the prototype (restCells, "OllamaCloudUsageCell"):
 *   "Correct as built. Only the fill token changes."
 *
 * "As built" kept both windows (5h and 7d) stacked, which this keeps. What
 * changes: the old UsageProgressBar coloured the two windows' label chips
 * indigo vs emerald — colour by which window it is, a category, not a
 * state (ground rule 5). It also hard-coded Tailwind red/amber/green for
 * the fill rather than using this app's threshold tokens, and it is not a
 * June component. CapacityBar already is: it is composed here directly
 * (sm, 6px, continuous — the table-cell size), one instance per window,
 * both sharing the one threshold rule every ratio cell in this part uses.
 * The reset countdown drops UsageProgressBar's per-minute ticking clock (an
 * interval per row, for a table column) in favour of a value computed at
 * render time from the account's own resets_at — this is not a live-ticking
 * value in a fixed-width slot, so it does not need tabular-nums or a timer.
 *
 * Prop is unchanged (`account: Account`).
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import CapacityBar from '@/components/common/CapacityBar.vue'
import { adminAPI } from '@/api/admin'
import type { OllamaCloudUsageState } from '@/types'
import { formatCountdown } from '@/utils/format'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OllamaCloudUsageState] }>()
const { t } = useI18n()

/* Local copy so an on-demand query updates this cell immediately; it follows
   the prop whenever the row is refreshed from the list. */
const localState = ref(props.account.ollama_cloud_usage)
watch(() => props.account.ollama_cloud_usage, (next) => { localState.value = next })
const state = computed(() => localState.value)
const refreshing = ref(false)

const refreshUsage = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const next = await adminAPI.accounts.refreshOllamaCloudUsage(props.account.id)
    localState.value = next
    emit('updated', next)
  } catch (error) {
    console.error('Failed to refresh Ollama Cloud usage:', error)
  } finally {
    refreshing.value = false
  }
}
const snapshot = computed(() => state.value?.snapshot)

const thresholdInk = (percent: number) => {
  if (percent > 100) return 'var(--destructive)'
  if (percent >= 80) return 'var(--s2a-attn)'
  return 'var(--foreground)'
}

const resetLabel = (resetsAt: string | null | undefined) => {
  const countdown = formatCountdown(resetsAt)
  return countdown ? t('misc.resetIn', { time: countdown }) : ''
}
</script>

<template>
  <div v-if="state?.eligible" class="ocu" data-testid="ollama-cloud-usage-cell">
    <div v-if="snapshot?.data?.five_hour" class="ocu__row" data-testid="ollama-cloud-five-hour">
      <span class="ocu__label">{{ t('admin.accounts.ollamaCloud.fiveHourShort') }}</span>
      <CapacityBar :percent="snapshot.data.five_hour.used_percent" size="sm" class="ocu__bar" />
      <span class="ocu__percent" :style="{ color: thresholdInk(snapshot.data.five_hour.used_percent) }">
        {{ Math.round(snapshot.data.five_hour.used_percent) }}%
      </span>
      <span v-if="resetLabel(snapshot.data.five_hour.reset_at)" class="ocu__reset">{{ resetLabel(snapshot.data.five_hour.reset_at) }}</span>
    </div>
    <div v-if="snapshot?.data?.seven_day" class="ocu__row" data-testid="ollama-cloud-seven-day">
      <span class="ocu__label">{{ t('admin.accounts.ollamaCloud.sevenDayShort') }}</span>
      <CapacityBar :percent="snapshot.data.seven_day.used_percent" size="sm" class="ocu__bar" />
      <span class="ocu__percent" :style="{ color: thresholdInk(snapshot.data.seven_day.used_percent) }">
        {{ Math.round(snapshot.data.seven_day.used_percent) }}%
      </span>
      <span v-if="resetLabel(snapshot.data.seven_day.reset_at)" class="ocu__reset">{{ resetLabel(snapshot.data.seven_day.reset_at) }}</span>
    </div>
    <div v-if="state?.configured" class="ocu__actions">
      <button
        type="button"
        class="ocu__query"
        :disabled="refreshing"
        data-testid="ollama-cloud-usage-query"
        @click="refreshUsage"
      >
        <i
          class="hgi-stroke hgi-refresh ocu__query-icon"
          :class="{ 'ocu__query-icon--spin': refreshing }"
          aria-hidden="true"
        />
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>
    </div>
  </div>
  <span v-else class="ocu__empty">-</span>
</template>

<style scoped>
.ocu {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.ocu__row {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.ocu__label {
  flex-shrink: 0;
  width: 16px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.ocu__bar {
  width: 40px;
  flex-shrink: 0;
}

.ocu__actions {
  display: flex;
  align-items: center;
  padding-top: 2px;
}

/* Neutral at rest; the brand tint is an interaction state, not a category. */
.ocu__query {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  border-radius: var(--r-sm);
  padding: 2px 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.ocu__query:hover:not(:disabled) {
  background: var(--brand-tint);
  color: var(--brand);
}

.ocu__query:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ocu__query-icon--spin {
  animation: ocu-spin 1s linear infinite;
}

@keyframes ocu-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .ocu__query-icon--spin {
    animation: none;
  }
}

.ocu__percent {
  flex-shrink: 0;
  width: 28px;
  font-size: var(--fs-2xs);
}

.ocu__reset {
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ocu__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
</style>
