<template>
  <section class="payment-stats" :aria-label="t('payment.admin.reportingCurrency')">
    <div class="payment-stats__toolbar">
      <div class="payment-stats__intro">
        <span class="payment-stats__eyebrow">{{ t('payment.admin.reportingCurrency') }}</span>
        <span class="payment-stats__caption">{{ t('payment.admin.reportingCurrencyHint') }}</span>
      </div>
      <div class="currency-folio" role="radiogroup" :aria-label="t('payment.admin.reportingCurrency')">
        <button
          v-for="currency in availableCurrencies"
          :key="currency"
          type="button"
          class="currency-folio__sheet"
          :class="{ 'currency-folio__sheet--active': currency === activeCurrency }"
          role="radio"
          :aria-checked="currency === activeCurrency"
          :aria-label="`${currency} · ${amountFor(stats.total_amount, currency)}`"
          :title="amountFor(stats.total_amount, currency)"
          @click="selectCurrency(currency)"
          @keydown="handleCurrencyKeydown($event, currency)"
        >
          <span class="currency-folio__label">{{ currency }}</span>
        </button>
      </div>
    </div>
    <span class="payment-stats__announce" aria-live="polite">{{ t('payment.admin.currencyChanged', { currency: activeCurrency }) }}</span>

    <div class="grid grid-cols-2 items-start gap-4 lg:grid-cols-4">
      <!-- Today Revenue -->
      <StatTile :label="t('payment.admin.todayRevenue')" :value="summaryValue(stats.today_amount) || '0'" :context="`${stats.today_count} ${t('payment.admin.orders')}`" icon="dollar-01" tone="success">
        <template #value>
          <span class="payment-stat__amount">{{ amountFor(stats.today_amount, activeCurrency, stats.today_count === 0 ? '0' : 'N/A') }}</span>
        </template>
      </StatTile>

      <!-- Total Revenue -->
      <StatTile :label="t('payment.admin.totalRevenue')" :value="summaryValue(stats.total_amount)" :context="`${stats.total_count} ${t('payment.admin.orders')}`" icon="credit-card" tone="brand">
        <template #value>
          <span class="payment-stat__amount">{{ amountFor(stats.total_amount, activeCurrency) }}</span>
        </template>
      </StatTile>

      <!-- Today Orders -->
      <StatTile :label="t('payment.admin.todayOrders')" :value="stats.today_count.toLocaleString()" :context="t('payment.admin.orders')" icon="chart-01" tone="brand" />

      <!-- Average Amount -->
      <StatTile :label="t('payment.admin.avgAmount')" :value="summaryValue(stats.avg_amount)" :context="t('payment.admin.orders')" icon="chart-01" tone="accent">
        <template #value>
          <span class="payment-stat__amount">{{ amountFor(stats.avg_amount, activeCurrency) }}</span>
        </template>
      </StatTile>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import StatTile from '@/components/common/StatTile.vue'
import type { CurrencyAmounts, DashboardStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  stats: DashboardStats
}>()

const currencyStorageKey = 'inferno.payment-dashboard.currency'
const selectedCurrency = ref<string | null>(readStoredCurrency())

const availableCurrencies = computed(() =>
  Array.from(
    new Set([
      ...Object.keys(props.stats.today_amount),
      ...Object.keys(props.stats.total_amount),
      ...Object.keys(props.stats.avg_amount),
    ])
  ).sort()
)

const activeCurrency = computed(() => {
  if (selectedCurrency.value && availableCurrencies.value.includes(selectedCurrency.value)) return selectedCurrency.value
  return availableCurrencies.value[0] || 'USD'
})

watch(activeCurrency, currency => {
  selectedCurrency.value = currency
})

watch(selectedCurrency, currency => {
  if (typeof window === 'undefined' || !currency) return
  try {
    window.localStorage.setItem(currencyStorageKey, currency)
  } catch {
    // Private browsing and blocked storage should not break the dashboard.
  }
})

function sortedAmounts(amounts: CurrencyAmounts): [string, number][] {
  return Object.entries(amounts).sort(([left], [right]) => left.localeCompare(right))
}

function summaryValue(amounts: CurrencyAmounts): string {
  return sortedAmounts(amounts).map(([currency, amount]) => formatMoney(currency, amount)).join(' | ')
}

function amountFor(amounts: CurrencyAmounts, currency: string, emptyValue = 'N/A'): string {
  const amount = amounts[currency]
  return amount == null ? emptyValue : formatMoney(currency, amount)
}

function selectCurrency(currency: string): void {
  selectedCurrency.value = currency
}

function handleCurrencyKeydown(event: KeyboardEvent, currency: string): void {
  const index = availableCurrencies.value.indexOf(currency)
  if (index < 0) return

  let nextIndex = index
  if (event.key === 'ArrowRight' || event.key === 'ArrowDown') nextIndex = (index + 1) % availableCurrencies.value.length
  if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') nextIndex = (index - 1 + availableCurrencies.value.length) % availableCurrencies.value.length
  if (event.key === 'Home') nextIndex = 0
  if (event.key === 'End') nextIndex = availableCurrencies.value.length - 1
  if (nextIndex === index) return

  event.preventDefault()
  selectCurrency(availableCurrencies.value[nextIndex])
}

function readStoredCurrency(): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(currencyStorageKey)
  } catch {
    return null
  }
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency, currencyDisplay: 'code' }).format(amount)
}
</script>

<style scoped>
.payment-stats {
  display: grid;
  gap: 16px;
}

.payment-stats__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  min-width: 0;
}

.payment-stats__intro {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.payment-stats__eyebrow {
  color: var(--foreground);
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  letter-spacing: -0.01em;
}

.payment-stats__caption {
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
}

.payment-stats__announce {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.currency-folio {
  display: flex;
  align-items: stretch;
  gap: 0;
  max-width: 100%;
  padding: 4px;
  overflow-x: auto;
  border: 1px solid color-mix(in oklch, var(--foreground) 6%, transparent);
  border-radius: var(--r-pill);
  background: color-mix(in oklch, var(--foreground) 5%, var(--card));
  scrollbar-width: none;
}

.currency-folio::-webkit-scrollbar {
  display: none;
}

.currency-folio__sheet {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 76px;
  min-height: var(--control-md);
  padding: 0 var(--sp-4);
  border: 0;
  border-radius: var(--r-pill);
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
  cursor: pointer;
  transition: background var(--motion-hover), color var(--motion-hover), box-shadow var(--motion-hover);
}

.currency-folio__sheet:hover {
  color: var(--foreground);
}

.currency-folio__sheet:focus-visible {
  outline: 2px solid var(--foreground);
  outline-offset: 2px;
}

.currency-folio__sheet--active {
  background: var(--card);
  color: var(--foreground);
  box-shadow: 0 1px 4px color-mix(in oklch, var(--foreground) 10%, transparent);
}

@media (max-width: 640px) {
  .payment-stats__toolbar {
    align-items: stretch;
    flex-direction: column;
    gap: 10px;
  }

  .currency-folio {
    align-self: stretch;
  }

  .currency-folio__sheet {
    min-width: 0;
    flex: 1 0 0;
  }
}

.payment-stat__amount {
  display: block;
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-2xl);
  font-weight: 400;
  line-height: 1.15;
}
</style>
