<template>
  <!--
    Kind D ("State"): the query/reset controls are the affordance the state
    is acted through, so they stay, but they are neutral chrome rather than
    permanently-blue/permanently-orange category colour (ground rule 5,
    colour is state, never category). Destructive tint on the reset control
    only appears on hover/focus, i.e. as an interaction state, not a resting
    paint. The text-node structure inside each button (the t() call, the
    conditional count span with its leading space) is untouched: the spec
    test for this file asserts on exact button order and rendered text.
  -->
  <div v-if="visible" class="oqr">
    <!--
      Unified action row. Parents that already render their own "local query"
      affordance (e.g. AccountUsageCell's active-sampling refresh) pass it in
      via the #pre-actions slot so the user sees a single row of related
      buttons rather than two near-duplicate "查询" rows.

      The 5h / 7d window bars are deliberately NOT rendered here — the local
      active-sampling display (UsageProgressBar in AccountUsageCell) already
      owns that real estate. This cell is purely about the rate-limit reset
      credit: query its count, consume one if needed.
    -->
    <div class="oqr__actions">
      <slot name="pre-actions" />

      <button
        type="button"
        class="oqr__btn"
        :disabled="loading || resetting"
        :title="countButtonTitle"
        @click="handleQuery()"
      >
        <i class="hgi-stroke hgi-refresh oqr__icon" :class="{ 'oqr__icon--spin': loading }" aria-hidden="true" />
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="data"> {{ availableResetCount }}</span>
      </button>

      <button
        type="button"
        class="oqr__btn"
        data-tone="reset"
        :disabled="resetting || loading || !canReset"
        :title="resetButtonTitle"
        @click="openResetConfirm"
      >
        <i class="hgi-stroke hgi-reload oqr__icon" :class="{ 'oqr__icon--spin': resetting }" aria-hidden="true" />
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>

    <div
      v-if="autoResetState"
      class="oqr__auto"
      data-testid="auto-reset-credit-state"
    >
      <span class="oqr__auto-chip" :class="autoResetStateModifier">
        {{ autoResetStateLabel }}
        <span v-if="autoResetState.trigger_window" class="oqr__auto-window">
          {{ autoResetState.trigger_window }}
        </span>
      </span>
      <span v-if="autoResetState.checked_at" class="oqr__auto-time">
        {{ formatResetCreditExpiry(autoResetState.checked_at, 'short') }}
      </span>
      <span
        v-if="autoResetState.error_code"
        class="oqr__auto-error"
        :title="autoResetState.error_code"
      >
        {{ autoResetState.error_code }}
      </span>
    </div>

    <div v-if="primaryResetCreditExpiry" class="oqr__expiry">
      <div class="oqr__expiry-row">
        <span
          class="oqr__chip"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'full') })"
        >
          {{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'short') }) }}
        </span>
        <button
          v-if="hiddenResetCreditCount > 0"
          type="button"
          data-testid="reset-credit-expiry-toggle"
          class="oqr__toggle"
          :aria-expanded="showResetCreditDetails"
          :aria-label="resetCreditDetailsToggleLabel"
          :title="resetCreditDetailsTitle"
          @click="toggleResetCreditDetails"
        >
          +{{ hiddenResetCreditCount }}
        </button>
      </div>

      <div
        v-if="showResetCreditDetails && resetCreditExpirations.length > 1"
        data-testid="reset-credit-expiry-details"
        class="oqr__details"
      >
        <span class="oqr__sr-only">{{ t('admin.accounts.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(expiresAt, index) in resetCreditExpirations"
          :key="`${expiresAt}-${index}`"
          class="oqr__detail-row"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(expiresAt, 'full') })"
        >
          <span class="oqr__detail-dot" />
          <span class="oqr__detail-text">{{ formatResetCreditExpiry(expiresAt, 'short') }}</span>
        </span>
      </div>
    </div>

    <!-- Error / success feedback -->
    <div v-if="error" class="oqr__feedback" data-tone="error" :title="error">
      {{ truncatedError }}
    </div>
    <div v-else-if="resetWarning" class="oqr__feedback" data-tone="warning">
      {{ resetWarning }}
    </div>
    <div v-else-if="resetMessage" class="oqr__feedback" data-tone="success">
      {{ resetMessage }}
    </div>

    <ConfirmDialog
      :show="showResetConfirm"
      :title="t('admin.accounts.openaiQuotaReset.confirmTitle')"
      :message="t('admin.accounts.openaiQuotaReset.confirmMessage', { count: availableResetCount })"
      :confirm-text="t('admin.accounts.openaiQuotaReset.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmReset"
      @cancel="showResetConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  refreshOpenAIQuota,
  resetOpenAIQuota,
  type OpenAIQuotaUsage,
  type OpenAIQuotaResetResult
} from '@/api/admin/accounts'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{
  account: Account
}>()

const emit = defineEmits<{
  'account-updated': [account: Account]
}>()

const { t } = useI18n()

// Visible only for OpenAI OAuth accounts.
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')

const loading = ref(false)
const resetting = ref(false)
const error = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const cachedData = ref<OpenAIQuotaUsage | null>(null)
const resetMessage = ref<string | null>(null)
const resetWarning = ref<string | null>(null)
const showResetConfirm = ref(false)
const showResetCreditDetails = ref(false)

// Rehydrate the card from the persisted snapshot. Credits that already expired
// are dropped and the count is clamped to what remains: the snapshot has no
// freshness signal, so an unfiltered read would offer to consume credits that no
// longer exist. A snapshot claiming credits with no usable expiration left is
// treated as absent, which keeps the reset button gated on a live query.
const readCachedResetCredits = (account: Account): OpenAIQuotaUsage | null => {
  const cached = account.extra?.codex_reset_credit_snapshot
  if (!cached || typeof cached !== 'object' || Array.isArray(cached)) return null

  const { available_count: count, credits: rawCredits } = cached as {
    available_count?: unknown
    credits?: unknown
  }
  if (typeof count !== 'number' || !Number.isFinite(count)) return null

  const now = Date.now()
  const credits: { expires_at?: string }[] = []
  if (Array.isArray(rawCredits)) {
    for (const credit of rawCredits) {
      if (!credit || typeof credit !== 'object') continue
      const expiresAt = (credit as { expires_at?: unknown }).expires_at
      if (typeof expiresAt !== 'string' || expiresAt.trim() === '') continue
      const expiryTime = new Date(expiresAt).getTime()
      // Unparsable timestamps are kept: they are already rendered verbatim and
      // dropping them would silently understate the available count.
      if (!Number.isNaN(expiryTime) && expiryTime <= now) continue
      credits.push({ expires_at: expiresAt })
    }
  }
  const availableCount = Math.min(Math.max(count, 0), credits.length)
  // A snapshot that claimed credits but has none left is no longer informative;
  // report "unknown" so the operator re-queries instead of trusting it.
  if (count > 0 && availableCount <= 0) return null
  return {
    fetched_at: 0,
    rate_limit_reset_credits: {
      available_count: availableCount,
      credits
    }
  }
}

cachedData.value = readCachedResetCredits(props.account)
data.value = cachedData.value

// 影子账号的额度查询会 resolve 到母账号,但影子本身不支持重置(后端返回 409);
// 重置必须在母账号上进行。前端据此禁用影子的重置入口(外审 F6)。
const isShadow = computed(() => props.account.parent_account_id != null)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
// Prefer the live payload and fall back to the persisted snapshot only when the
// live state is unknown, so the count and the expirations never come from two
// different generations of the same data.
const resetCreditExpirations = computed(() =>
  ((data.value ?? cachedData.value)?.rate_limit_reset_credits?.credits ?? [])
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareResetCreditExpiry)
)
const primaryResetCreditExpiry = computed(() => resetCreditExpirations.value[0] ?? '')

// Auto-reset-credit runtime state, written by the scheduler. Read-only here.
type AutoResetCreditState = NonNullable<NonNullable<Account['extra']>['codex_auto_reset_credit_state']>
const validAutoResetStatuses = new Set(['checking', 'available', 'resetting', 'success', 'no_credit', 'failed'])
/* Three guards, all load-bearing: an operator who turned auto-reset off must
   not keep seeing a chip driven by a stale state left in extra, and an unknown
   status must render nothing rather than fall through to a missing i18n key. */
const autoResetState = computed<AutoResetCreditState | null>(() => {
  if (props.account.extra?.auto_reset_credit_enabled !== true) return null
  const state = props.account.extra?.codex_auto_reset_credit_state
  if (!state || typeof state !== 'object' || !validAutoResetStatuses.has(String(state.status))) return null
  return state
})

const autoResetStateLabel = computed(() => {
  const status = autoResetState.value?.status
  return status ? t(`admin.accounts.openaiQuotaReset.autoStatus.${status === 'no_credit' ? 'noCredit' : status}`) : ''
})

/* Colour is STATE here, not category, so it is earned (ground rule 5). One
   modifier per status; the palette lives in the scoped block below. */
const autoResetStateModifier = computed(() => {
  switch (autoResetState.value?.status) {
    case 'available': return 'oqr__auto-chip--info'
    case 'success': return 'oqr__auto-chip--ok'
    case 'no_credit':
    case 'failed': return 'oqr__auto-chip--bad'
    case 'resetting': return 'oqr__auto-chip--busy'
    default: return ''
  }
})
const hiddenResetCreditCount = computed(() => Math.max(resetCreditExpirations.value.length - 1, 0))
const canReset = computed(() => availableResetCount.value > 0 && !isShadow.value)

const resetCreditDetailsTitle = computed(() =>
  resetCreditExpirations.value
    .map((expiresAt) => formatResetCreditExpiry(expiresAt, 'full'))
    .join('\n')
)

const resetCreditDetailsToggleLabel = computed(() => {
  if (showResetCreditDetails.value) {
    return t('admin.accounts.openaiQuotaReset.collapseExpirations')
  }
  return t('admin.accounts.openaiQuotaReset.expandExpirations', { count: hiddenResetCreditCount.value })
})

const resetButtonTitle = computed(() => {
  if (isShadow.value) return t('admin.accounts.openaiQuotaReset.resetTooltipShadow')
  if (!data.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
  if (!canReset.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNoCredits')
  return t('admin.accounts.openaiQuotaReset.resetTooltipReady')
})

// "次数" button doubles as the upstream-query trigger and the count display.
// Tooltip differs between "click to load" (no data yet) and "click to refresh".
const countButtonTitle = computed(() => {
  if (!data.value) return t('admin.accounts.openaiQuotaReset.countTooltipLoad')
  return t('admin.accounts.openaiQuotaReset.countTooltipRefresh')
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}…` : error.value
})

const getResetCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

const compareResetCreditExpiry = (a: string, b: string): number => {
  const diff = getResetCreditExpiryTime(a) - getResetCreditExpiryTime(b)
  if (diff !== 0) return diff
  return a.localeCompare(b)
}

const formatResetCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') {
    options.year = 'numeric'
  }

  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const extractErrorMessage = (e: unknown): string => {
  // The project's axios response interceptor (api/client.ts) flattens server
  // errors into { status, code, message, reason, ... } and re-rejects them, so
  // the message lives at the top level rather than under .response.data. Fall
  // back to the raw axios shape for the cancellation/network branches that
  // bypass the flattening, and finally to the generic i18n string.
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

const toggleResetCreditDetails = () => {
  if (hiddenResetCreditCount.value <= 0) return
  showResetCreditDetails.value = !showResetCreditDetails.value
}

const handleQuery = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  showResetCreditDetails.value = false
  try {
    const result = await refreshOpenAIQuota(props.account.id)
    // The upstream read succeeded even when the snapshot write was rejected, so
    // the live count is always adopted. Only the persisted view is left alone,
    // which keeps the displayed expirations consistent with what is stored.
    data.value = result
    if (result.cache_persisted) {
      cachedData.value = result
    } else {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

const openResetConfirm = () => {
  if (resetting.value || loading.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  showResetConfirm.value = true
}

const confirmReset = async () => {
  showResetConfirm.value = false
  if (resetting.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  resetting.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(props.account.id)
    showResetCreditDetails.value = false
    if (result.cache_refreshed && result.quota) {
      data.value = result.quota
      cachedData.value = result.quota
    } else {
      // A credit was consumed but the post-reset count could not be read back.
      // Whatever we still hold is one generation stale, so report the count as
      // unknown instead of letting a second consumption start from stale data.
      data.value = null
    }
    if (result.account) emit('account-updated', result.account)

    if (result.warning_code === 'reset_credit_cache_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    } else if (result.warning_code === 'account_state_recovery_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    } else if (result.warning_code === 'account_state_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRefreshFailed')
    } else {
      resetMessage.value = t('admin.accounts.openaiQuotaReset.resetSuccess', {
        windows: result.windows_reset
      })
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    resetting.value = false
  }
}

watch(
  () => props.account.id,
  () => {
    // Account row may be reused across paginated lists; reset local state.
    cachedData.value = readCachedResetCredits(props.account)
    data.value = cachedData.value
    error.value = null
    resetMessage.value = null
    resetWarning.value = null
    loading.value = false
    resetting.value = false
    showResetConfirm.value = false
    showResetCreditDetails.value = false
  }
)

watch(
  resetCreditExpirations,
  () => {
    if (hiddenResetCreditCount.value <= 0) {
      showResetCreditDetails.value = false
    }
  }
)
</script>

<style scoped>
.oqr {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.oqr__auto {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  font-size: var(--fs-2xs);
  min-width: 0;
}

.oqr__auto-chip {
  display: inline-flex;
  align-items: center;
  border-radius: var(--r-sm);
  padding: 1px 6px;
  font-weight: var(--fw-medium);
  background: var(--muted);
  color: var(--muted-foreground);
}

.oqr__auto-chip--info { background: var(--brand-tint); color: var(--brand); }
.oqr__auto-chip--ok { background: color-mix(in oklch, var(--success) 14%, var(--card)); color: var(--success); }
.oqr__auto-chip--bad { background: var(--destructive-soft); color: var(--destructive); }
.oqr__auto-chip--busy { background: var(--s2a-attn-soft); color: var(--s2a-attn); }

.oqr__auto-window {
  margin-left: 4px;
  font-variant-numeric: tabular-nums;
}

.oqr__auto-time { color: var(--muted-foreground); }

.oqr__auto-error {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--destructive);
}

.oqr__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

/* Neutral by default: colour here is chrome, not a permanent category paint.
   The reset control's destructive tint only shows on hover (an interaction
   state), never at rest. */
.oqr__btn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  height: 18px;
  padding: 0 5px;
  border: 0;
  border-radius: var(--r-xs);
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}
.oqr__btn:hover:not(:disabled) {
  background: var(--sidebar-accent);
  color: var(--foreground);
}
.oqr__btn[data-tone='reset']:hover:not(:disabled) {
  background: var(--destructive-soft);
  color: var(--destructive);
}
.oqr__btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.oqr__icon {
  font-size: 10px; /* june-lint-disable ground-rule-4: icon glyph */
  flex-shrink: 0;
}
.oqr__icon--spin {
  animation: oqr-spin 0.7s linear infinite;
}
@media (prefers-reduced-motion: reduce) {
  .oqr__icon--spin {
    animation-duration: 0.01ms;
  }
}
@keyframes oqr-spin {
  to {
    transform: rotate(360deg);
  }
}

.oqr__expiry {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.oqr__expiry-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.oqr__chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  height: 16px;
  padding: 0 6px;
  border-radius: var(--r-xs);
  background: var(--muted);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  line-height: 16px;
}

.oqr__toggle {
  display: inline-flex;
  align-items: center;
  height: 16px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--r-pill);
  background: var(--muted);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  line-height: 16px;
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}
.oqr__toggle:hover {
  background: var(--sidebar-accent);
  color: var(--foreground);
}

.oqr__details {
  display: inline-grid;
  gap: 2px;
  max-width: 100%;
  padding: 6px 7px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-sm);
  background: var(--popover);
  box-shadow: var(--shadow-sm);
}

.oqr__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.oqr__detail-row {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}

.oqr__detail-dot {
  flex-shrink: 0;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--muted-foreground);
}

.oqr__detail-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.oqr__feedback {
  font-size: var(--fs-2xs);
  color: var(--muted-foreground);
}
.oqr__feedback[data-tone='error'] {
  color: var(--destructive);
}
.oqr__feedback[data-tone='warning'] {
  color: var(--s2a-attn);
}
.oqr__feedback[data-tone='success'] {
  color: var(--success);
}
</style>
