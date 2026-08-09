<template>
  <div class="cc">
    <CapacityBar
      size="sm"
      :percent="primary.percent"
      :label="primary.label"
      :trailing="primary.trailing"
      :title="primary.tooltip"
    />
    <button
      v-if="otherDimensions.length"
      type="button"
      class="cc-expand"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <i
        class="hgi-stroke hgi-arrow-down-01 cc-expand__chevron"
        :class="{ 'cc-expand__chevron--open': expanded }"
        aria-hidden="true"
        style="font-size: 9px; /* june-lint-disable ground-rule-4: icon glyph */"
      />
      {{ expanded ? t('admin.accounts.capacity.hideLimits') : t('admin.accounts.capacity.moreLimits', { count: otherDimensions.length }) }}
    </button>
    <div v-if="expanded" class="cc-expandlist">
      <CapacityBar
        v-for="d in otherDimensions"
        :key="d.key"
        size="sm"
        :percent="d.percent"
        :label="d.label"
        :trailing="d.trailing"
        :title="d.tooltip"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * AccountCapacityCell — part 08, kind C ("ratio"), the "restCells" table's
 * entry: "Already a ratio. Loses its own colour scale for the shared
 * threshold rule."
 *
 * Every dimension below (concurrency, 5h window cost, sessions, RPM, and
 * API-key daily/weekly/total quota) used to render as its own
 * CapacityBadge/QuotaBadge, each with a hand-picked Tailwind colour scale
 * (its own red/yellow/orange/emerald thresholds, sometimes with a third
 * "sticky-only" tier baked into the colour rather than the tooltip). Ground
 * rule 5 -- colour encodes state, never category -- and this cell's kind
 * ("a number and a bar, sharing the one threshold rule: brand under 80,
 * attention from 80, destructive over 100") both point at the same fix:
 * one shared threshold, via CapacityBar, same as the usage cell. The
 * multi-tier nuance each metric used to encode in colour (e.g. "sticky
 * sessions only" vs "fully blocked") is not lost, it moves into the
 * tooltip text, which is where a reason belongs (part 01: "colour never
 * carries a reason text cannot").
 *
 * Up to seven dimensions could apply to one account (concurrency, window
 * cost, sessions, RPM, quota daily/weekly/total); stacking all of them
 * would blow well past a 36px row, the same problem AccountUsageCell had
 * with up to four windows. This cell follows the same resolution: one bar
 * for whichever dimension is closest to its limit, the rest one click away.
 */
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import CapacityBar from '@/components/common/CapacityBar.vue'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

interface CapacityDimension {
  key: string
  label: string
  percent: number
  trailing: string
  tooltip?: string
}

const formatCost = (value: number | null | undefined) => {
  if (value === null || value === undefined) return '0.00'
  return value.toFixed(2)
}

// ====== 并发 ======
const currentConcurrency = computed(() => props.account.current_concurrency || 0)
const concurrencyDimension = computed<CapacityDimension>(() => {
  const max = props.account.concurrency || 0
  const current = currentConcurrency.value
  return {
    key: 'concurrency',
    label: t('admin.accounts.capacity.dimension.concurrency'),
    percent: max > 0 ? (current / max) * 100 : 0,
    trailing: `${current} / ${max}`
  }
})

// ====== 窗口费用 ======
const isAnthropicOAuthOrSetupToken = computed(() =>
  props.account.platform === 'anthropic' &&
  (props.account.type === 'oauth' || props.account.type === 'setup-token')
)

const showWindowCost = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.window_cost_limit != null &&
  props.account.window_cost_limit > 0
)

const currentWindowCost = computed(() => props.account.current_window_cost ?? 0)

const windowCostTooltip = computed(() => {
  const current = currentWindowCost.value
  const limit = props.account.window_cost_limit || 0
  const reserve = props.account.window_cost_sticky_reserve || 10
  if (current >= limit + reserve) return t('admin.accounts.capacity.windowCost.blocked')
  if (current >= limit) return t('admin.accounts.capacity.windowCost.stickyOnly')
  return t('admin.accounts.capacity.windowCost.normal')
})

const windowCostDimension = computed<CapacityDimension | null>(() => {
  if (!showWindowCost.value) return null
  const limit = props.account.window_cost_limit || 0
  const current = currentWindowCost.value
  return {
    key: 'window_cost',
    label: t('admin.accounts.capacity.dimension.windowCost'),
    percent: limit > 0 ? (current / limit) * 100 : 0,
    trailing: `$${formatCost(current)} / $${formatCost(limit)}`,
    tooltip: windowCostTooltip.value
  }
})

// ====== 会话限制 ======
const showSessionLimit = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.max_sessions != null &&
  props.account.max_sessions > 0
)

const activeSessions = computed(() => props.account.active_sessions ?? 0)

const sessionLimitTooltip = computed(() => {
  const current = activeSessions.value
  const max = props.account.max_sessions || 0
  const idle = props.account.session_idle_timeout_minutes || 5
  if (current >= max) return t('admin.accounts.capacity.sessions.full', { idle })
  return t('admin.accounts.capacity.sessions.normal', { idle })
})

const sessionDimension = computed<CapacityDimension | null>(() => {
  if (!showSessionLimit.value) return null
  const max = props.account.max_sessions || 0
  const current = activeSessions.value
  return {
    key: 'sessions',
    label: t('admin.accounts.capacity.dimension.sessions'),
    percent: max > 0 ? (current / max) * 100 : 0,
    trailing: `${current} / ${max}`,
    tooltip: sessionLimitTooltip.value
  }
})

// ====== RPM ======
const showRpmLimit = computed(() =>
  isAnthropicOAuthOrSetupToken.value &&
  props.account.base_rpm != null &&
  props.account.base_rpm > 0
)

const currentRPM = computed(() => props.account.current_rpm ?? 0)
const rpmStrategy = computed(() => props.account.rpm_strategy || 'tiered')
const rpmStrategyTag = computed(() => rpmStrategy.value === 'sticky_exempt' ? 'S' : 'T')

const rpmBuffer = computed(() => {
  const base = props.account.base_rpm || 0
  return props.account.rpm_sticky_buffer ?? (base > 0 ? Math.max(1, Math.floor(base / 5)) : 0)
})

const rpmTooltip = computed(() => {
  const current = currentRPM.value
  const base = props.account.base_rpm ?? 0
  const buffer = rpmBuffer.value
  if (rpmStrategy.value === 'tiered') {
    if (current >= base + buffer) return t('admin.accounts.capacity.rpm.tieredBlocked', { buffer })
    if (current >= base) return t('admin.accounts.capacity.rpm.tieredStickyOnly', { buffer })
    if (current >= base * 0.8) return t('admin.accounts.capacity.rpm.tieredWarning')
    return t('admin.accounts.capacity.rpm.tieredNormal')
  }
  if (current >= base) return t('admin.accounts.capacity.rpm.stickyExemptOver')
  if (current >= base * 0.8) return t('admin.accounts.capacity.rpm.stickyExemptWarning')
  return t('admin.accounts.capacity.rpm.stickyExemptNormal')
})

const rpmDimension = computed<CapacityDimension | null>(() => {
  if (!showRpmLimit.value) return null
  const base = props.account.base_rpm ?? 0
  const current = currentRPM.value
  return {
    key: 'rpm',
    label: t('admin.accounts.capacity.dimension.rpm'),
    percent: base > 0 ? (current / base) * 100 : 0,
    trailing: `${current} / ${base} [${rpmStrategyTag.value}]`,
    tooltip: rpmTooltip.value
  }
})

// ====== 配额 ======
const isQuotaEligible = computed(() => props.account.type === 'apikey' || props.account.type === 'bedrock')

const quotaTooltip = (used: number, limit: number) =>
  used >= limit ? t('admin.accounts.capacity.quota.exceeded') : t('admin.accounts.capacity.quota.normal')

const quotaDailyDimension = computed<CapacityDimension | null>(() => {
  const limit = props.account.quota_daily_limit
  if (!isQuotaEligible.value || limit == null || limit <= 0) return null
  const used = props.account.quota_daily_used ?? 0
  return {
    key: 'quota_daily',
    label: t('admin.accounts.capacity.dimension.quotaDaily'),
    percent: (used / limit) * 100,
    trailing: `$${formatCost(used)} / $${formatCost(limit)}`,
    tooltip: quotaTooltip(used, limit)
  }
})

const quotaWeeklyDimension = computed<CapacityDimension | null>(() => {
  const limit = props.account.quota_weekly_limit
  if (!isQuotaEligible.value || limit == null || limit <= 0) return null
  const used = props.account.quota_weekly_used ?? 0
  return {
    key: 'quota_weekly',
    label: t('admin.accounts.capacity.dimension.quotaWeekly'),
    percent: (used / limit) * 100,
    trailing: `$${formatCost(used)} / $${formatCost(limit)}`,
    tooltip: quotaTooltip(used, limit)
  }
})

const quotaTotalDimension = computed<CapacityDimension | null>(() => {
  const limit = props.account.quota_limit
  if (!isQuotaEligible.value || limit == null || limit <= 0) return null
  const used = props.account.quota_used ?? 0
  return {
    key: 'quota_total',
    label: t('admin.accounts.capacity.dimension.quotaTotal'),
    percent: (used / limit) * 100,
    trailing: `$${formatCost(used)} / $${formatCost(limit)}`,
    tooltip: quotaTooltip(used, limit)
  }
})

// Concurrency is unconditional, so this list is never empty.
const dimensions = computed<CapacityDimension[]>(() => {
  return [
    concurrencyDimension.value,
    windowCostDimension.value,
    sessionDimension.value,
    rpmDimension.value,
    quotaDailyDimension.value,
    quotaWeeklyDimension.value,
    quotaTotalDimension.value
  ].filter((d): d is CapacityDimension => d !== null)
})

// Take the max, not the reader: the dimension closest to its own limit is
// the one that can cause an incident, same rule as AccountUsageCell.
const primary = computed<CapacityDimension>(() =>
  dimensions.value.reduce((max, d) => (d.percent > max.percent ? d : max), dimensions.value[0])
)

const otherDimensions = computed<CapacityDimension[]>(() =>
  dimensions.value.filter((d) => d.key !== primary.value.key)
)

const expanded = ref(false)
watch(() => props.account.id, () => { expanded.value = false })
</script>

<style scoped>
.cc {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

/* Every other limit is one click away, same pattern as AccountUsageCell. */
.cc-expand {
  display: inline-flex;
  align-self: flex-start;
  align-items: center;
  gap: 3px;
  border: 0;
  padding: 0;
  background: transparent;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: pointer;
}
.cc-expand:hover {
  color: var(--foreground);
}
.cc-expand:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--focus-ring);
}
.cc-expand__chevron {
  transition: transform var(--t-fast) var(--ease-out);
}
.cc-expand__chevron--open {
  transform: rotate(180deg);
}

.cc-expandlist {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 6px 0 2px;
}
</style>
