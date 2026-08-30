<template>
  <AppLayout>
    <div class="orders-page">
      <!-- Filters -->
      <section class="orders-filter-panel" :aria-label="t('payment.admin.orderFilters')">
        <header class="orders-filter-panel__header">
          <div class="orders-filter-panel__copy">
            <p class="orders-filter-panel__eyebrow">{{ t('payment.admin.orders') }}</p>
            <h2 class="orders-filter-panel__title">{{ t('payment.admin.orderFilters') }}</h2>
            <p class="orders-filter-panel__hint">{{ t('payment.admin.orderFiltersHint') }}</p>
          </div>
          <IconButton
            icon="hgi-arrow-reload-horizontal"
            :label="t('common.refresh')"
            :loading="ordersLoading"
            :disabled="ordersLoading"
            class="orders-refresh"
            @click="loadOrders"
          />
        </header>
        <div class="orders-filter-panel__controls">
          <div class="orders-filter-panel__search">
            <input
              v-model="orderSearch"
              type="text"
              :placeholder="t('payment.admin.searchOrders')"
              :aria-label="t('payment.admin.searchOrders')"
              class="field-control"
              @input="debounceLoadOrders"
            />
          </div>
          <Select
            v-model="orderFilters.status"
            :options="statusFilterOptions"
            :aria-label="t('payment.admin.allStatuses')"
            class="orders-filter-panel__select"
            @change="loadOrders"
          />
          <Select
            v-model="orderFilters.payment_type"
            :options="paymentTypeFilterOptions"
            :aria-label="t('payment.admin.allPaymentTypes')"
            class="orders-filter-panel__select"
            @change="loadOrders"
          />
          <Select
            v-model="orderFilters.order_type"
            :options="orderTypeFilterOptions"
            :aria-label="t('payment.admin.allOrderTypes')"
            class="orders-filter-panel__select"
            @change="loadOrders"
          />
        </div>
      </section>

      <!-- Table -->
      <div class="orders-table-region">
        <OrderTable :orders="orders" :loading="ordersLoading" show-user>
          <template #actions="{ row }">
            <div class="flex items-center gap-1">
              <Button variant="ghost" size="xs" icon="hgi-eye" @click="showOrderDetail(row)">
                {{ t('common.view') }}
              </Button>
              <Button v-if="row.status === 'PENDING'" variant="warning" size="xs" icon="hgi-cancel-01" @click="handleCancelOrder(row)">
                {{ t('payment.orders.cancel') }}
              </Button>
              <Button v-if="row.status === 'FAILED'" variant="ghost" size="xs" icon="hgi-arrow-reload-horizontal" @click="handleRetryOrder(row)">
                {{ t('payment.admin.retry') }}
              </Button>
              <template v-if="row.status === 'REFUND_REQUESTED'">
                <span v-if="row.refund_amount" class="rounded-full bg-[var(--brand-tint)] px-1.5 py-0.5 text-xs font-[var(--fw-medium)] text-[var(--brand)] bg-[color-mix(in_srgb,var(--brand-tint)_30%,transparent)] text-[var(--brand)]">{{ creditedAmountSymbol }}{{ row.refund_amount.toFixed(2) }}</span>
                <Button variant="ghost" size="xs" icon="hgi-checkmark-circle-02" @click="openRefundDialog(row)">
                  {{ t('payment.admin.approveRefund') }}
                </Button>
              </template>
              <Button v-else-if="row.status === 'REFUND_FAILED'" variant="ghost" size="xs" icon="hgi-arrow-reload-horizontal" @click="openRefundDialog(row)">
                {{ t('payment.admin.retryRefund') }}
              </Button>
              <Button v-else-if="row.status === 'REFUND_PENDING'" variant="warning" size="xs" icon="hgi-arrow-reload-horizontal" :loading="refundQueryingIds.has(row.id)" :disabled="refundQueryingIds.has(row.id)" @click="handleQueryRefund(row)">
                {{ t('payment.admin.queryRefundStatus') }}
              </Button>
              <Button v-else-if="row.status === 'COMPLETED' || row.status === 'PARTIALLY_REFUNDED'" variant="danger" size="xs" icon="hgi-dollar-circle" @click="openRefundDialog(row)">
                {{ t('payment.admin.refund') }}
              </Button>
            </div>
          </template>
        </OrderTable>
        <Pagination v-if="orderPagination.total > 0" :page="orderPagination.page" :total="orderPagination.total" :page-size="orderPagination.page_size" @update:page="handleOrderPageChange" @update:pageSize="handleOrderPageSizeChange" />
      </div>
    </div>

    <!-- Order Detail Dialog -->
    <BaseDialog :show="showDetailDialog" :title="t('payment.admin.orderDetail')" width="wide" @close="showDetailDialog = false">
      <div v-if="selectedOrder" class="space-y-4">
        <div class="grid grid-cols-2 gap-4">
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.orderId') }}</p><p class="font-mono text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">#{{ selectedOrder.id }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.orderNo') }}</p><p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ selectedOrder.out_trade_no }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.status') }}</p><OrderStatusBadge :status="selectedOrder.status" /></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.amount') }}</p><p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ creditedAmountSymbol }}{{ selectedOrder.amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.payAmount') }}</p><p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ paymentAmountSymbol(selectedOrder) }}{{ selectedOrder.pay_amount.toFixed(2) }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.paymentMethod') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ t('payment.methods.' + selectedOrder.payment_type, selectedOrder.payment_type) }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.feeRate') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ selectedOrder.fee_rate }}%</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.createdAt') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(selectedOrder.created_at) }}</p></div>
          <div><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.expiresAt') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(selectedOrder.expires_at) }}</p></div>
          <div v-if="selectedOrder.paid_at"><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.paidAt') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(selectedOrder.paid_at) }}</p></div>
          <div v-if="selectedOrder.refund_amount"><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.refundAmount') }}</p><p class="text-sm font-[var(--fw-medium)] text-[var(--destructive)] text-[var(--destructive)]">{{ creditedAmountSymbol }}{{ selectedOrder.refund_amount.toFixed(2) }}</p></div>
          <div v-if="selectedOrder.refund_reason" class="col-span-2"><p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.refundReason') }}</p><p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ selectedOrder.refund_reason }}</p></div>
          <!-- Refund request info -->
          <div v-if="selectedOrder.refund_requested_at" class="col-span-2 border-t border-[var(--brand-line)] pt-3 dark:border-[var(--border-subtle)]">
            <p class="mb-2 text-xs font-[var(--fw-medium)] text-[var(--brand)] text-[var(--brand)]">{{ t('payment.admin.refundRequestInfo') }}</p>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.refundRequestedAt') }}</p>
                <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(selectedOrder.refund_requested_at) }}</p>
              </div>
              <div>
                <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.refundRequestedBy') }}</p>
                <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">#{{ selectedOrder.refund_requested_by }}</p>
              </div>
              <div class="col-span-2">
                <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.refundRequestReason') }}</p>
                <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ selectedOrder.refund_request_reason }}</p>
              </div>
            </div>
          </div>
        </div>
        <!-- Audit Logs -->
        <div v-if="orderAuditLogs.length > 0" class="border-t border-[var(--brand-line)] pt-4 dark:border-[var(--border-subtle)]">
          <p class="mb-2 text-xs font-[var(--fw-medium)] text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.auditLogs') }}</p>
          <div class="max-h-48 space-y-2 overflow-y-auto">
            <div v-for="log in orderAuditLogs" :key="log.id" class="rounded-lg border border-[var(--brand-line)] bg-[var(--brand-tint)] p-2.5 dark:border-[var(--border-subtle)] dark:bg-[var(--card)]">
              <div class="flex items-center justify-between">
                <span class="text-xs font-[var(--fw-medium)] text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ log.action }}</span>
                <span class="text-xs text-[var(--muted-foreground)]">{{ formatDateTime(log.created_at) }}</span>
              </div>
              <div v-if="log.detail" class="mt-1 break-all text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ log.detail }}</div>
              <div v-if="log.operator" class="mt-1 text-xs text-[var(--muted-foreground)]">{{ t('payment.admin.operator') }}: {{ log.operator }}</div>
            </div>
          </div>
        </div>
      </div>
    </BaseDialog>

    <AdminRefundDialog :show="showRefundDialog" :order="selectedOrder" :submitting="refundSubmitting" :require-force="refundRequireForce" :warning="refundWarning" @confirm="handleRefund" @cancel="closeRefundDialog" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { formatOrderDateTime } from '@/components/payment/orderUtils'
import type { PaymentOrder } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Button from '@/components/common/Button.vue'
import IconButton from '@/components/common/IconButton.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import AdminRefundDialog from '@/components/admin/payment/AdminRefundDialog.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import OrderTable from '@/components/payment/OrderTable.vue'
import { currencySymbol } from '@/components/payment/currency'

interface AuditLog {
  id: number
  action: string
  detail: string | null
  operator: string | null
  created_at: string
}

const { t } = useI18n()
const appStore = useAppStore()

const ordersLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const orderSearch = ref('')
const orderFilters = reactive({ status: '', payment_type: '', order_type: '' })
const orderPagination = reactive({ page: 1, page_size: 20, total: 0 })
const selectedOrder = ref<PaymentOrder | null>(null)
const showDetailDialog = ref(false)
const showRefundDialog = ref(false)
const refundSubmitting = ref(false)
const refundRequireForce = ref(false)
const refundWarning = ref('')
const refundQueryingIds = ref(new Set<number>())
const orderAuditLogs = ref<AuditLog[]>([])
const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder | null | undefined): string {
  return currencySymbol(order?.currency)
}

let debounceTimer: ReturnType<typeof setTimeout> | null = null
function debounceLoadOrders() {
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => loadOrders(), 300)
}

async function loadOrders() {
  ordersLoading.value = true
  try {
    const res = await adminPaymentAPI.getOrders({
      page: orderPagination.page, page_size: orderPagination.page_size,
      keyword: orderSearch.value || undefined, status: orderFilters.status || undefined,
      payment_type: orderFilters.payment_type || undefined, order_type: orderFilters.order_type || undefined,
    })
    orders.value = res.data.items || []
    orderPagination.total = res.data.total || 0
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally { ordersLoading.value = false }
}

function handleOrderPageChange(page: number) { orderPagination.page = page; loadOrders() }
function handleOrderPageSizeChange(size: number) { orderPagination.page_size = size; orderPagination.page = 1; loadOrders() }

const statusFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allStatuses') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'PAID', label: t('payment.status.paid') },
  { value: 'COMPLETED', label: t('payment.status.completed') },
  { value: 'EXPIRED', label: t('payment.status.expired') },
  { value: 'CANCELLED', label: t('payment.status.cancelled') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUNDED', label: t('payment.status.refunded') },
  { value: 'REFUND_REQUESTED', label: t('payment.status.refund_requested') },
  { value: 'REFUND_PENDING', label: t('payment.status.refund_pending') },
  { value: 'REFUND_FAILED', label: t('payment.status.refund_failed') },
])

const paymentTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allPaymentTypes') },
  { value: 'alipay', label: t('payment.methods.alipay') },
  { value: 'wxpay', label: t('payment.methods.wxpay') },
  { value: 'stripe', label: t('payment.methods.stripe') },
  { value: 'airwallex', label: t('payment.methods.airwallex') },
])

const orderTypeFilterOptions = computed(() => [
  { value: '', label: t('payment.admin.allOrderTypes') },
  { value: 'balance', label: t('payment.admin.balanceOrder') },
  { value: 'subscription', label: t('payment.admin.subscriptionOrder') },
])

async function showOrderDetail(order: PaymentOrder) {
  selectedOrder.value = order
  orderAuditLogs.value = []
  showDetailDialog.value = true
  try {
    const res = await adminPaymentAPI.getOrder(order.id)
    const data = res.data as unknown as Record<string, unknown>
    if (data.order) selectedOrder.value = data.order as PaymentOrder
    orderAuditLogs.value = ((data.auditLogs || data.audit_logs || []) as unknown) as AuditLog[]
  } catch (_err: unknown) { /* keep cached order data */ }
}

async function handleCancelOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.cancelOrder(order.id); appStore.showSuccess(t('payment.admin.orderCancelled')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

async function handleRetryOrder(order: PaymentOrder) {
  try { await adminPaymentAPI.retryRecharge(order.id); appStore.showSuccess(t('payment.admin.retrySuccess')); loadOrders() }
  catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
}

function openRefundDialog(order: PaymentOrder) {
  selectedOrder.value = order
  refundRequireForce.value = false
  refundWarning.value = ''
  showRefundDialog.value = true
}

function closeRefundDialog() {
  showRefundDialog.value = false
  refundRequireForce.value = false
  refundWarning.value = ''
}

function isRefundPendingWarning(warning: string | undefined): boolean {
  return /pending|处理中|待/.test(String(warning || '').toLowerCase())
}

async function handleRefund(data: { amount: number; reason: string; deduct_balance: boolean; force: boolean }) {
  if (!selectedOrder.value) return
  refundSubmitting.value = true
  try {
    const res = await adminPaymentAPI.refundOrder(selectedOrder.value.id, { amount: data.amount, reason: data.reason, deduct_balance: data.deduct_balance, force: data.force })
    if (res.data.success) {
      appStore.showSuccess(t('payment.admin.refundSuccess'))
      closeRefundDialog()
      loadOrders()
      return
    }
    if (isRefundPendingWarning(res.data.warning)) {
      appStore.showSuccess(t('payment.admin.refundPending'))
      closeRefundDialog()
      loadOrders()
      return
    }
    if (res.data.require_force) {
      // Backend needs an explicit force confirmation (e.g. the user spent their
      // balance after requesting the refund). Keep the dialog open and surface
      // the force checkbox instead of dropping the admin back to the list.
      refundRequireForce.value = true
      refundWarning.value = res.data.warning || ''
      return
    }
    appStore.showError(res.data.warning || t('common.error'))
  } catch (err: unknown) { appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error'))) }
  finally { refundSubmitting.value = false }
}

async function handleQueryRefund(order: PaymentOrder) {
  refundQueryingIds.value = new Set(refundQueryingIds.value).add(order.id)
  try {
    const res = await adminPaymentAPI.queryRefund(order.id)
    if (res.data.success) {
      appStore.showSuccess(t('payment.admin.refundSuccess'))
    } else if (isRefundPendingWarning(res.data.warning)) {
      appStore.showSuccess(t('payment.admin.refundPending'))
    } else {
      appStore.showError(res.data.warning || t('common.error'))
    }
    loadOrders()
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    const next = new Set(refundQueryingIds.value)
    next.delete(order.id)
    refundQueryingIds.value = next
  }
}

function formatDateTime(dateStr: string): string { return formatOrderDateTime(dateStr) }

onMounted(() => loadOrders())
</script>

<style scoped>
.orders-page {
  display: grid;
  gap: var(--sp-5);
}

.orders-filter-panel {
  display: grid;
  gap: var(--sp-4);
  padding: var(--sp-5);
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.orders-filter-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--sp-4);
  padding-bottom: var(--sp-4);
  border-bottom: 1px solid var(--border-subtle);
}

.orders-filter-panel__copy {
  display: grid;
  gap: var(--sp-1);
  min-width: 0;
}

.orders-filter-panel__eyebrow {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.orders-filter-panel__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
  letter-spacing: -0.02em;
}

.orders-filter-panel__hint {
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.orders-filter-panel__controls {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: var(--sp-3);
}

.orders-filter-panel__search {
  grid-column: span 2;
  min-width: 0;
}

.orders-filter-panel__select {
  min-width: 0;
}

.orders-refresh {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: var(--control-lg);
  height: var(--control-lg);
  flex: 0 0 auto;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-md);
  background: var(--card);
  color: var(--muted-foreground);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover);
}

.orders-refresh:hover:not(:disabled) {
  background: var(--brand-tint);
  color: var(--foreground);
}

.orders-refresh:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.orders-refresh:disabled {
  cursor: wait;
  opacity: 0.6;
}

.orders-table-region {
  display: grid;
  gap: var(--sp-4);
  min-width: 0;
}

@media (max-width: 900px) {
  .orders-filter-panel__controls {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .orders-filter-panel__search {
    grid-column: span 2;
  }
}

@media (max-width: 640px) {
  .orders-filter-panel {
    padding: var(--sp-4);
  }

  .orders-filter-panel__controls {
    grid-template-columns: minmax(0, 1fr);
  }

  .orders-filter-panel__search {
    grid-column: auto;
  }
}
</style>
