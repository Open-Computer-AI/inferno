<template>
  <BaseDialog :show="show" :title="t('admin.affiliates.withdraw.title')" width="normal" @close="handleClose">
    <form class="space-y-4" @submit.prevent="submit">
      <div>
        <label class="input-label">{{ t('admin.affiliates.withdraw.user') }}</label>
        <div v-if="selectedUser" class="flex items-center justify-between gap-3 rounded-md border border-primary-200 bg-primary-50 px-3 py-2 dark:border-primary-700/50 dark:bg-primary-900/20">
          <div class="min-w-0 truncate text-sm">
            <span class="font-mono text-gray-500 dark:text-dark-400">#{{ selectedUser.id }}</span>
            <span class="ml-2 font-medium text-gray-900 dark:text-white">{{ selectedUser.email }}</span>
            <span v-if="selectedUser.username" class="ml-1 text-xs text-gray-500 dark:text-dark-400">({{ selectedUser.username }})</span>
          </div>
          <button type="button" class="text-lg leading-none text-gray-400 hover:text-red-600 disabled:opacity-50" :disabled="submitting || outcomeUncertain" @click="clearUser">×</button>
        </div>
        <template v-else>
          <input v-model="userQuery" type="text" class="input" :placeholder="t('admin.affiliates.withdraw.userPlaceholder')" @input="onUserQueryInput" />
          <div v-if="userResults.length" class="mt-1 max-h-40 overflow-y-auto rounded border border-gray-200 dark:border-dark-700">
            <button v-for="user in userResults" :key="user.id" type="button" class="w-full px-3 py-1.5 text-left text-sm text-gray-900 hover:bg-gray-100 dark:text-white dark:hover:bg-dark-800" @click="selectUser(user)">
              {{ user.email }} <span class="text-xs text-gray-500 dark:text-dark-400">({{ user.username }})</span>
            </button>
          </div>
          <p v-else-if="searched" class="input-hint">{{ t('admin.affiliates.withdraw.noUserFound') }}</p>
        </template>
      </div>

      <div v-if="selectedUser" class="rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex items-center justify-between text-sm">
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.withdraw.availableQuota') }}</span>
          <span v-if="overviewLoading" class="h-4 w-4 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></span>
          <span v-else class="font-semibold text-gray-900 dark:text-white">${{ formatPreciseAmount(availableQuota) }}</span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.withdraw.frozenHint') }}</p>
      </div>

      <div>
        <label class="input-label">{{ t('admin.affiliates.withdraw.amount') }}</label>
        <div class="flex gap-2">
          <input v-model="amount" type="number" min="0" step="any" inputmode="decimal" class="input" :disabled="!selectedUser || overviewLoading || submitting || outcomeUncertain" />
          <button type="button" class="btn btn-secondary shrink-0" :disabled="!selectedUser || overviewLoading || submitting || outcomeUncertain || availableQuota <= 0" @click="fillAll">{{ t('admin.affiliates.withdraw.fillAll') }}</button>
        </div>
        <p v-if="amountError" class="input-error-text">{{ amountError }}</p>
        <p v-else class="input-hint">{{ t('admin.affiliates.withdraw.amountHint') }}</p>
      </div>

      <div v-if="outcomeUncertain" class="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800 dark:border-blue-700/50 dark:bg-blue-900/20 dark:text-blue-300">
        {{ t('admin.affiliates.withdraw.uncertainHint') }}
      </div>
      <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-700/50 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('admin.affiliates.withdraw.warning') }}
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="submitting || outcomeUncertain" @click="handleClose">{{ t('common.cancel') }}</button>
        <button type="button" class="btn btn-primary" :disabled="!canSubmit" @click="submit">
          {{ submitting ? t('admin.affiliates.withdraw.submitting') : outcomeUncertain ? t('admin.affiliates.withdraw.retry') : t('admin.affiliates.withdraw.submit') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { affiliatesAPI, type AffiliateWithdrawResult, type SimpleUser } from '@/api/admin/affiliates'
import { useAppStore } from '@/stores/app'
import { extractApiErrorCode, extractI18nErrorMessage } from '@/utils/apiError'
import { canDismissAffiliateWithdraw, completeAffiliateWithdrawOperation, prepareAffiliateWithdrawOperation, type AffiliateWithdrawOperation } from './affiliateWithdrawOperation'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{ close: []; success: [result: AffiliateWithdrawResult] }>()
const SCALE = 1e8
const { t } = useI18n()
const appStore = useAppStore()
const userQuery = ref('')
const userResults = ref<SimpleUser[]>([])
const searched = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
const availableQuota = ref(0)
const overviewLoading = ref(false)
const amount = ref<number | string>('')
const submitting = ref(false)
const pendingOperation = ref<AffiliateWithdrawOperation | null>(null)
const outcomeUncertain = computed(() => pendingOperation.value?.outcomeUncertain === true)
const amountText = computed(() => String(amount.value ?? '').trim())
const parsedAmount = computed(() => amountText.value ? Number(amountText.value) : Number.NaN)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchSequence = 0
let overviewSequence = 0

const amountError = computed(() => {
  if (outcomeUncertain.value || !selectedUser.value || overviewLoading.value || !amountText.value) return ''
  const value = parsedAmount.value
  if (!Number.isFinite(value) || Math.round(value * SCALE) <= 0) return t('admin.affiliates.withdraw.amountRequired')
  if (Math.round(value * SCALE) > Math.round(availableQuota.value * SCALE)) return t('admin.affiliates.withdraw.amountExceeds')
  return ''
})
const canSubmit = computed(() => selectedUser.value !== null && !submitting.value && (outcomeUncertain.value || (!overviewLoading.value && !!amountText.value && !amountError.value)))

function formatPreciseAmount(value: number) {
  const [whole, fraction = ''] = Number(value || 0).toFixed(8).replace(/0+$/, '').split('.')
  return `${whole}.${fraction.padEnd(2, '0')}`
}
function showError(error: unknown) {
  appStore.showError(extractI18nErrorMessage(error, t, 'admin.affiliates.errors', t('common.error')))
}
function onUserQueryInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searched.value = false
  const query = userQuery.value.trim()
  if (!query) {
    searchSequence++
    userResults.value = []
    return
  }
  searchTimer = setTimeout(() => void searchUsers(query), 300)
}
async function searchUsers(query: string) {
  const sequence = ++searchSequence
  try {
    const users = await affiliatesAPI.lookupUsers(query)
    if (sequence !== searchSequence) return
    userResults.value = users || []
    searched.value = true
  } catch (error) {
    if (sequence === searchSequence) showError(error)
  }
}
async function loadAvailableQuota(userId: number) {
  const sequence = ++overviewSequence
  overviewLoading.value = true
  try {
    const overview = await affiliatesAPI.getUserOverview(userId)
    if (sequence === overviewSequence) availableQuota.value = Number(overview.available_quota || 0)
  } catch (error) {
    if (sequence !== overviewSequence) return
    availableQuota.value = 0
    if (extractApiErrorCode(error) !== 'USER_NOT_FOUND') {
      if (!outcomeUncertain.value) selectedUser.value = null
      showError(error)
    }
  } finally {
    if (sequence === overviewSequence) overviewLoading.value = false
  }
}
function selectUser(user: SimpleUser) {
  if (searchTimer) clearTimeout(searchTimer)
  searchSequence++
  selectedUser.value = user
  userQuery.value = ''
  userResults.value = []
  searched.value = false
  availableQuota.value = 0
  amount.value = ''
  void loadAvailableQuota(user.id)
}
function clearUser() {
  overviewSequence++
  selectedUser.value = null
  availableQuota.value = 0
  overviewLoading.value = false
  amount.value = ''
}
function fillAll() {
  amount.value = Math.round(availableQuota.value * SCALE) / SCALE
}
function resetForm() {
  if (searchTimer) clearTimeout(searchTimer)
  searchSequence++
  overviewSequence++
  userQuery.value = ''
  userResults.value = []
  searched.value = false
  selectedUser.value = null
  availableQuota.value = 0
  overviewLoading.value = false
  amount.value = ''
  submitting.value = false
  pendingOperation.value = null
}
function handleClose() {
  if (canDismissAffiliateWithdraw(submitting.value, outcomeUncertain.value)) emit('close')
}
async function submit() {
  const user = selectedUser.value
  if (!user || !canSubmit.value) return
  if (!pendingOperation.value) pendingOperation.value = prepareAffiliateWithdrawOperation(user.id, parsedAmount.value)
  const operation = pendingOperation.value
  submitting.value = true
  try {
    const { result, replayed } = await affiliatesAPI.withdrawUserQuota(operation.userId, { amount: operation.amount }, operation.key)
    completeAffiliateWithdrawOperation(operation)
    pendingOperation.value = null
    appStore.showSuccess(t(replayed ? 'admin.affiliates.withdraw.replayed' : 'admin.affiliates.withdraw.success', {
      amount: `$${formatPreciseAmount(result.amount)}`,
      remaining: `$${formatPreciseAmount(result.available_quota_after)}`,
    }))
    emit('success', result)
  } catch (error) {
    showError(error)
    const status = (error as { status?: number } | null)?.status
    if (!operation.outcomeUncertain && status && status >= 400 && status < 500 && status !== 408 && status !== 409) {
      completeAffiliateWithdrawOperation(operation)
      pendingOperation.value = null
    } else {
      operation.outcomeUncertain = true
    }
    void loadAvailableQuota(operation.userId)
  } finally {
    submitting.value = false
  }
}
watch(() => props.show, (show) => { if (show) resetForm() }, { immediate: true })
onBeforeUnmount(() => { if (searchTimer) clearTimeout(searchTimer) })
</script>
