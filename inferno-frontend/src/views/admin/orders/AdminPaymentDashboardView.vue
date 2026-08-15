<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Header with Day Switcher -->
      <div class="payment-toolbar">
        <div class="payment-toolbar__controls">
          <Segmented
            v-model="daysModel"
            :items="dayRangeItems"
            :aria-label="t('payment.admin.reportingWindow')"
          />
          <IconButton
            icon="hgi-arrow-reload-horizontal"
            :label="t('common.refresh')"
            :loading="loading"
            :disabled="loading"
            class="payment-refresh"
            @click="loadDashboard"
          />
        </div>
      </div>

      <!-- Dashboard Content -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>
      <template v-else-if="stats">
        <OrderStatsCards :stats="stats" />
        <DailyRevenueChart :data="stats.daily_series || []" :loading="loading" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <PaymentMethodChart :methods="stats.payment_methods || []" />
          <TopUsersLeaderboard :users="stats.top_users || {}" />
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { DashboardStats } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import IconButton from '@/components/common/IconButton.vue'
import Segmented from '@/components/common/Segmented.vue'
import OrderStatsCards from '@/components/admin/payment/OrderStatsCards.vue'
import DailyRevenueChart from '@/components/admin/payment/DailyRevenueChart.vue'
import PaymentMethodChart from '@/components/admin/payment/PaymentMethodChart.vue'
import TopUsersLeaderboard from '@/components/admin/payment/TopUsersLeaderboard.vue'

const { t } = useI18n()
const appStore = useAppStore()

const DAYS_OPTIONS = [7, 30, 90] as const
const days = ref<number>(30)
const loading = ref(false)
const stats = ref<DashboardStats | null>(null)

const dayRangeItems = computed(() => DAYS_OPTIONS.map(value => ({
  value: String(value),
  label: `${value}${t('payment.admin.daySuffix')}`,
})))

const daysModel = computed<string>({
  get: () => String(days.value),
  set: value => {
    days.value = Number(value)
  },
})

async function loadDashboard() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getDashboard(days.value)
    stats.value = res.data
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    loading.value = false
  }
}

watch(days, () => loadDashboard())
onMounted(() => loadDashboard())
</script>

<style scoped>
.payment-toolbar {
  display: flex;
  justify-content: flex-end;
}

.payment-toolbar__controls {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-2);
  max-width: 100%;
}

.payment-refresh {
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

.payment-refresh:hover:not(:disabled) {
  background: var(--brand-tint);
  color: var(--foreground);
}

.payment-refresh:focus-visible {
  outline: 2px solid var(--focus-ring);
  outline-offset: 2px;
}

.payment-refresh:disabled {
  cursor: wait;
  opacity: 0.6;
}

@media (max-width: 640px) {
  .payment-toolbar__controls {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
