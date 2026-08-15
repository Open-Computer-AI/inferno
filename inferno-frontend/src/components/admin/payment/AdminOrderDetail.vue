<template>
  <BaseDialog
    :show="show"
    :title="t('payment.admin.orderDetail')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="order" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.orderId') }}</p>
          <p class="font-mono text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">#{{ order.id }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.status') }}</p>
          <span :class="['badge', statusBadgeClass(order.status)]">
            {{ t('payment.status.' + order.status.toLowerCase(), order.status) }}
          </span>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.baseAmount') }}</p>
          <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ paymentAmountSymbol }}{{ baseAmount.toFixed(2) }}</p>
        </div>
        <div v-if="order.fee_rate > 0">
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.fee') }} ({{ order.fee_rate }}%)</p>
          <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ paymentAmountSymbol }}{{ feeAmount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.payAmount') }}</p>
          <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ paymentAmountSymbol }}{{ order.pay_amount.toFixed(2) }}</p>
        </div>
        <div v-if="order.amount !== order.pay_amount">
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.creditedAmount') }}</p>
          <p class="text-sm font-[var(--fw-medium)] text-[var(--foreground)] dark:text-white">{{ creditedAmountSymbol }}{{ order.amount.toFixed(2) }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.paymentMethod') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">
            {{ t('payment.methods.' + order.payment_type, order.payment_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.orderType') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">
            {{ t('payment.admin.' + order.order_type + 'Order', order.order_type) }}
          </p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.userId') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">#{{ order.user_id }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.orders.createdAt') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(order.created_at) }}</p>
        </div>
        <div>
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.expiresAt') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(order.expires_at) }}</p>
        </div>
        <div v-if="order.paid_at">
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.paidAt') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(order.paid_at) }}</p>
        </div>
        <div v-if="order.completed_at">
          <p class="text-xs text-[var(--muted-foreground)] text-[var(--muted-foreground)]">{{ t('payment.admin.completedAt') }}</p>
          <p class="text-sm text-[var(--body-copy)] text-[var(--muted-foreground)]">{{ formatDateTime(order.completed_at) }}</p>
        </div>
      </div>

      <div
        v-if="order.refund_amount"
        class="rounded-lg border border-[var(--destructive)] bg-[var(--destructive-soft)] p-3 border-[var(--destructive)] bg-[var(--destructive-soft)/20]"
      >
        <h4 class="mb-2 text-sm font-[var(--fw-medium)] text-[var(--destructive)] text-[var(--destructive)]">
          {{ t('payment.admin.refundInfo') }}
        </h4>
        <div class="grid grid-cols-2 gap-2 text-sm">
          <div>
            <span class="text-[var(--destructive)] text-[var(--destructive)]">{{ t('payment.admin.refundAmount') }}:</span>
            <span class="ml-1 font-[var(--fw-medium)] text-[var(--destructive)] text-[var(--destructive)]">{{ creditedAmountSymbol }}{{ order.refund_amount.toFixed(2) }}</span>
          </div>
          <div v-if="order.refund_reason" class="col-span-2">
            <span class="text-[var(--destructive)] text-[var(--destructive)]">{{ t('payment.admin.refundReason') }}:</span>
            <span class="ml-1 text-[var(--destructive)] text-[var(--destructive)]">{{ order.refund_reason }}</span>
          </div>
        </div>
      </div>

      <div class="flex items-center justify-end gap-2 border-t border-[var(--brand-line)] pt-4 border-[var(--brand-line)]">
        <AppButton
          v-if="order.status === 'PENDING'"
          @click="emit('cancel', order)"
          variant="warning"
          size="sm"
        >
          {{ t('payment.orders.cancel') }}
        </AppButton>
        <AppButton
          v-if="order.status === 'FAILED'"
          @click="emit('retry', order)"
          variant="secondary"
          size="sm"
        >
          {{ t('payment.admin.retry') }}
        </AppButton>
        <AppButton
          v-if="canRefund(order)"
          @click="emit('refund', order)"
          variant="danger"
          size="sm"
        >
          {{ t('payment.admin.refund') }}
        </AppButton>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AppButton from '@/components/common/Button.vue'
import type { PaymentOrder } from '@/types/payment'
import { statusBadgeClass, canRefund as canRefundStatus, formatOrderDateTime } from '@/components/payment/orderUtils'
import { currencySymbol } from '@/components/payment/currency'

const { t } = useI18n()

const props = defineProps<{
  show: boolean
  order: PaymentOrder | null
}>()

const creditedAmountSymbol = currencySymbol('USD')

const paymentAmountSymbol = computed(() => currencySymbol(props.order?.currency))

/** 充值金额 (base amount before fee) = pay_amount - fee = pay_amount / (1 + fee_rate/100) */
const baseAmount = computed(() => {
  if (!props.order) return 0
  const feeRate = Number(props.order.fee_rate) || 0
  if (feeRate <= 0) return props.order.pay_amount
  return props.order.pay_amount / (1 + feeRate / 100)
})

/** 手续费 = pay_amount - baseAmount */
const feeAmount = computed(() => {
  if (!props.order) return 0
  const feeRate = Number(props.order.fee_rate) || 0
  if (feeRate <= 0) return 0
  return props.order.pay_amount - baseAmount.value
})

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'cancel', order: PaymentOrder): void
  (e: 'retry', order: PaymentOrder): void
  (e: 'refund', order: PaymentOrder): void
}>()

function canRefund(order: PaymentOrder): boolean {
  return canRefundStatus(order.status)
}

function formatDateTime(dateStr: string): string {
  return formatOrderDateTime(dateStr)
}
</script>
