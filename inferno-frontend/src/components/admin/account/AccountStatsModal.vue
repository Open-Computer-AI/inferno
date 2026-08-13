<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.usageStatistics')"
    width="extra-wide"
    @close="handleClose"
  >
    <div class="space-y-6">
      <!-- Account Info Header -->
      <div
        v-if="account"
        class="flex items-center justify-between rounded-xl border border-primary-200 bg-gradient-to-r from-primary-50 to-primary-100 p-3 dark:border-primary-700/50 dark:from-primary-900/20 dark:to-primary-800/20"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
          >
            <Icon name="chartBar" size="md" class="text-white" />
          </div>
          <div>
            <div class="font-semibold text-gray-900 dark:text-gray-100">{{ account.name }}</div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.last30DaysUsage') }}
            </div>
          </div>
        </div>
        <span
          :class="[
            'rounded-full px-2.5 py-1 text-xs font-semibold',
            account.status === 'active'
              ? 'bg-green-100 text-green-700 dark:bg-green-500/20 dark:text-green-400'
              : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
          ]"
        >
          {{ account.status }}
        </span>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- Row 1: Main Stats Cards -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <!-- 30-Day Total Cost -->
          <div
            class="card border-emerald-200 bg-gradient-to-br from-emerald-50 to-white p-4 dark:border-emerald-800/30 dark:from-emerald-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.totalCost')
              }}</span>
              <div class="rounded-lg bg-emerald-100 p-1.5 dark:bg-emerald-900/30">
                <Icon name="dollar" size="sm" class="text-emerald-600 dark:text-emerald-400" />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              ${{ formatCost(stats.summary.total_cost) }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.accumulatedCost') }}
              <span class="text-gray-400 dark:text-gray-500">
                ({{ t('usage.userBilled') }}: ${{ formatCost(stats.summary.total_user_cost) }} ·
                {{ t('admin.accounts.stats.standardCost') }}: ${{
                  formatCost(stats.summary.total_standard_cost)
                }})
              </span>
            </p>
          </div>

          <!-- 30-Day Total Requests -->
          <div
            class="card border-blue-200 bg-gradient-to-br from-blue-50 to-white p-4 dark:border-blue-800/30 dark:from-blue-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.totalRequests')
              }}</span>
              <div class="rounded-lg bg-blue-100 p-1.5 dark:bg-blue-900/30">
                <Icon name="bolt" size="sm" class="text-blue-600 dark:text-blue-400" />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              {{ formatNumber(stats.summary.total_requests) }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.totalCalls') }}
            </p>
          </div>

          <!-- Daily Average Cost -->
          <div
            class="card border-amber-200 bg-gradient-to-br from-amber-50 to-white p-4 dark:border-amber-800/30 dark:from-amber-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.avgDailyCost')
              }}</span>
              <div class="rounded-lg bg-amber-100 p-1.5 dark:bg-amber-900/30">
                <Icon
                  name="calculator"
                  size="sm"
                  class="text-amber-600 dark:text-amber-400"
                />
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              ${{ formatCost(stats.summary.avg_daily_cost) }}
            </p>
             <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{
                t('admin.accounts.stats.basedOnActualDays', {
                  days: stats.summary.actual_days_used
                })
              }}
              <span class="text-gray-400 dark:text-gray-500">
                ({{ t('usage.userBilled') }}: ${{ formatCost(stats.summary.avg_daily_user_cost) }})
              </span>
            </p>
          </div>

          <!-- Daily Average Requests -->
          <div
            class="card border-purple-200 bg-gradient-to-br from-purple-50 to-white p-4 dark:border-purple-800/30 dark:from-purple-900/10 dark:to-dark-700"
          >
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{
                t('admin.accounts.stats.avgDailyRequests')
              }}</span>
              <div class="rounded-lg bg-purple-100 p-1.5 dark:bg-purple-900/30">
                <svg
                  class="h-4 w-4 text-purple-600 dark:text-purple-400"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M7 12l3-3 3 3 4-4M8 21l4-4 4 4M3 4h18M4 4h16v12a1 1 0 01-1 1H5a1 1 0 01-1-1V4z"
                  />
                </svg>
              </div>
            </div>
            <p class="text-2xl font-bold text-gray-900 dark:text-white">
              {{ formatNumber(Math.round(stats.summary.avg_daily_requests)) }}
            </p>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.stats.avgDailyUsage') }}
            </p>
          </div>
        </div>

        <!-- Row 2: Today, Highest Cost, Highest Requests -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Today Overview -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-cyan-100 p-1.5 dark:bg-cyan-900/30">
                <Icon name="clock" size="sm" class="text-cyan-600 dark:text-cyan-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.todayOverview')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.user_cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.today?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.tokens')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                }}</span>
              </div>
            </div>
          </div>

          <!-- Highest Cost Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-orange-100 p-1.5 dark:bg-orange-900/30">
                <Icon name="fire" size="sm" class="text-orange-600 dark:text-orange-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.highestCostDay')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.date')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  stats.summary.highest_cost_day?.label || '-'
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') }}</span>
                <span class="text-sm font-semibold text-orange-600 dark:text-orange-400"
                  >${{ formatCost(stats.summary.highest_cost_day?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_cost_day?.user_cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.highest_cost_day?.requests || 0)
                }}</span>
              </div>
            </div>
          </div>

          <!-- Highest Request Day -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-indigo-100 p-1.5 dark:bg-indigo-900/30">
                <Icon
                  name="trendingUp"
                  size="sm"
                  class="text-indigo-600 dark:text-indigo-400"
                />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.highestRequestDay')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.date')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  stats.summary.highest_request_day?.label || '-'
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.requests')
                }}</span>
                <span class="text-sm font-semibold text-indigo-600 dark:text-indigo-400">{{
                  formatNumber(stats.summary.highest_request_day?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.accountBilled') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_request_day?.cost || 0) }}</span
                >
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('usage.userBilled') }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.highest_request_day?.user_cost || 0) }}</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Row 3: Token Stats -->
        <div class="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <!-- Accumulated Tokens -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-teal-100 p-1.5 dark:bg-teal-900/30">
                <Icon name="cube" size="sm" class="text-teal-600 dark:text-teal-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.accumulatedTokens')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.totalTokens')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.total_tokens)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.dailyAvgTokens')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(Math.round(stats.summary.avg_daily_tokens))
                }}</span>
              </div>
            </div>
          </div>

          <!-- Performance -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-rose-100 p-1.5 dark:bg-rose-900/30">
                <Icon name="bolt" size="sm" class="text-rose-600 dark:text-rose-400" />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.performance')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.avgResponseTime')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatDuration(stats.summary.avg_duration_ms)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.daysActive')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >{{ stats.summary.actual_days_used }} / {{ stats.summary.days }}</span
                >
              </div>
            </div>
          </div>

          <!-- Recent Activity -->
          <div class="card p-4">
            <div class="mb-3 flex items-center gap-2">
              <div class="rounded-lg bg-lime-100 p-1.5 dark:bg-lime-900/30">
                <Icon
                  name="clipboard"
                  size="sm"
                  class="text-lime-600 dark:text-lime-400"
                />
              </div>
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                t('admin.accounts.stats.recentActivity')
              }}</span>
            </div>
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayRequests')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatNumber(stats.summary.today?.requests || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayTokens')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white">{{
                  formatTokens(stats.summary.today?.tokens || 0)
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-xs text-gray-500 dark:text-gray-400">{{
                  t('admin.accounts.stats.todayCost')
                }}</span>
                <span class="text-sm font-semibold text-gray-900 dark:text-white"
                  >${{ formatCost(stats.summary.today?.cost || 0) }}</span
                >
              </div>
            </div>
          </div>
        </div>

        <!-- Usage trend: three strips, not three overlaid lines on two axes.
             Account cost and user cost share a unit but do not sum (they are
             two sides of one transaction), and requests is a count. Each
             strip's peak is printed in its head so the two USD series stay
             numerically comparable despite independent scales. -->
        <div class="astat__trend">
          <h3 class="astat__trend-title">{{ t('admin.accounts.stats.usageTrend') }}</h3>

          <template v-if="trendModel">
            <div v-for="strip in trendModel.strips" :key="strip.key" class="astat__strip">
              <div class="astat__strip-head">
                <span class="astat__legend">
                  <span class="astat__swatch" :style="{ background: strip.color }" aria-hidden="true" />
                  {{ strip.label }}
                </span>
                <span class="astat__peak">{{ strip.peak }}</span>
              </div>
              <div class="astat__plot">
                <DitherArea
                  :series="strip.series"
                  :labels="trendModel.labels"
                  :rest-color="trendRestColor"
                  :marker-color="trendTokens.brand"
                />
              </div>
            </div>
          </template>

          <p v-else class="astat__trend-empty">{{ t('admin.dashboard.noDataAvailable') }}</p>
        </div>

        <!-- Model Distribution -->
        <ModelDistributionChart :model-stats="stats.models" :loading="false" />

        <EndpointDistributionChart
          :endpoint-stats="stats.endpoints || []"
          :loading="false"
          :title="t('usage.inboundEndpoint')"
        />

        <EndpointDistributionChart
          :endpoint-stats="stats.upstream_endpoints || []"
          :loading="false"
          :title="t('usage.upstreamEndpoint')"
        />
      </template>

      <!-- No Data State -->
      <div
        v-else-if="!loading"
        class="flex flex-col items-center justify-center py-12 text-gray-500 dark:text-gray-400"
      >
        <Icon name="chartBar" size="xl" class="mb-4 h-12 w-12" />
        <p class="text-sm">{{ t('admin.accounts.stats.noData') }}</p>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button
          @click="handleClose"
          class="rounded-lg bg-gray-100 px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-300 dark:hover:bg-dark-500"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DitherArea, { type AreaSeries } from '@/components/charts/DitherArea.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useChartTokens } from '@/composables/useChartTokens'
import type { Account, AccountUsageStatsResponse } from '@/types'


const { t } = useI18n()

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const loading = ref(false)
const stats = ref<AccountUsageStatsResponse | null>(null)

// Dark mode detection

// Line chart data
const { tokens: trendTokens } = useChartTokens()

const trendRestColor = computed(
  () => `color-mix(in oklch, ${trendTokens.mutedForeground} 26%, ${trendTokens.card})`
)

/*
 * Was three <Line> datasets on a dual y-axis: account cost and user cost in USD
 * on the left, request count on the right. Neither pairing can share a scale
 * honestly -- the two costs are two sides of one transaction and do not sum to
 * anything, and a count shares no unit with a dollar amount. A second y-axis
 * also implies a correlation the data does not carry: where the cost line meets
 * the request line depends only on how the ranges happened to be scaled.
 *
 * Three strips, each with its own scale. The tradeoff is that comparing account
 * cost against user cost is no longer a matter of reading which line sits
 * higher, so each strip prints its peak -- the numbers stay comparable even
 * though the heights are scaled independently.
 */
const trendModel = computed(() => {
  const history = stats.value?.history
  if (!history?.length) return null

  const labels = history.map((h) => h.label)
  const accountCost = history.map((h) => h.actual_cost ?? 0)
  const userCost = history.map((h) => h.user_cost ?? 0)
  const requests = history.map((h) => h.requests ?? 0)
  const peak = (xs: number[]) => (xs.length ? Math.max(...xs) : 0)

  const strip = (key: string, label: string, values: number[], i: number, peakLabel: string) => ({
    key,
    label,
    color: trendTokens.ramp[i] ?? trendTokens.brand,
    peak: peakLabel,
    series: [
      { key, label, values, color: trendTokens.ramp[i] ?? trendTokens.brand }
    ] satisfies AreaSeries[]
  })

  return {
    labels,
    strips: [
      strip('accountCost', `${t('usage.accountBilled')} (USD)`, accountCost, 0, `$${formatCost(peak(accountCost))}`),
      strip('userCost', `${t('usage.userBilled')} (USD)`, userCost, 1, `$${formatCost(peak(userCost))}`),
      strip('requests', t('admin.accounts.stats.requests'), requests, 2, formatNumber(peak(requests)))
    ]
  }
})

// Load stats when modal opens
watch(
  () => props.show,
  async (newVal) => {
    if (newVal && props.account) {
      await loadStats()
    } else {
      stats.value = null
    }
  }
)

const loadStats = async () => {
  if (!props.account) return

  loading.value = true
  try {
    stats.value = await adminAPI.accounts.getStats(props.account.id, 30)
  } catch (error) {
    console.error('Failed to load account stats:', error)
    stats.value = null
  } finally {
    loading.value = false
  }
}

const handleClose = () => {
  emit('close')
}

// Format helpers
const formatCost = (value: number): string => {
  if (value >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  } else if (value >= 1) {
    return value.toFixed(2)
  } else if (value >= 0.01) {
    return value.toFixed(3)
  }
  return value.toFixed(4)
}

const formatNumber = (value: number): string => {
  if (value >= 1_000_000) {
    return (value / 1_000_000).toFixed(2) + 'M'
  } else if (value >= 1_000) {
    return (value / 1_000).toFixed(2) + 'K'
  }
  return value.toLocaleString()
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}
</script>

<style scoped>
/* Only the usage-trend block is converted here; the rest of this modal is
   still on the legacy palette and is counted as unconverted by
   conversion-status.mjs, which is honest -- it is not done yet. */
.astat__trend {
  padding: 16px;
  border: var(--border-width) solid var(--border-subtle);
  border-radius: var(--r-lg);
  background: var(--card);
}

.astat__trend-title {
  margin: 0 0 12px;
  color: var(--foreground);
  font-size: var(--fs-lg);
  font-weight: var(--fw-medium);
}

.astat__trend-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 160px;
  margin: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-md);
}

.astat__strip + .astat__strip {
  margin-top: 12px;
  padding-top: 12px;
  border-top: var(--border-width) solid var(--border-subtle);
}

.astat__strip-head {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.astat__legend {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--muted-foreground);
  font-size: var(--fs-sm);
}

.astat__swatch {
  flex: none;
  width: 9px;
  height: 9px;
  border-radius: 3px;
}

/* The peak is what keeps the two USD strips comparable once each one scales
   independently. */
.astat__peak {
  color: var(--foreground);
  font-family: var(--font-serif);
  font-size: var(--fs-lg);
  font-variant-numeric: tabular-nums;
}

.astat__plot {
  position: relative;
  height: 92px;
  border-radius: 5px;
}
</style>
