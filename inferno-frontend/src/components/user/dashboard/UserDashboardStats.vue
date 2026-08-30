<template>
  <div class="space-y-4">
    <div class="dashboard-tiles">
      <StatTile v-if="!isSimple" icon="dollar-circle" tone="success" :label="t('dashboard.balance')" :value="`$${formatBalance(balance)}`" :context="t('common.available')" />
      <StatTile icon="key-01" tone="info" :label="t('dashboard.apiKeys')" :value="formatNumber(stats.total_api_keys)" :context="`${formatNumber(stats.active_api_keys)} ${t('common.active')}`" to="/keys" />
      <StatTile icon="chart" tone="brand" :label="t('dashboard.todayRequests')" :value="formatNumber(stats.today_requests)" :context="`${t('common.total')}: ${formatNumber(stats.total_requests)}`" to="/usage" />
      <StatTile icon="dollar-circle" tone="accent" :label="t('dashboard.todayCost')" :value="`$${formatCost(stats.today_actual_cost)}`" :context="`${t('dashboard.standard')}: $${formatCost(stats.today_cost)} · ${t('common.total')}: $${formatCost(stats.total_actual_cost)}`" to="/usage" />
    </div>

    <div class="dashboard-tiles">
      <StatTile :label="t('dashboard.todayTokens')" :value="formatTokens(stats.today_tokens)" :context="`${t('dashboard.input')}: ${formatTokens(stats.today_input_tokens)} · ${t('dashboard.output')}: ${formatTokens(stats.today_output_tokens)} · ${t('dashboard.cache')}: ${formatTokens((stats.today_cache_creation_tokens || 0) + (stats.today_cache_read_tokens || 0))}`" to="/usage" />
      <StatTile :label="t('dashboard.totalTokens')" :value="formatTokens(stats.total_tokens)" :context="`${t('dashboard.input')}: ${formatTokens(stats.total_input_tokens)} · ${t('dashboard.output')}: ${formatTokens(stats.total_output_tokens)} · ${t('dashboard.cache')}: ${formatTokens((stats.total_cache_creation_tokens || 0) + (stats.total_cache_read_tokens || 0))}`" to="/usage" />
      <StatTile :label="t('dashboard.performance')" :value="`${formatTokens(stats.rpm)} RPM`" :context="`${formatTokens(stats.tpm)} TPM`" />
      <StatTile :label="t('dashboard.avgResponse')" :value="formatDuration(stats.average_duration_ms)" :context="t('dashboard.averageTime')" />
    </div>

    <section v-if="!isSimple && platformCards.length > 0" class="dashboard-panel platform-breakdown">
      <header class="dashboard-panel__header">
        <div>
          <h3 class="dashboard-panel__title">{{ t('dashboard.platformBreakdown') }}</h3>
          <p class="dashboard-panel__description">{{ t('dashboard.platformCount', { count: sortedPlatforms.length }) }}</p>
        </div>
      </header>
      <div class="platform-breakdown__grid">
        <article v-for="item in platformCards" :key="item.platform" class="platform-breakdown__item" :data-other="item.isOther || undefined">
          <div class="platform-breakdown__head">
            <span class="platform-breakdown__name">{{ item.isOther ? t('dashboard.platformOther') : platformLabel(item.platform) }}</span>
            <span class="platform-breakdown__total" :title="t('dashboard.actual')">${{ formatCost(item.total_actual_cost) }}</span>
          </div>
          <dl class="platform-breakdown__metrics">
            <div><dt>{{ t('dashboard.todayCost') }}</dt><dd>${{ formatCost(item.today_actual_cost) }}</dd></div>
            <div><dt>{{ t('dashboard.requests') }}</dt><dd>{{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}</dd></div>
            <div><dt>{{ t('dashboard.tokens') }}</dt><dd>{{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}</dd></div>
          </dl>

          <div v-if="hasAnyLimit(item.quota) && !item.isOther" class="platform-breakdown__quota">
            <p class="platform-breakdown__eyebrow">{{ t('dashboard.platformQuota.title') }}</p>
            <template v-for="w in (['daily', 'weekly', 'monthly'] as const)" :key="w">
              <div v-if="quotaVal(item.quota, `${w}_limit_usd`) != null" class="platform-breakdown__window">
                <template v-if="(quotaVal(item.quota, `${w}_limit_usd`) as number) === 0">
                  <div class="platform-breakdown__window-head"><span>{{ t(`dashboard.platformQuota.${w}`) }}</span><strong data-status="critical">{{ t('dashboard.platformQuota.disabled') }}</strong></div>
                  <div class="platform-breakdown__bar"><span data-status="critical" style="width:100%" /></div>
                </template>
                <template v-else>
                  <div class="platform-breakdown__window-head"><span>{{ t(`dashboard.platformQuota.${w}`) }}</span><span>${{ formatUsd((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0) }} / ${{ formatUsd(quotaVal(item.quota, `${w}_limit_usd`) as number) }}</span></div>
                  <div class="platform-breakdown__bar"><span :data-status="calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) >= 95 ? 'critical' : calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) >= 75 ? 'warning' : 'ok'" :style="{ width: calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) + '%' }" /></div>
                  <p v-if="quotaVal(item.quota, `${w}_window_resets_at`)" class="platform-breakdown__reset">{{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(quotaVal(item.quota, `${w}_window_resets_at`) as string) }) }}</p>
                </template>
              </div>
            </template>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import StatTile from '@/components/common/StatTile.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

interface FusedPlatformCard {
  platform: string
  total_actual_cost: number
  today_actual_cost: number
  total_requests: number
  total_tokens: number
  isOther?: boolean
  quota?: PlatformQuotaItem
}

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  platformQuotas?: PlatformQuotaItem[] | null
}>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity'
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

const sortedPlatforms = computed(() => {
  const list = props.stats?.by_platform ?? []
  return [...list].sort((a, b) => b.total_actual_cost - a.total_actual_cost)
})

// 处理"各平台之和 < 总值"的差值：后端按平台聚合时过滤了无法归属平台的行
// （group 与 account 都缺 platform）。这里把差值作为"其他"卡片显式展示，
// 避免 Row 1 总值与 Row 3 平台拆分加总对不上、用户困惑。
const OTHER_THRESHOLD = 0.0001
const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, (typeof sortedPlatforms.value)[number]>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // union 平台集合。后端 by_platform / quota 接口均不会返回 platform='__other__'，
  // 无需显式排除；__other__ 由下方差值补差逻辑单独追加。
  const platforms = new Set<string>([...byPlat.keys(), ...byQuota.keys()])

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
  const cards: FusedPlatformCard[] = []

  for (const p of platforms) {
    const stat = byPlat.get(p)
    cards.push({
      platform: p,
      total_actual_cost: stat?.total_actual_cost ?? 0,
      today_actual_cost: stat?.today_actual_cost ?? 0,
      total_requests: stat?.total_requests ?? 0,
      total_tokens: stat?.total_tokens ?? 0,
      quota: byQuota.get(p),
    })
  }

  // 排序：按 PLATFORM_ORDER，未知平台按名称排序
  cards.sort((a, b) => {
    const ai = PLATFORM_ORDER.indexOf(a.platform)
    const bi = PLATFORM_ORDER.indexOf(b.platform)
    if (ai === -1 && bi === -1) return a.platform.localeCompare(b.platform)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  // __other__ 补差逻辑：只对 by_platform 有 usage 数据的总和计算
  const total = props.stats?.total_actual_cost ?? 0
  const today = props.stats?.today_actual_cost ?? 0
  const sumTotal = cards.reduce((s, c) => s + c.total_actual_cost, 0)
  const sumToday = cards.reduce((s, c) => s + c.today_actual_cost, 0)
  const diffTotal = Math.max(0, total - sumTotal)
  const diffToday = Math.max(0, today - sumToday)

  if (diffTotal > OTHER_THRESHOLD || diffToday > OTHER_THRESHOLD) {
    cards.push({
      platform: '__other__',
      total_actual_cost: diffTotal,
      today_actual_cost: diffToday,
      total_requests: 0,
      total_tokens: 0,
      isOther: true,
    })
  }

  return cards
})

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'
type QuotaField = `${QuotaWindow}_limit_usd` | `${QuotaWindow}_usage_usd` | `${QuotaWindow}_window_resets_at`

function quotaVal(q: PlatformQuotaItem | undefined, key: QuotaField): PlatformQuotaItem[QuotaField] {
  return q?.[key]
}

function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
}

function calcPercent(usage: number, limit: number): number {
  if (!limit || limit <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((usage / limit) * 100)))
}

// 与 formatBalance 一致使用 Intl.NumberFormat 做半偶舍入，避免 toFixed 在不同 JS 引擎
// 下偶发截断而非四舍五入（与后端展示精度不一致）。
const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
function formatUsd(n: number): string {
  if (!Number.isFinite(n)) return '0.00'
  return usdFormatter.format(n)
}

function formatResetTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`
</script>

<style scoped>
.platform-breakdown__grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}
.platform-breakdown__item{min-width:0;padding:12px;border:1px solid var(--border-subtle);border-radius:var(--r-md);background:var(--surface-subtle)}
.platform-breakdown__item[data-other="true"]{border-style:dashed}
.platform-breakdown__head,.platform-breakdown__window-head{display:flex;align-items:baseline;justify-content:space-between;gap:12px}
.platform-breakdown__name{color:var(--foreground);font-size:var(--fs-md);font-weight:var(--fw-medium)}
.platform-breakdown__total{color:var(--warm-strong);font-family:var(--font-mono);font-size:var(--fs-sm);font-variant-numeric:tabular-nums}
.platform-breakdown__metrics{display:grid;gap:6px;margin:12px 0 0}
.platform-breakdown__metrics>div{display:flex;align-items:baseline;justify-content:space-between;gap:12px;font-size:var(--fs-sm)}
.platform-breakdown__metrics dt{color:var(--muted-foreground)}
.platform-breakdown__metrics dd{margin:0;color:var(--foreground);font-family:var(--font-mono);font-variant-numeric:tabular-nums}
.platform-breakdown__quota{display:grid;gap:10px;margin-top:12px;padding-top:10px;border-top:1px solid var(--border-subtle)}
.platform-breakdown__eyebrow{margin:0;color:var(--muted-foreground);font-size:var(--fs-2xs);letter-spacing:.08em;text-transform:uppercase}
.platform-breakdown__window{display:grid;gap:4px;color:var(--muted-foreground);font-size:var(--fs-xs)}
.platform-breakdown__window-head span:last-child,.platform-breakdown__window-head strong{color:var(--foreground);font-family:var(--font-mono);font-weight:400;font-variant-numeric:tabular-nums}
.platform-breakdown__window-head strong[data-status="critical"]{color:var(--destructive)}
.platform-breakdown__bar{height:6px;overflow:hidden;border-radius:var(--r-pill);background:var(--muted)}
.platform-breakdown__bar span{display:block;height:100%;border-radius:inherit;background:var(--success);transition:width var(--motion-hover),background-color var(--motion-hover)}
.platform-breakdown__bar span[data-status="warning"]{background:var(--warm-strong)}
.platform-breakdown__bar span[data-status="critical"]{background:var(--destructive)}
.platform-breakdown__reset{margin:0;color:var(--muted-foreground);font-size:var(--fs-2xs)}
</style>
