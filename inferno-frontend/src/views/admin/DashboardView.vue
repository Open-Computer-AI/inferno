<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <template v-else-if="stats">
        <!-- No PageHeader. The sidebar row is highlighted, the tab title says
             it, and the verdict sentence below is the real headline -- a title
             repeating "Dashboard" costs a line and tells you nothing.
             The page's one opinion. Everything below is the evidence for it. -->
        <DashboardVerdict
          :total-accounts="stats.total_accounts"
          :normal-accounts="stats.normal_accounts"
          :error-accounts="stats.error_accounts"
        />

        <!-- The four numbers that answer "is anything wrong", and only those.
             Keys, users and token totals are inventory and volume, not health,
             so they moved below the fold.

             Each tile carries ONE context line on the tray floor, because
             every number here is unreadable alone: 48,201 requests is neither
             good nor bad until you know the current rate, and $18.40 means
             nothing without the list price beside it. -->
        <div class="tiles">
          <StatTile
            icon="database-01"
            tone="brand"
            to="/admin/accounts"
            :label="t('admin.dashboard.accounts')"
            :value="`${stats.normal_accounts} / ${stats.total_accounts}`"
            :context="accountsContext.text"
            :context-tone="accountsContext.tone"
          />
          <StatTile
            icon="exchange-01"
            tone="info"
            :label="t('admin.dashboard.todayRequests')"
            :value="formatNumber(stats.today_requests)"
            to="/admin/usage"
            :exact="String(stats.today_requests)"
            :delta="requestsDelta"
            :context="t('admin.dashboard.tileRequestsRate', { rate: formatNumber(Math.round(stats.rpm)) })"
          />
          <StatTile
            icon="database-02"
            tone="accent"
            :label="t('admin.dashboard.todayTokens')"
            :value="formatTokens(stats.today_tokens)"
            to="/admin/usage"
            :exact="String(stats.today_tokens)"
            :delta="tokensDelta"
            :context="todayCacheContext"
          />
          <StatTile
            icon="dollar-circle"
            tone="success"
            :label="t('admin.dashboard.todayCost')"
            to="/admin/usage"
            :value="`$${formatCost(stats.today_actual_cost)}`"
            :delta="costDelta"
            delta-polarity="lower-is-better"
            :context="costContext.text"
            :context-tone="costContext.tone"
          />
        </div>

        <!-- Charts Section -->
        <div class="space-y-6">
          <!-- Date Range Filter -->
          <div class="card p-4">
            <div class="flex flex-wrap items-center gap-4">
              <div class="flex items-center gap-2">
                <span class="text-sm text-gray-700 dark:text-gray-300"
                  >{{ t('admin.dashboard.timeRange') }}:</span
                >
                <DateRangePicker
                  v-model:start-date="startDate"
                  v-model:end-date="endDate"
                  @change="onDateRangeChange"
                />
              </div>
              <button @click="loadDashboardStats" :disabled="chartsLoading" class="btn btn-secondary">
                {{ t('common.refresh') }}
              </button>
              <div class="ml-auto flex items-center gap-2">
                <span class="text-sm text-gray-700 dark:text-gray-300"
                  >{{ t('admin.dashboard.granularity') }}:</span
                >
                <div class="w-28">
                  <Select
                    v-model="granularity"
                    :options="granularityOptions"
                    @change="loadChartData"
                  />
                </div>
              </div>
            </div>
          </div>

          <TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" />
        </div>

        <!-- The token mix, as a canvas donut. Part-of-whole by definition:
             input + output + cache write + cache read IS total tokens, and the
             split is what explains the cost tile above. -->
        <div class="panels">
          <TokenMixPanel />
          <ModelMixPanel />
          <TokenTrendPanel class="panels__wide" />
        </div>

        <!-- Below the fold: the other four numbers, then distribution and
             history. Part 14 puts distribution here because "which model is
             popular" is never the answer to "is anything wrong".

             Same tiles, no icon tiles. That absence IS the hierarchy: the
             coloured tiles above answer "is anything wrong", these are
             reference figures, and eight equally loud cards was the original
             dashboard's defect. Each still earns its context line -- a token
             total without its input/output split is a number you cannot act
             on, and "new users today" belongs here as context rather than
             occupying a tile of its own. -->
        <div class="tiles">
          <StatTile
            :label="t('admin.dashboard.apiKeys')"
            :value="formatNumber(stats.total_api_keys)"
            :context="t('admin.dashboard.tileKeysActive', { count: formatNumber(stats.active_api_keys) })"
          />
          <StatTile
            to="/admin/users"
            :label="t('admin.dashboard.users')"
            :value="formatNumber(stats.total_users)"
            :context="usersContext"
          />
          <StatTile
            to="/admin/usage"
            :label="t('admin.dashboard.totalTokens')"
            :value="formatTokens(stats.total_tokens)"
            :exact="String(stats.total_tokens)"
            :context="totalCacheContext"
          />
          <StatTile
            to="/admin/ops"
            :label="t('admin.dashboard.avgResponse')"
            :value="formatDuration(stats.average_duration_ms)"
            :context="t('admin.dashboard.tileAcrossRequests', { count: formatTokens(stats.total_requests) })"
          />
        </div>

        <div class="space-y-6">
          <!-- Charts Grid -->
          <div class="grid grid-cols-1 gap-6">
            <ModelDistributionChart
              :model-stats="modelStats"
              :enable-ranking-view="true"
              :ranking-items="rankingItems"
              :ranking-total-actual-cost="rankingTotalActualCost"
              :ranking-total-requests="rankingTotalRequests"
              :ranking-total-tokens="rankingTotalTokens"
              :loading="chartsLoading"
              :ranking-loading="rankingLoading"
              :ranking-error="rankingError"
              :start-date="startDate"
              :end-date="endDate"
              @ranking-click="goToUserUsage"
            />
          </div>

          <!-- User Usage Trend (Full Width) -->
          <div class="card p-4">
            <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.dashboard.recentUsage') }} (Top 12)
            </h3>
            <div class="h-64">
              <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                <LoadingSpinner size="md" />
              </div>
              <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div
                v-else
                class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.dashboard.noDataAvailable') }}
              </div>
            </div>
          </div>
        <!-- Quick Actions -->
        <div class="card p-4">
          <div class="mb-3 flex items-center justify-between">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('admin.dashboard.quickActions') }}
            </h2>
          </div>
          <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
            <button
              v-if="canUseBatchImage"
              type="button"
              class="qa-row"
              @click="router.push('/batch-image')"
            >
              <span class="qa-icon">
                <Icon name="sparkles" size="md" :stroke-width="2" />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm text-gray-900 dark:text-white">
                  {{ t('admin.dashboard.batchImage') }}
                </span>
                <span class="block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.batchImageDesc') }}
                </span>
              </span>
              <Icon name="chevronRight" size="sm" class="text-gray-400" />
            </button>
            <button
              type="button"
              class="qa-row"
              @click="router.push('/admin/groups')"
            >
              <span class="qa-icon">
                <Icon name="grid" size="md" :stroke-width="2" />
              </span>
              <span class="min-w-0 flex-1">
                <span class="block text-sm text-gray-900 dark:text-white">
                  {{ t('admin.dashboard.groupPricing') }}
                </span>
                <span class="block text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.dashboard.groupPricingDesc') }}
                </span>
              </span>
              <Icon name="chevronRight" size="sm" class="text-gray-400 group-hover:text-emerald-500" />
            </button>
          </div>
        </div>

        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
import { adminAPI } from '@/api/admin'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  UserUsageTrendPoint,
  UserSpendingRankingItem
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import StatTile from '@/components/common/StatTile.vue'
import TokenMixPanel from '@/components/admin/TokenMixPanel.vue'
import ModelMixPanel from '@/components/admin/ModelMixPanel.vue'
import TokenTrendPanel from '@/components/admin/TokenTrendPanel.vue'
import DashboardVerdict from '@/components/admin/DashboardVerdict.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
import { latestHourOn, sumWindow, toDelta } from '@/utils/dayOverDay'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
)

const appStore = useAppStore()
const router = useRouter()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const chartsLoading = ref(false)
const userTrendLoading = ref(false)
const rankingLoading = ref(false)
const rankingError = ref(false)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const userTrend = ref<UserUsageTrendPoint[]>([])
const rankingItems = ref<UserSpendingRankingItem[]>([])
const rankingTotalActualCost = ref(0)
const rankingTotalRequests = ref(0)
const rankingTotalTokens = ref(0)
let chartLoadSeq = 0
let usersTrendLoadSeq = 0
let rankingLoadSeq = 0
const rankingLimit = 12

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

const getLast24HoursRangeDates = (): { start: string; end: string } => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return {
    start: formatLocalDate(start),
    end: formatLocalDate(end)
  }
}

// Date range
const granularity = ref<'day' | 'hour'>('hour')
const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') }
])

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

// Line chart options (for user trend chart)
const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      itemSort: (a: any, b: any) => {
        const aValue = typeof a?.raw === 'number' ? a.raw : Number(a?.parsed?.y ?? 0)
        const bValue = typeof b?.raw === 'number' ? b.raw : Number(b?.parsed?.y ?? 0)
        return bValue - aValue
      },
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    }
  }
}))

// User trend chart data
const userTrendChartData = computed(() => {
  if (!userTrend.value?.length) return null

  const getDisplayName = (point: UserUsageTrendPoint): string => {
    const username = point.username?.trim()
    if (username) {
      return username
    }

    const email = point.email?.trim()
    if (email) {
      return email
    }

    return t('admin.redeem.userPrefix', { id: point.user_id })
  }

  // Group by user_id to avoid merging different users with the same display name
  const userGroups = new Map<number, { name: string; data: Map<string, number> }>()
  const allDates = new Set<string>()

  userTrend.value.forEach((point) => {
    allDates.add(point.date)
    const key = point.user_id
    if (!userGroups.has(key)) {
      userGroups.set(key, { name: getDisplayName(point), data: new Map() })
    }
    userGroups.get(key)!.data.set(point.date, point.tokens)
  })

  const sortedDates = Array.from(allDates).sort()
  const colors = [
    '#3b82f6',
    '#10b981',
    '#f59e0b',
    '#ef4444',
    '#8b5cf6',
    '#ec4899',
    '#14b8a6',
    '#f97316',
    '#6366f1',
    '#84cc16',
    '#06b6d4',
    '#a855f7'
  ]

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]}20`,
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

// Format helpers
const formatTokens = (value: number | undefined): string => {
  if (value === undefined || value === null) return '0'
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

const formatNumber = (value: number | null | undefined): string => {
  return toFiniteNumber(value).toLocaleString()
}

const formatCost = (value: number | null | undefined): string => {
  const safeValue = toFiniteNumber(value)
  /* Exact zero is not a small number, it is no spend, and the 4-decimal branch
     rendered it as "$0.0000" -- four digits of false precision on the most
     common state a fresh instance is in. */
  if (safeValue === 0) {
    return '0.00'
  }
  if (safeValue >= 1000) {
    return (safeValue / 1000).toFixed(2) + 'K'
  } else if (safeValue >= 1) {
    return safeValue.toFixed(2)
  } else if (safeValue >= 0.01) {
    return safeValue.toFixed(3)
  }
  return safeValue.toFixed(4)
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}

/*
 * The two tray-floor lines that are conditional. The other two are direct
 * reads (rpm, tpm) and stay inline in the template.
 *
 * Accounts is the only tile whose context can change tone: it restates the
 * verdict's evidence, so it takes ink for the same reason the verdict does.
 */
/*
 * Cache share, not the input/output split.
 *
 * The split looked right and was quietly useless: total_tokens includes cache
 * creation and cache read, so "Input 853.9M / Output 40.6M" under a 12.36B
 * headline accounts for under a tenth of it. Nothing is mislabelled, but
 * anyone who adds the two numbers is left wondering where the rest went, and
 * the answer -- cache -- was the interesting part all along.
 *
 * Cache reads are the cheap tokens, so this ratio is what actually explains
 * the cost tile beside it. Reads only: cache CREATION is written at a premium
 * and counting it as "served from cache" would flatter the number.
 *
 * Shown on both today and lifetime deliberately, so the two are comparable --
 * today 93% against a lifetime 88% says cache is improving, which neither
 * figure says alone.
 */
function cacheShare(cacheRead: number, total: number): string {
  const safeTotal = toFiniteNumber(total)
  if (safeTotal <= 0) return t('admin.dashboard.tileNoTraffic')
  const pct = (toFiniteNumber(cacheRead) / safeTotal) * 100
  // One decimal below 10%: the difference between 0.4% and 4% is worth seeing,
  // and above 10% the decimal is noise.
  return t('admin.dashboard.tileCacheShare', {
    pct: pct < 10 ? pct.toFixed(1) : String(Math.round(pct))
  })
}

const todayCacheContext = computed(() =>
  stats.value ? cacheShare(stats.value.today_cache_read_tokens, stats.value.today_tokens) : ''
)

const totalCacheContext = computed(() =>
  stats.value ? cacheShare(stats.value.total_cache_read_tokens, stats.value.total_tokens) : ''
)

/*
 * Restores "new users today", which was a whole tile on the original
 * dashboard. As a headline it was noise -- a single day's signups is not a
 * system reading -- but as context under the user count it is exactly the
 * right size, and it costs no tile. Dropped entirely on a day with no signups
 * rather than printing "0 new today", which reads as a problem.
 */
const usersContext = computed(() => {
  const s = stats.value
  if (!s) return ''
  const active = formatNumber(s.active_users)
  if (s.today_new_users > 0) {
    return t('admin.dashboard.tileUsersActive', { active, new: formatNumber(s.today_new_users) })
  }
  return t('admin.dashboard.tileUsersActiveOnly', { active })
})

type ContextTone = 'muted' | 'good' | 'attention'

/*
 * LIKE-FOR-LIKE, which is the whole difficulty. Comparing today's running
 * total against yesterday's finished total is the standard dashboard bug: at
 * 14:00 today is 58% of a day and yesterday is 100% of one, so every metric
 * reads "down 40%" all day. sumWindow bounds both sides at the same hour;
 * see src/utils/dayOverDay.ts for why the buckets are sliced, not parsed.
 */
const deltaTrend = ref<TrendDataPoint[]>([])

/*
 * The hour both comparison windows are bounded at.
 *
 * NOT simply the browser's hour: buckets are stamped in the DATABASE's
 * timezone, so a browser behind the DB would discard the most recent buckets.
 * Taking the max with the latest bucket actually present today self-corrects
 * either offset -- see latestHourOn.
 */
const boundHour = computed(() => {
  const today = formatLocalDate(new Date())
  return Math.max(new Date().getHours(), latestHourOn(deltaTrend.value, today))
})

const dayOverDay = computed(() => {
  if (deltaTrend.value.length === 0) return null
  const now = new Date()
  const hour = boundHour.value
  return {
    today: sumWindow(deltaTrend.value, formatLocalDate(now), hour),
    yesterday: sumWindow(
      deltaTrend.value,
      formatLocalDate(new Date(now.getTime() - 24 * 60 * 60 * 1000)),
      hour
    )
  }
})

const requestsDelta = computed(() =>
  dayOverDay.value ? toDelta(dayOverDay.value.today.requests, dayOverDay.value.yesterday.requests) : null
)
const tokensDelta = computed(() =>
  dayOverDay.value ? toDelta(dayOverDay.value.today.tokens, dayOverDay.value.yesterday.tokens) : null
)
const costDelta = computed(() =>
  dayOverDay.value ? toDelta(dayOverDay.value.today.cost, dayOverDay.value.yesterday.cost) : null
)

const accountsContext = computed<{ text: string; tone: ContextTone }>(() => {
  const s = stats.value
  if (!s) return { text: '', tone: 'muted' }
  if (s.error_accounts > 0) {
    return {
      text: t('admin.dashboard.tileAccountsAttention', { count: s.error_accounts }),
      tone: 'attention'
    }
  }
  /* Not 'good'. Zero accounts is not a healthy system, it is an unconfigured
     one, and a green check on "none connected yet" would congratulate the
     operator for having nothing to serve traffic with. */
  if (s.total_accounts === 0) return { text: t('admin.dashboard.tileAccountsEmpty'), tone: 'muted' }
  return { text: t('admin.dashboard.tileAccountsHealthy'), tone: 'good' }
})

/*
 * `today_cost` is the standard list price, `today_actual_cost` is what was
 * actually deducted; the headline shows the latter because that is the number
 * that leaves the account. The difference is the group discount, and it is
 * only worth a line when it is non-zero -- "saved $0.00" is noise.
 */
const costContext = computed<{ text: string; tone: ContextTone }>(() => {
  const s = stats.value
  if (!s) return { text: '', tone: 'muted' }
  const saved = s.today_cost - s.today_actual_cost
  /* Paying list price is not a fault, so it gets no mark -- only the discount
     earns one, because it is the case that could have gone the other way. */
  if (saved <= 0.005) return { text: t('admin.dashboard.tileCostStandard'), tone: 'muted' }
  return {
    text: t('admin.dashboard.tileCostSaved', {
      standard: `$${formatCost(s.today_cost)}`,
      saved: `$${formatCost(saved)}`
    }),
    tone: 'good'
  }
})

const goToUserUsage = (item: UserSpendingRankingItem) => {
  void router.push({
    path: '/admin/usage',
    query: {
      user_id: String(item.user_id),
      start_date: startDate.value,
      end_date: endDate.value
    }
  })
}

// Date range change handler
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  // Auto-select granularity based on date range
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  // If range is 1 day, use hourly granularity
  if (daysDiff <= 1) {
    granularity.value = 'hour'
  } else {
    granularity.value = 'day'
  }

  loadChartData()
}

// Load data
const loadDashboardSnapshot = async (includeStats: boolean) => {
  const currentSeq = ++chartLoadSeq
  if (includeStats && !stats.value) {
    loading.value = true
  }
  chartsLoading.value = true
  try {
    const response = await adminAPI.dashboard.getSnapshotV2({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      include_stats: includeStats,
      include_trend: true,
      include_model_stats: true,
      include_group_stats: false,
      include_users_trend: false
    })
    if (currentSeq !== chartLoadSeq) return
    if (includeStats && response.stats) {
      stats.value = response.stats
    }
    trendData.value = response.trend || []
    modelStats.value = response.models || []
  } catch (error) {
    if (currentSeq !== chartLoadSeq) return
    appStore.showError(t('admin.dashboard.failedToLoad'))
    console.error('Error loading dashboard snapshot:', error)
  } finally {
    if (currentSeq === chartLoadSeq) {
      loading.value = false
      chartsLoading.value = false
    }
  }
}

const loadUsersTrend = async () => {
  const currentSeq = ++usersTrendLoadSeq
  userTrendLoading.value = true
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      limit: 12
    })
    if (currentSeq !== usersTrendLoadSeq) return
    userTrend.value = response.trend || []
  } catch (error) {
    if (currentSeq !== usersTrendLoadSeq) return
    console.error('Error loading users trend:', error)
    userTrend.value = []
  } finally {
    if (currentSeq === usersTrendLoadSeq) {
      userTrendLoading.value = false
    }
  }
}

const loadUserSpendingRanking = async () => {
  const currentSeq = ++rankingLoadSeq
  rankingLoading.value = true
  rankingError.value = false
  try {
    const response = await adminAPI.dashboard.getUserSpendingRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: rankingLimit
    })
    if (currentSeq !== rankingLoadSeq) return
    rankingItems.value = response.ranking || []
    rankingTotalActualCost.value = response.total_actual_cost || 0
    rankingTotalRequests.value = response.total_requests || 0
    rankingTotalTokens.value = response.total_tokens || 0
  } catch (error) {
    if (currentSeq !== rankingLoadSeq) return
    console.error('Error loading user spending ranking:', error)
    rankingItems.value = []
    rankingTotalActualCost.value = 0
    rankingTotalRequests.value = 0
    rankingTotalTokens.value = 0
    rankingError.value = true
  } finally {
    if (currentSeq === rankingLoadSeq) {
      rankingLoading.value = false
    }
  }
}

const loadDashboardStats = async () => {
  await Promise.all([
    loadDashboardSnapshot(true),
    loadUsersTrend(),
    loadUserSpendingRanking(),
    loadDayOverDay()
  ])
}

/*
 * Fixed to yesterday-through-now, and deliberately NOT tied to the range
 * picker. The tiles it feeds all say "Today", and they read `stats`, which is
 * itself range-independent -- so a delta that moved with the picker would
 * silently start comparing today against a week ago while the label still
 * said today. One extra call, no backend change.
 */
const loadDayOverDay = async () => {
  const now = new Date()
  try {
    const response = await adminAPI.dashboard.getSnapshotV2({
      /*
       * TWO days back, not one, to fetch a day we do not display.
       *
       * The backend filters the range in the CLIENT's timezone but labels the
       * buckets in the DATABASE's. Asking for yesterday from a viewer 2.5h
       * behind the DB starts the window at yesterday 02:30 DB-time, so the
       * first three labelled buckets of the comparison day are missing -- the
       * baseline is understated and every delta reads too high. Observed
       * exactly that: a true +21% rendered as +25%.
       *
       * Widening the request is the whole fix, because sumWindow and
       * densifyHours both select by the label date string. The extra day is
       * fetched and then ignored; it exists only to push the clipped edge
       * outside the two days we actually read.
       */
      start_date: formatLocalDate(new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000)),
      end_date: formatLocalDate(now),
      granularity: 'hour',
      include_stats: false,
      include_trend: true,
      include_model_stats: false,
      include_group_stats: false,
      include_users_trend: false
    })
    deltaTrend.value = response.trend || []
  } catch {
    /* A missing comparison is not a page error. The tiles simply render
       without a chip, which is the same as a day with no prior traffic. */
    deltaTrend.value = []
  }
}

const loadChartData = async () => {
  await Promise.all([
    loadDashboardSnapshot(false),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

onMounted(() => {
  void refreshBatchImageAccess()
  loadDashboardStats()
})
</script>

<style scoped>
/* Both panels carry their own tray, so they sit side by side and each keeps a
   natural width: a 200px ring beside a four-row legend, and a four-column plot,
   both look marooned stretched across a 1600px content column. auto-fit drops
   them to one per row before either gets too narrow to read. */
.panels {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: 16px;
}

/* The trend spans the row: a time series wants length, and squeezing 90 days
   into a half-width plot puts the cell columns closer together than the daily
   points they sample, which loses the shape it exists to show. */
.panels__wide {
  grid-column: 1 / -1;
}

/* The fold. auto-fit rather than a fixed 4 so the tiles reflow to 2x2 and
   then 1-up without a media query: below about 240px a tile truncates its
   own number, which is the one thing it exists to show. */
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 12px;
}

/* Quick actions. One treatment for every action -- the old per-action hues
   (sky for batch image, emerald for group pricing) were colour encoding a
   category, which ground rule 5 reserves for state.

   That was only half done: the row background lost its hue but the icon tile
   inside kept bg-sky-100 / bg-emerald-100, so a green square still sat on the
   dashboard meaning nothing. .qa-icon below is the one treatment the comment
   above always claimed. */
.qa-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  border-radius: var(--r-md);
  background: var(--surface-subtle);
  text-align: left;
  cursor: pointer;
  /* Background only, never border-color. */
  transition: background var(--motion-hover);
}

.qa-row:hover {
  background: var(--sidebar-accent);
}

.qa-icon {
  display: grid;
  place-items: center;
  flex: none;
  width: 40px;
  height: 40px;
  border-radius: var(--r-md);
  background: var(--muted);
  color: var(--muted-foreground);
}
</style>
