<template>
  <section class="payment-panel" :aria-label="t('payment.admin.paymentDistribution')">
    <header class="payment-panel__header">
      <div>
        <p class="payment-panel__eyebrow">{{ t('payment.admin.paymentDistribution') }}</p>
        <h3 class="payment-panel__title">{{ t('payment.admin.paymentMethods') }}</h3>
      </div>
      <span class="payment-panel__summary">{{ t('payment.admin.paymentMethodsSummary', { count: methods.length }) }}</span>
    </header>

    <div v-if="!methods?.length" class="payment-panel__empty">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else class="payment-methods">
      <article v-for="method in methods" :key="method.type" class="payment-method">
        <header class="payment-method__header">
          <div class="payment-method__name">
            <span class="payment-method__dot" :data-method="method.type" aria-hidden="true" />
            <span>{{ t('payment.methods.' + method.type, method.type) }}</span>
          </div>
          <div class="payment-method__meta">
            <span v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="payment-method__amount">
              {{ formatMoney(currency, amount) }}
            </span>
            <span class="payment-method__count">{{ method.count }} {{ t('payment.admin.orders') }}</span>
          </div>
        </header>

        <div class="payment-method__bars">
          <div v-for="[currency, amount] in sortedAmounts(method.amount)" :key="currency" class="payment-method__bar-row">
            <span class="payment-method__currency">{{ currency }}</span>
            <div class="payment-method__track" role="presentation">
              <span class="payment-method__fill" :data-method="method.type" :style="{ width: `${barWidth(currency, amount)}%` }" />
            </div>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CurrencyAmounts, PaymentMethodStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  methods: PaymentMethodStats[]
}>()

const maxAmounts = computed<CurrencyAmounts>(() =>
  props.methods.reduce<CurrencyAmounts>((maximums, method) => {
    for (const [currency, amount] of Object.entries(method.amount)) {
      maximums[currency] = Math.max(maximums[currency] || 0, amount)
    }
    return maximums
  }, {})
)

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}

function barWidth(currency: string, amount: number): number {
  return Math.min((amount / (maxAmounts.value[currency] || 1)) * 100, 100)
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(amount)
}
</script>

<style scoped>
.payment-panel {
  min-width: 0;
  padding: var(--sp-5);
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-xl);
  background: var(--card);
}

.payment-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--sp-4);
  padding-bottom: var(--sp-4);
  border-bottom: var(--border-width) solid var(--border-subtle);
}

.payment-panel__eyebrow {
  margin: 0 0 var(--sp-1);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  letter-spacing: 0.04em;
}

.payment-panel__title {
  margin: 0;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.payment-panel__summary {
  flex: none;
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  text-align: right;
}

.payment-panel__empty {
  display: grid;
  min-height: 160px;
  place-items: center;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.payment-methods {
  display: grid;
  gap: var(--sp-4);
  padding-top: var(--sp-4);
}

.payment-method {
  display: grid;
  gap: var(--sp-3);
}

.payment-method + .payment-method {
  padding-top: var(--sp-4);
  border-top: var(--border-width) solid var(--border-subtle);
}

.payment-method__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--sp-4);
}

.payment-method__name {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: var(--sp-2);
  color: var(--foreground);
  font-size: var(--fs-md);
  font-weight: var(--fw-medium);
}

.payment-method__dot {
  flex: none;
  width: var(--sp-3);
  height: var(--sp-3);
  border-radius: var(--r-pill);
  background: var(--brand);
}

.payment-method__dot[data-method*='wxpay'] {
  background: var(--success);
}

.payment-method__meta {
  display: grid;
  justify-items: end;
  gap: var(--sp-1);
  text-align: right;
}

.payment-method__amount {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-lg);
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
}

.payment-method__count {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.payment-method__bars {
  display: grid;
  gap: var(--sp-2);
}

.payment-method__bar-row {
  display: grid;
  grid-template-columns: 32px minmax(0, 1fr);
  align-items: center;
  gap: var(--sp-2);
}

.payment-method__currency {
  color: var(--muted-foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
}

.payment-method__track {
  height: var(--sp-2);
  overflow: hidden;
  border-radius: var(--r-pill);
  background: var(--brand-tint);
}

.payment-method__fill {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: var(--brand);
  transition: width var(--motion-layout);
}

.payment-method__fill[data-method*='wxpay'] {
  background: var(--success);
}

@media (max-width: 640px) {
  .payment-panel__header,
  .payment-method__header {
    flex-direction: column;
  }

  .payment-panel__summary {
    text-align: left;
  }

  .payment-method__meta {
    justify-items: start;
    text-align: left;
  }
}
</style>
