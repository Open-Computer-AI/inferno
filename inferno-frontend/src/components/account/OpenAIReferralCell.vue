<template>
  <div class="oir">
    <div class="oir__actions">
      <button
        type="button"
        data-testid="referral-count"
        class="oir__btn oir__btn--count"
        :disabled="loading || sending"
        :title="countTitle"
        @click="refresh()"
      >
        {{ t('admin.accounts.openaiReferral.available') }} {{ countDisplay }}
      </button>
      <button
        type="button"
        data-testid="referral-open"
        class="oir__btn oir__btn--invite"
        :disabled="sending || isShadow"
        :title="isShadow ? t('admin.accounts.openaiReferral.shadowHint') : undefined"
        @click="openDialog"
      >
        {{ t('admin.accounts.openaiReferral.invite') }}
      </button>
      <span v-if="error && !show" class="oir__feedback oir__feedback--error" :title="error">{{ error }}</span>
      <span v-if="warning && !show" class="oir__feedback oir__feedback--warning" :title="warning">{{ warning }}</span>
    </div>

    <BaseDialog
      v-if="show"
      :show="show"
      :title="t('admin.accounts.openaiReferral.invite')"
      width="wide"
      :show-close-button="!sending"
      :close-on-escape="!sending"
      @close="closeDialog"
    >
      <form :id="formID" class="oir__form" @submit.prevent="sendInvite">
        <p class="oir__account-copy">
          {{ t('admin.accounts.openaiReferral.fromAccount') }} <strong>{{ account.name }}</strong>
        </p>

        <div class="oir__summary">
          <div>
            <p class="oir__program">{{ programLabel }}</p>
            <p class="oir__available">
              {{ t('admin.accounts.openaiReferral.available') }} {{ countDisplay }}
            </p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || sending" @click="refresh()">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
        </div>

        <div
          v-if="eligibility?.title || eligibility?.description || eligibility?.rules?.length"
          class="oir__offer"
        >
          <p v-if="eligibility.title" class="oir__offer-title">{{ eligibility.title }}</p>
          <p v-if="eligibility.description">{{ eligibility.description }}</p>
          <ul v-if="eligibility.rules?.length" class="oir__rules">
            <li v-for="(rule, index) in eligibility.rules" :key="index">{{ rule }}</li>
          </ul>
        </div>

        <p
          v-if="fresh && (!eligibility?.should_show || count === null || count <= 0)"
          class="oir__availability-warning"
        >
          {{ t('admin.accounts.openaiReferral.unavailable') }}
        </p>

        <div>
          <label :for="emailID" class="input-label">{{ t('admin.accounts.openaiReferral.email') }}</label>
          <input
            :id="emailID"
            v-model="email"
            type="email"
            class="input"
            autocomplete="off"
            maxlength="254"
            required
            :disabled="sending"
            placeholder="friend@example.com"
            data-testid="referral-email"
          />
        </div>

        <div v-if="needsConsent" class="oir__consent">
          <Checkbox v-model="confirmed" :disabled="sending" data-testid="referral-consent">
            {{ t('admin.accounts.openaiReferral.consent') }}
          </Checkbox>
        </div>

        <p v-if="error" role="alert" class="oir__dialog-feedback oir__dialog-feedback--error">{{ error }}</p>
        <p v-if="sentEmail" role="status" class="oir__dialog-feedback oir__dialog-feedback--success">
          {{ t('admin.accounts.openaiReferral.sent', { email: sentEmail }) }}
        </p>
        <p v-if="warning" role="status" class="oir__dialog-feedback oir__dialog-feedback--warning">{{ warning }}</p>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="sending" @click="closeDialog">
          {{ t('common.close') }}
        </button>
        <button type="submit" :form="formID" class="btn btn-primary" :disabled="!canSend" data-testid="referral-send">
          {{ sending ? t('admin.accounts.openaiReferral.sending') : t('admin.accounts.openaiReferral.send') }}
        </button>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import type { OpenAIReferralEligibility } from '@/types/openaiReferrals'
import { refreshOpenAIReferrals, sendOpenAIReferralInvite } from '@/api/admin/accounts'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Checkbox from '@/components/common/Checkbox.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()

const cachedEligibility = (account: Account): OpenAIReferralEligibility | null => {
  const snapshot = (account.extra as Record<string, unknown> | undefined)?.codex_referral_snapshot
  return snapshot && typeof snapshot === 'object' && !Array.isArray(snapshot)
    ? snapshot as OpenAIReferralEligibility
    : null
}

const eligibility = ref<OpenAIReferralEligibility | null>(cachedEligibility(props.account))
const show = ref(false)
const loading = ref(false)
const sending = ref(false)
const fresh = ref(false)
const email = ref('')
const confirmed = ref(false)
const error = ref('')
const warning = ref('')
const sentEmail = ref('')
let generation = 0

const isShadow = computed(() => props.account.parent_account_id != null)
const formID = computed(() => `codex-referral-form-${props.account.id}`)
const emailID = computed(() => `codex-referral-email-${props.account.id}`)
const count = computed(() => {
  const value = eligibility.value?.available_invites
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : null
})
const countDisplay = computed(() => count.value ?? '—')
const countTitle = computed(() => {
  const time = eligibility.value?.fetched_at
  return time
    ? t('admin.accounts.openaiReferral.checkedAt', { time: new Date(time * 1000).toLocaleString() })
    : t('admin.accounts.openaiReferral.queryHint')
})
const programLabel = computed(() => t(
  eligibility.value?.program_id === 'codex_referral_workspace'
    ? 'admin.accounts.openaiReferral.workspace'
    : 'admin.accounts.openaiReferral.personal'
))
const needsConsent = computed(() => eligibility.value?.requires_explicit_confirmation !== false)
const canSend = computed(() => fresh.value && !isShadow.value && !loading.value && !sending.value
  && eligibility.value?.should_show === true && count.value !== null && count.value > 0
  && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim())
  && (!needsConsent.value || confirmed.value))

function errorMessage(value: unknown, duringSend = false): string {
  const err = value as { status?: number; reason?: string; code?: string; message?: string }
  const key = err.reason || err.code || ''
  if (duringSend && (err.status === 0 || ['ECONNABORTED', 'ETIMEDOUT', 'ERR_NETWORK', 'ERR_CANCELED'].includes(key))) {
    return t('admin.accounts.openaiReferral.sendUnknown')
  }
  const keys: Record<string, string> = {
    OPENAI_REFERRAL_UNAVAILABLE: 'unavailable',
    OPENAI_REFERRAL_FORBIDDEN: 'unavailable',
    OPENAI_REFERRAL_INVALID_EMAIL: 'invalidEmail',
    OPENAI_REFERRAL_REJECTED: 'rejected',
    OPENAI_REFERRAL_ALREADY_EXISTS: 'alreadyInvited',
    OPENAI_REFERRAL_RATE_LIMITED: 'rateLimited',
    OPENAI_REFERRAL_SEND_UNKNOWN: 'sendUnknown',
    OPENAI_REFERRAL_PROGRAM_CHANGED: 'programChanged',
    OPENAI_REFERRAL_CONFIRMATION_REQUIRED: 'consentRequired'
  }
  return keys[key] ? t(`admin.accounts.openaiReferral.${keys[key]}`) : err.message || t('common.error')
}

async function refresh() {
  if (loading.value || sending.value) return
  const current = generation
  loading.value = true
  fresh.value = false
  error.value = ''
  warning.value = ''
  try {
    const result = await refreshOpenAIReferrals(props.account.id)
    if (current !== generation) return
    eligibility.value = result.eligibility
    fresh.value = result.eligibility !== null
    if (!result.cache_persisted) warning.value = t('admin.accounts.openaiReferral.cacheFailed')
  } catch (err) {
    if (current === generation) error.value = errorMessage(err)
  } finally {
    if (current === generation) loading.value = false
  }
}

function openDialog() {
  if (isShadow.value || sending.value) return
  show.value = true
  email.value = ''
  confirmed.value = false
  sentEmail.value = ''
  void refresh()
}

function closeDialog() {
  if (!sending.value) show.value = false
}

async function sendInvite() {
  if (!canSend.value || !eligibility.value) return
  const current = generation
  sending.value = true
  error.value = ''
  warning.value = ''
  sentEmail.value = ''
  try {
    const result = await sendOpenAIReferralInvite(props.account.id, {
      email: email.value.trim(),
      program_id: eligibility.value.program_id,
      confirmed: confirmed.value
    })
    if (current !== generation) return
    if (!result.sent) throw { code: 'OPENAI_REFERRAL_SEND_UNKNOWN' }
    eligibility.value = result.eligibility
    fresh.value = !result.refresh_failed && result.eligibility !== null
    sentEmail.value = result.email
    email.value = ''
    confirmed.value = false
    if (result.refresh_failed) warning.value = t('admin.accounts.openaiReferral.refreshFailed')
    else if (!result.cache_persisted) warning.value = t('admin.accounts.openaiReferral.cacheFailed')
  } catch (err) {
    if (current !== generation) return
    fresh.value = false
    error.value = errorMessage(err, true)
  } finally {
    if (current === generation) sending.value = false
  }
}

watch(email, () => { confirmed.value = false })
watch(() => props.account.id, () => {
  generation++
  eligibility.value = cachedEligibility(props.account)
  show.value = false
  fresh.value = false
  loading.value = false
  sending.value = false
  email.value = ''
  confirmed.value = false
  error.value = ''
  warning.value = ''
  sentEmail.value = ''
})
</script>

<style scoped>
.oir {
  min-width: 0;
}

.oir__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  min-width: 0;
}

.oir__btn {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-height: 20px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--r-xs);
  background: transparent;
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  line-height: 20px;
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover), opacity var(--motion-hover);
}

.oir__btn:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.oir__btn--count {
  color: var(--accent-foreground);
}

.oir__btn--count:hover:not(:disabled) {
  background: var(--accent);
}

.oir__btn--invite {
  color: var(--primary);
}

.oir__btn--invite:hover:not(:disabled) {
  background: var(--primary-muted);
}

.oir__feedback,
.oir__dialog-feedback {
  font-size: var(--fs-2xs);
}

.oir__feedback {
  max-width: 192px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.oir__feedback--error,
.oir__dialog-feedback--error {
  color: var(--destructive);
}

.oir__feedback--warning,
.oir__dialog-feedback--warning,
.oir__availability-warning {
  color: var(--s2a-attn);
}

.oir__form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.oir__account-copy,
.oir__available,
.oir__offer,
.oir__consent,
.oir__dialog-feedback {
  color: var(--muted-foreground);
}

.oir__account-copy,
.oir__offer,
.oir__consent,
.oir__dialog-feedback,
.oir__availability-warning {
  font-size: var(--fs-sm);
}

.oir__summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--muted);
}

.oir__program {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
}

.oir__available {
  margin-top: 4px;
}

.oir__offer {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.oir__offer-title {
  color: var(--foreground);
  font-weight: var(--fw-medium);
}

.oir__rules {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 20px;
  list-style: disc;
}

.oir__consent {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.oir__dialog-feedback--success {
  color: var(--success);
}
</style>
