<template>
  <div class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />
      <button
        v-if="visible"
        type="button"
        data-testid="claude-reset-count"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || redeeming"
        :title="countButtonTitle"
        @click="refresh"
      >
        <i class="hgi-stroke hgi-refresh-01" :class="{ 's2a-spinner': loading }" aria-hidden="true" />
        {{ t('admin.accounts.claudeResetCredits.count') }}<span v-if="status" class="ml-0.5 tabular-nums">{{ totalResets }}</span>
      </button>
      <button
        v-if="visible"
        type="button"
        data-testid="claude-reset-redeem"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="redeeming || loading || !canRedeem"
        :title="redeemButtonTitle"
        @click="openRedeemConfirm"
      >
        <i class="hgi-stroke hgi-reload-01" :class="{ 's2a-spinner': redeeming }" aria-hidden="true" />
        {{ t('admin.accounts.claudeResetCredits.reset') }}
      </button>
    </div>
    <div v-if="visible && status && (primaryCredit || !status.eligible || cooldownActive)" class="flex flex-wrap items-center gap-1">
      <span v-if="primaryCredit?.expires_at" data-testid="claude-reset-expiry" class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300" :title="creditTitle">
        {{ t('admin.accounts.claudeResetCredits.expiresAt', { time: formatTime(primaryCredit.expires_at, 'short') }) }}
      </span>
      <span v-if="status.eligible && totalResets > 0 && status.available_count === 0" data-testid="claude-reset-not-usable" class="text-[10px] text-gray-500 dark:text-gray-400">{{ t('admin.accounts.claudeResetCredits.notUsableNow') }}</span>
      <span v-if="!status.eligible" class="text-[10px] text-amber-600 dark:text-amber-400">{{ t('admin.accounts.claudeResetCredits.ineligible') }}</span>
      <span v-if="cooldownActive" data-testid="claude-reset-cooldown" class="text-[10px] text-amber-600 dark:text-amber-400">{{ t('admin.accounts.claudeResetCredits.cooldown', { time: formatTime(status.cooldown_until!, 'short') }) }}</span>
    </div>
    <div v-if="visible && error" role="alert" class="text-[10px] text-red-600 dark:text-red-400">{{ t('admin.accounts.claudeResetCredits.error') }}</div>
    <div v-if="visible && redeemFeedback" data-testid="claude-reset-feedback" :role="redeemFeedback.kind === 'success' ? 'status' : 'alert'" class="text-[10px]" :class="feedbackClass" :title="redeemFeedback.text">{{ redeemFeedback.text }}</div>
    <ConfirmDialog
      v-if="visible"
      :show="showRedeemConfirm"
      :title="t('admin.accounts.claudeResetCredits.confirmTitle')"
      :message="confirmMessage"
      :confirm-text="t('admin.accounts.claudeResetCredits.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmRedeem"
      @cancel="showRedeemConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { getClaudeResetCredits, redeemClaudeResetCredit, type ClaudeResetCredits, type ClaudeResetOutcome } from '@/api/admin/claudeResetCredits'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{ account: Account }>()
const emit = defineEmits<{ redeemed: [outcome: ClaudeResetOutcome] }>()
const { t } = useI18n()
const status = ref<ClaudeResetCredits | null>(null)
const loading = ref(false)
const error = ref(false)
const redeeming = ref(false)
const showRedeemConfirm = ref(false)
const redeemFeedback = ref<{ kind: 'success' | 'warning' | 'error'; text: string } | null>(null)
let pendingKey: string | null = null
let generation = 0

const visible = computed(() => props.account.platform === 'anthropic' && props.account.type === 'oauth')
watch(() => [props.account.id, props.account.platform, props.account.type], () => {
  generation++
  status.value = null
  loading.value = false
  error.value = false
  redeeming.value = false
  showRedeemConfirm.value = false
  redeemFeedback.value = null
  pendingKey = null
})

const formatTime = (value: string, style: 'short' | 'full') => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const options: Intl.DateTimeFormatOptions = { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
  if (style === 'full') options.year = 'numeric'
  return new Intl.DateTimeFormat(undefined, options).format(date)
}
const totalResets = computed(() => (status.value?.credits ?? []).reduce((sum, credit) => sum + credit.resets_left, 0))
const expiryMs = (value?: string) => {
  const ms = value ? new Date(value).getTime() : NaN
  return Number.isNaN(ms) ? Number.POSITIVE_INFINITY : ms
}
const primaryCredit = computed(() => {
  const credits = status.value?.credits ?? []
  return credits.find(credit => credit.redeemable) ?? [...credits].sort((a, b) => expiryMs(a.expires_at) - expiryMs(b.expires_at))[0] ?? null
})
const cooldownActive = computed(() => {
  const until = status.value?.cooldown_until
  return Boolean(until && new Date(until).getTime() > Date.now())
})
const windowKeys: Record<string, string> = { five_hour: 'fiveHour', seven_day: 'sevenDay', seven_day_overage_included: 'sevenDayOverage' }
const windowLabels = (windows?: string[]) => (windows ?? []).map(window => windowKeys[window] ? t(`admin.accounts.claudeResetCredits.windows.${windowKeys[window]}`) : window).join(', ')
const creditTitle = computed(() => {
  const credit = primaryCredit.value
  if (!credit) return ''
  const lines = [credit.label]
  if (credit.expires_at) lines.push(t('admin.accounts.claudeResetCredits.expiresAtFull', { time: formatTime(credit.expires_at, 'full') }))
  if (credit.clears.length) lines.push(t('admin.accounts.claudeResetCredits.clears', { windows: windowLabels(credit.clears) }))
  if (credit.use_requires_limit) lines.push(t('admin.accounts.claudeResetCredits.requiresLimit'))
  return lines.join('\n')
})
const countButtonTitle = computed(() => !status.value
  ? t('admin.accounts.claudeResetCredits.countTooltipLoad')
  : [t('admin.accounts.claudeResetCredits.countTooltipRefresh'), t('admin.accounts.claudeResetCredits.fetched', { time: formatTime(status.value.fetched_at, 'full') })].join('\n'))
const canRedeem = computed(() => (status.value?.available_count ?? 0) > 0)
const redeemButtonTitle = computed(() => !status.value
  ? t('admin.accounts.claudeResetCredits.resetTooltipNeedQuery')
  : canRedeem.value ? t('admin.accounts.claudeResetCredits.resetTooltipReady') : t('admin.accounts.claudeResetCredits.resetTooltipNone'))
const confirmMessage = computed(() => t('admin.accounts.claudeResetCredits.confirmMessage', {
  windows: windowLabels(status.value?.credits.find(credit => credit.redeemable)?.clears) || '—',
  count: Math.max(totalResets.value - 1, 0)
}))
const feedbackClass = computed(() => redeemFeedback.value?.kind === 'success' ? 'text-emerald-600 dark:text-emerald-400' : redeemFeedback.value?.kind === 'warning' ? 'text-amber-600 dark:text-amber-400' : 'text-red-600 dark:text-red-400')
const newOperationKey = (accountID: number) => `claude-reset-${accountID}-${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
const openRedeemConfirm = () => { if (!redeeming.value && !loading.value && canRedeem.value) showRedeemConfirm.value = true }
const outcomeFeedback = (result: ClaudeResetOutcome) => {
  const key = 'admin.accounts.claudeResetCredits.outcome'
  switch (result.outcome) {
    case 'reset': return { kind: 'success' as const, text: t(`${key}.reset`, { windows: windowLabels(result.cleared) || '—' }) }
    case 'already_used': return { kind: 'warning' as const, text: t(`${key}.alreadyUsed`) }
    case 'cooldown': return { kind: 'warning' as const, text: result.cooldown_until ? t(`${key}.cooldownUntil`, { time: formatTime(result.cooldown_until, 'short') }) : t(`${key}.cooldown`) }
    case 'not_limited': return { kind: 'warning' as const, text: t(`${key}.notLimited`) }
    case 'ineligible': return { kind: 'error' as const, text: t(`${key}.ineligible`) }
    default: return { kind: 'warning' as const, text: result.reason === 'upstream_unavailable' ? t(`${key}.unavailable`) : t(`${key}.unknown`) }
  }
}
const preClaimRefusals = new Set(['CLAUDE_RESET_BUSY', 'CLAUDE_RESET_NOT_AVAILABLE', 'CLAUDE_RESET_UNRESOLVED', 'CLAUDE_RESET_UPSTREAM_UNAVAILABLE'])
const errorText = (value: unknown) => {
  const reason = (value as { reason?: string })?.reason
  const key = 'admin.accounts.claudeResetCredits.outcome'
  const mapped: Record<string, string> = {
    CLAUDE_RESET_UNRESOLVED: 'unknown', CLAUDE_RESET_BUSY: 'busy', CLAUDE_RESET_NOT_AVAILABLE: 'notAvailable',
    CLAUDE_RESET_UPSTREAM_UNAVAILABLE: 'unavailable', IDEMPOTENCY_IN_PROGRESS: 'inProgress', IDEMPOTENCY_RETRY_BACKOFF: 'retryBackoff'
  }
  return mapped[reason ?? ''] ? t(`${key}.${mapped[reason ?? '']}`) : (value as { message?: string })?.message || t(`${key}.failed`)
}
const refresh = async () => {
  if (loading.value || redeeming.value) return
  const current = ++generation
  loading.value = true
  error.value = false
  try {
    const result = await getClaudeResetCredits(props.account.id)
    if (current === generation) status.value = result
  } catch {
    if (current === generation) { error.value = true; status.value = null }
  } finally {
    if (current === generation) loading.value = false
  }
}
const confirmRedeem = async () => {
  showRedeemConfirm.value = false
  if (redeeming.value || loading.value || !canRedeem.value) return
  const accountID = props.account.id
  const current = generation
  pendingKey ??= newOperationKey(accountID)
  redeeming.value = true
  redeemFeedback.value = null
  try {
    const result = await redeemClaudeResetCredit(accountID, pendingKey)
    if (current !== generation) return
    pendingKey = null
    redeemFeedback.value = outcomeFeedback(result)
    emit('redeemed', result)
    redeeming.value = false
    await refresh()
  } catch (value) {
    if (current !== generation) return
    if (preClaimRefusals.has((value as { reason?: string })?.reason ?? '')) pendingKey = null
    redeemFeedback.value = { kind: 'error', text: errorText(value) }
  } finally {
    if (current === generation) redeeming.value = false
  }
}
</script>
