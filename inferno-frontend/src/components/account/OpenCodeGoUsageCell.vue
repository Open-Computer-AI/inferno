<script setup lang="ts">
/**
 * OpenCodeGoUsageCell keeps the OpenCode Go usage windows in the same compact
 * ratio treatment as the June Ollama cell. The backend may mount this state
 * on an OpenCode account or on a compatible API-key account, so rendering is
 * driven by the explicit eligibility flag rather than a display name.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, OpenCodeGoUsageState } from '@/types'
import { adminAPI } from '@/api/admin'
import CapacityBar from '@/components/common/CapacityBar.vue'
import { formatCountdown } from '@/utils/format'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ updated: [state: OpenCodeGoUsageState] }>()
const { t } = useI18n()

const localState = ref(props.account.opencode_go_usage)
watch(() => props.account.opencode_go_usage, (next) => { localState.value = next })
const state = computed(() => localState.value)
const snapshot = computed(() => state.value?.snapshot)
const refreshing = ref(false)

const thresholdInk = (percent: number) => {
  if (percent > 100) return 'var(--destructive)'
  if (percent >= 80) return 'var(--s2a-attn)'
  return 'var(--foreground)'
}

const percentLabel = (percent: number) => `${Math.round(percent)}%`
const resetLabel = (resetsAt: string | undefined) => {
  const countdown = formatCountdown(resetsAt)
  return countdown ? t('userSubscriptions.resetIn', { time: countdown }) : ''
}

const statusLabel = computed(() => {
  if (!snapshot.value) return t('admin.accounts.opencodeGo.notRefreshed')
  if (snapshot.value.status === 'unauthorized') return t('admin.accounts.opencodeGo.unauthorized')
  if (snapshot.value.status === 'failed') return t('admin.accounts.opencodeGo.failed')
  return t('admin.accounts.opencodeGo.ok')
})

const refreshUsage = async () => {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const next = await adminAPI.accounts.refreshOpenCodeGoUsage(props.account.id)
    localState.value = next
    emit('updated', next)
  } catch (error) {
    console.error('Failed to refresh OpenCode Go usage:', error)
  } finally {
    refreshing.value = false
  }
}
</script>

<template>
  <div v-if="state?.eligible" class="ocgu" data-testid="opencode-go-usage-cell">
    <div v-if="snapshot?.data?.rolling" class="ocgu__row" data-testid="opencode-go-rolling">
      <span class="ocgu__label">{{ t('admin.accounts.opencodeGo.rollingShort') }}</span>
      <CapacityBar :percent="snapshot.data.rolling.percent" size="sm" class="ocgu__bar" />
      <span class="ocgu__percent" :style="{ color: thresholdInk(snapshot.data.rolling.percent) }">
        {{ percentLabel(snapshot.data.rolling.percent) }}
      </span>
      <span v-if="resetLabel(snapshot.data.rolling.resets_at)" class="ocgu__reset">
        {{ resetLabel(snapshot.data.rolling.resets_at) }}
      </span>
    </div>
    <div v-if="snapshot?.data?.weekly" class="ocgu__row" data-testid="opencode-go-weekly">
      <span class="ocgu__label">{{ t('admin.accounts.opencodeGo.weeklyShort') }}</span>
      <CapacityBar :percent="snapshot.data.weekly.percent" size="sm" class="ocgu__bar" />
      <span class="ocgu__percent" :style="{ color: thresholdInk(snapshot.data.weekly.percent) }">
        {{ percentLabel(snapshot.data.weekly.percent) }}
      </span>
      <span v-if="resetLabel(snapshot.data.weekly.resets_at)" class="ocgu__reset">
        {{ resetLabel(snapshot.data.weekly.resets_at) }}
      </span>
    </div>
    <div v-if="snapshot?.data?.monthly" class="ocgu__row" data-testid="opencode-go-monthly">
      <span class="ocgu__label">{{ t('admin.accounts.opencodeGo.monthlyShort') }}</span>
      <CapacityBar :percent="snapshot.data.monthly.percent" size="sm" class="ocgu__bar" />
      <span class="ocgu__percent" :style="{ color: thresholdInk(snapshot.data.monthly.percent) }">
        {{ percentLabel(snapshot.data.monthly.percent) }}
      </span>
      <span v-if="resetLabel(snapshot.data.monthly.resets_at)" class="ocgu__reset">
        {{ resetLabel(snapshot.data.monthly.resets_at) }}
      </span>
    </div>
    <span
      v-if="snapshot && snapshot.status !== 'ok'"
      class="ocgu__status"
      :data-tone="snapshot.status === 'unauthorized' ? 'attn' : 'danger'"
      data-testid="opencode-go-status-badge"
    >{{ statusLabel }}</span>
    <div class="ocgu__actions">
      <button
        type="button"
        class="ocgu__query"
        :disabled="refreshing"
        data-testid="opencode-go-usage-query"
        @click="refreshUsage"
      >
        <i
          class="hgi-stroke hgi-refresh-01 ocgu__query-icon"
          :class="{ 'ocgu__query-icon--spin': refreshing }"
          aria-hidden="true"
        />
        {{ t('admin.accounts.usageWindow.activeQuery') }}
      </button>
    </div>
  </div>
  <span v-else class="ocgu__empty">-</span>
</template>

<style scoped>
.ocgu {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.ocgu__row {
  display: flex;
  align-items: center;
  gap: 5px;
  min-width: 0;
}

.ocgu__label {
  flex-shrink: 0;
  width: 24px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.ocgu__bar {
  width: 40px;
  flex-shrink: 0;
}

.ocgu__percent {
  flex-shrink: 0;
  width: 28px;
  font-size: var(--fs-2xs);
}

.ocgu__reset {
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ocgu__status {
  align-self: flex-start;
  border-radius: var(--r-xs);
  padding: 1px 5px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  background: var(--surface-subtle);
}

.ocgu__status[data-tone='attn'] {
  background: var(--s2a-attn-soft);
  color: var(--s2a-attn);
}

.ocgu__status[data-tone='danger'] {
  background: var(--destructive-soft);
  color: var(--destructive);
}

.ocgu__actions {
  display: flex;
  align-items: center;
  padding-top: 2px;
}

.ocgu__query {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  border-radius: var(--r-sm);
  padding: 2px 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
}

.ocgu__query:hover:not(:disabled) {
  background: var(--brand-tint);
  color: var(--brand);
}

.ocgu__query:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.ocgu__query-icon--spin {
  animation: ocgu-spin 1s linear infinite;
}

@keyframes ocgu-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .ocgu__query-icon--spin {
    animation: none;
  }
}

.ocgu__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}
</style>
