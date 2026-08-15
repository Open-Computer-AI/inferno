<template>
  <section class="payment-panel" :aria-label="t('payment.admin.topUsers')">
    <header class="payment-panel__header">
      <div>
        <p class="payment-panel__eyebrow">{{ t('payment.admin.topUsers') }}</p>
        <h3 class="payment-panel__title">{{ t('payment.admin.customerLeaderboard') }}</h3>
      </div>
      <span class="payment-panel__summary">{{ t('payment.admin.topUsersSummary') }}</span>
    </header>

    <div v-if="!hasUsers(props.users)" class="payment-panel__empty">
      {{ t('payment.admin.noData') }}
    </div>

    <div v-else class="leaderboard">
      <section v-for="[currency, currencyUsers] in sortedUsers(props.users)" :key="currency" class="leaderboard__currency">
        <div class="leaderboard__currency-head">
          <span class="leaderboard__currency-code">{{ currency }}</span>
          <span class="leaderboard__currency-rule" />
        </div>
        <div class="leaderboard__rows">
          <div v-for="(user, idx) in currencyUsers" :key="user.user_id" class="leaderboard__row">
            <span class="leaderboard__rank" :data-rank="idx + 1">{{ idx + 1 }}</span>
            <span class="leaderboard__email" :title="user.email">{{ user.email }}</span>
            <span class="leaderboard__amount">{{ formatMoney(currency, user.amount) }}</span>
          </div>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { TopUserPaymentStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  users: Record<string, TopUserPaymentStats[]>
}>()

function hasUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): boolean {
  return Object.values(usersByCurrency).some(users => users.length > 0)
}

function sortedUsers(usersByCurrency: Record<string, TopUserPaymentStats[]>): [string, TopUserPaymentStats[]][] {
  return Object.entries(usersByCurrency).sort(([left], [right]) => left.localeCompare(right))
}

function formatMoney(currency: string, amount: number): string {
  return new Intl.NumberFormat(undefined, { style: 'currency', currency, currencyDisplay: 'code' }).format(amount)
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

.leaderboard {
  display: grid;
  gap: var(--sp-5);
  padding-top: var(--sp-4);
}

.leaderboard__currency {
  display: grid;
  gap: var(--sp-2);
}

.leaderboard__currency-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}

.leaderboard__currency-code {
  color: var(--brand);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
  letter-spacing: 0.05em;
}

.leaderboard__currency-rule {
  height: var(--border-width);
  flex: 1;
  background: var(--border-subtle);
}

.leaderboard__rows {
  display: grid;
  gap: var(--sp-1);
}

.leaderboard__row {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--sp-3);
  min-width: 0;
  padding: var(--sp-2) var(--sp-3);
  border-radius: var(--r-md);
  transition: background var(--motion-hover);
}

.leaderboard__row:hover {
  background: var(--brand-tint);
}

.leaderboard__rank {
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border-radius: var(--r-pill);
  background: var(--brand-tint);
  color: var(--muted-foreground);
  font-size: var(--fs-xs);
  font-weight: var(--fw-medium);
}

.leaderboard__rank[data-rank='1'] {
  background: color-mix(in oklch, var(--warning) 14%, var(--card));
  color: var(--warning);
}

.leaderboard__email {
  min-width: 0;
  overflow: hidden;
  color: var(--body-copy);
  font-size: var(--fs-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.leaderboard__amount {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-md);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .payment-panel__header {
    flex-direction: column;
  }

  .payment-panel__summary {
    text-align: left;
  }

  .leaderboard__row {
    grid-template-columns: 28px minmax(0, 1fr);
  }

  .leaderboard__amount {
    grid-column: 2;
  }
}
</style>
