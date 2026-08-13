<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <StatTile
      :label="t('usage.totalRequests')"
      :value="formatNumber(stats?.total_requests || 0)"
      :context="t('usage.inSelectedRange')"
    />
    <StatTile :label="t('usage.totalTokens')" :value="formatTokens(stats?.total_tokens || 0)">
      <template #context>
        <span class="tile__foot-text" :title="cacheDetailLabel()">
          {{ t('usage.in') }}: {{ formatTokens(stats?.total_input_tokens || 0) }} /
          {{ t('usage.out') }}: {{ formatTokens(stats?.total_output_tokens || 0) }} /
          {{ cacheLabel() }}: {{ formatTokens(stats?.total_cache_tokens || 0) }}
          <span class="sr-only">
            {{ cacheDetailLabel() }}:
            {{ t('usage.cacheCreationTokensLabel') }} {{ formatTokens(stats?.total_cache_creation_tokens || 0) }},
            {{ t('usage.cacheReadTokensLabel') }} {{ formatTokens(stats?.total_cache_read_tokens || 0) }}
          </span>
          <HelpTooltip width-class="w-56">
            <strong class="mb-2 block">{{ cacheDetailLabel() }}</strong>
            <span class="flex items-center justify-between gap-3">
              <span>{{ t('usage.cacheCreationTokensLabel') }}</span>
              <span class="tabular-nums">{{ formatTokens(stats?.total_cache_creation_tokens || 0) }}</span>
            </span>
            <span class="mt-1 flex items-center justify-between gap-3">
              <span>{{ t('usage.cacheReadTokensLabel') }}</span>
              <span class="tabular-nums">{{ formatTokens(stats?.total_cache_read_tokens || 0) }}</span>
            </span>
          </HelpTooltip>
        </span>
      </template>
    </StatTile>
    <StatTile :label="t('usage.totalCost')" :value="`$${(stats?.total_actual_cost || 0).toFixed(4)}`">
      <template #context>
        <span class="tile__foot-text">
          <template v-if="showAccountCost && totalAccountCost != null">
            {{ t('usage.accountCost') }} ${{ totalAccountCost.toFixed(4) }} ·
          </template>
          {{ t('usage.standardCost') }}
          <span :class="{ 'line-through': strikeStandardCost }">${{ (stats?.total_cost || 0).toFixed(4) }}</span>
        </span>
      </template>
    </StatTile>
    <StatTile
      :label="t('usage.avgDuration')"
      :value="formatDuration(stats?.average_duration_ms || 0)"
      :context="t('usage.inSelectedRange')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import StatTile from '@/components/common/StatTile.vue'

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const formatNumber = (value: number) => value.toLocaleString()
const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

const formatTokens = (value: number) => {
  if (value >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (value >= 1e6) return (value / 1e6).toFixed(2) + 'M'
  if (value >= 1e3) return (value / 1e3).toFixed(2) + 'K'
  return value.toLocaleString()
}

const cacheLabel = () => t('usage.cacheTotal')
const cacheDetailLabel = () => t('usage.cacheBreakdown')
</script>
