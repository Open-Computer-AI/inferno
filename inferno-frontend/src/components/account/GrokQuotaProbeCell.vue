<template>
  <div v-if="visible" class="gqp">
    <div class="gqp-row">
      <Button
        variant="ghost"
        size="xs"
        :icon="loading ? undefined : 'hgi-refresh-01'"
        :loading="loading"
        :disabled="loading"
        :title="t('admin.accounts.usageWindow.grokProbeTooltip')"
        @click="handleProbe"
      >
        {{ t('admin.accounts.usageWindow.grokProbe') }}
      </Button>
    </div>

    <!-- Compact mode: parent already shows 7d/30d/prepaid or 24h — only surface errors. -->
    <div v-if="!compact && summary" class="gqp-summary">
      {{ summary }}
    </div>
    <div v-if="error" class="gqp-error" :title="error">
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * GrokQuotaProbeCell — part 08, kind F ("action"). Implements the approved
 * UX departure "a cell never fetches" (part 08, "Three decisions, taken").
 *
 * This cell never fetched automatically: `handleProbe` only ever ran from
 * the button's click handler, so there was no per-render request to strip
 * out here (unlike the batched cells in the same departure). The bug the
 * departure targets was one level up: AccountUsageCell used to embed this
 * button inline on every visible Grok row, which normalizes a real billable
 * xAI call as an ordinary per-row action. That embedding is now removed
 * (see AccountUsageCell.vue's file header). This component's own contract
 * is unchanged: it still takes `account` and `compact`, and it is meant to
 * be composed wherever a probe affordance belongs -- today nowhere by
 * default, per the departure "off by default, and the switch says what
 * turning it on costs." See the build report for what an opt-in column
 * needs from the table that would host it.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { GrokQuotaProbeResult } from '@/api/admin/grok'
import type { Account } from '@/types'
import Button from '@/components/common/Button.vue'

const props = withDefaults(
  defineProps<{
    account: Account
    /** When true, only show the probe button (+ errors). No duplicate weekly summary. */
    compact?: boolean
  }>(),
  { compact: false }
)

const emit = defineEmits<{ probed: [result: GrokQuotaProbeResult] }>()

const { t } = useI18n()

const visible = computed(() => props.account.platform === 'grok' && props.account.type === 'oauth')
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<GrokQuotaProbeResult | null>(null)

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const summary = computed(() => {
  if (props.compact || !data.value) return ''
  // Non-compact fallback (rarely used): brief weekly percent if present.
  const billing = data.value.billing
  if (billing?.period_type?.toLowerCase() === 'weekly' && billing.usage_percent != null) {
    return t('admin.accounts.usageWindow.grokWeeklyUsage', {
      percent: Math.round(Math.min(100, Math.max(0, billing.usage_percent)))
    })
  }
  return ''
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    data.value = await adminAPI.grok.queryQuota(props.account.id)
    error.value = data.value.probe_error || null
    emit('probed', data.value)
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.account.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
  }
)
</script>

<style scoped>
.gqp {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.gqp-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.gqp-summary {
  font-size: var(--fs-sm);
  color: var(--body-copy);
}

.gqp-error {
  overflow: hidden;
  max-width: 220px;
  color: var(--destructive);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
