<script setup lang="ts">
/**
 * UserPlatformQuotaCell — table cells part 08, kind C ("Ratio").
 *
 * Migration note from the prototype (restCells, "UserPlatformQuotaCell"):
 *   "Renders one bar per platform stacked. Becomes the closest limit, same
 *    rule as the usage cell."
 *
 * The literal note asks for one bar per configured platform, stacked. A user
 * can have up to five platforms configured (`PLATFORM_ORDER`), and five
 * stacked ratio rows do not fit a 36px table row — the same problem part 08
 * section 02 solved for AccountUsageCell by collapsing every window down to
 * the single one closest to its limit. This cell applies that identical
 * rule one level up: across every configured platform and every window on
 * it (daily/weekly/monthly), the single window closest to its limit is
 * shown as one bar, and the rest collapse into a "+N platforms" chip whose
 * title lists them. The row this cell lives in already opens a full
 * breakdown modal on click (wired by the caller), so nothing here is a dead
 * end — see the report.
 *
 * CapacityBar is composed for the bar itself (sm, 6px, continuous — the one
 * size with room in a table cell) rather than reimplemented; only the
 * surrounding label/count layout is local, because CapacityBar's optional
 * label row adds vertical height this cell cannot spend twice.
 *
 * Prop is unchanged (`quotas?: PlatformQuotaItem[]`).
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import CapacityBar from '@/components/common/CapacityBar.vue'
import type { PlatformQuotaItem, PlatformQuotaPlatform } from '@/api/admin/users'

const props = defineProps<{ quotas?: PlatformQuotaItem[] }>()
const { t } = useI18n()

const PLATFORM_ORDER: PlatformQuotaPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']

type Window = 'daily' | 'weekly' | 'monthly'
const WINDOWS: Window[] = ['daily', 'weekly', 'monthly']

// 仅展示「至少一档限额非空」的平台（配额列，非用量列）
const configured = computed(() => {
  if (!props.quotas) return []
  return props.quotas
    .filter(
      (q) =>
        q.daily_limit_usd != null ||
        q.weekly_limit_usd != null ||
        q.monthly_limit_usd != null
    )
    .slice()
    .sort((a, b) => PLATFORM_ORDER.indexOf(a.platform) - PLATFORM_ORDER.indexOf(b.platform))
})

interface WindowRatio {
  platform: PlatformQuotaPlatform
  window: Window
  percent: number
  usage: number
  limit: number
}

// Every computable ratio, across every configured platform and window,
// sorted closest-to-limit first — the same ordering rule AccountUsageCell
// uses to pick the one window that can cause an incident.
const allRatios = computed<WindowRatio[]>(() => {
  const ratios: WindowRatio[] = []
  for (const row of configured.value) {
    for (const w of WINDOWS) {
      const limit = row[`${w}_limit_usd`]
      if (limit == null || limit <= 0) continue
      const usage = row[`${w}_usage_usd`]
      ratios.push({ platform: row.platform, window: w, percent: (usage / limit) * 100, usage, limit })
    }
  }
  return ratios.sort((a, b) => b.percent - a.percent)
})

const closest = computed(() => allRatios.value[0] ?? null)

// Other configured platforms not represented by the closest-limit bar.
const otherPlatformCount = computed(() => {
  if (!closest.value) return configured.value.length
  const shown = new Set([closest.value.platform])
  return configured.value.filter((row) => !shown.has(row.platform)).length
})

const windowLabel = (window: Window) => t(`admin.users.platformQuota.window${window.charAt(0).toUpperCase()}${window.slice(1)}`)

const overflowTitle = computed(() => {
  if (!closest.value) return ''
  return configured.value
    .filter((row) => row.platform !== closest.value?.platform)
    .map((row) => row.platform)
    .join(', ')
})

const thresholdInk = (percent: number) => {
  if (percent > 100) return 'var(--destructive)'
  if (percent >= 80) return 'var(--s2a-attn)'
  return 'var(--foreground)'
}
</script>

<template>
  <div v-if="props.quotas === undefined" class="upq upq--loading">
    <span class="upq__skel" />
  </div>
  <span v-else-if="configured.length === 0" class="upq__empty">
    {{ t('admin.users.platformQuota.cellNotConfigured') }}
  </span>
  <div v-else-if="closest" class="upq">
    <span class="upq__platform">{{ closest.platform }}</span>
    <span class="upq__window">{{ windowLabel(closest.window) }}</span>
    <CapacityBar :percent="closest.percent" size="sm" class="upq__bar" />
    <span class="upq__percent" :style="{ color: thresholdInk(closest.percent) }">{{ Math.round(closest.percent) }}%</span>
    <span v-if="otherPlatformCount > 0" class="upq__more" :title="overflowTitle">
      +{{ otherPlatformCount }}
    </span>
  </div>
  <span v-else class="upq__empty">-</span>
</template>

<style scoped>
.upq {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.upq--loading {
  min-width: 64px;
}

.upq__skel {
  display: block;
  width: 44%;
  height: 8px;
  border-radius: var(--r-xs);
  background: var(--surface-subtle);
}

.upq__platform {
  flex-shrink: 0;
  color: var(--foreground);
  font-family: var(--font-mono);
  font-size: var(--fs-2xs);
}

.upq__window {
  flex-shrink: 0;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
}

.upq__bar {
  width: 56px;
  flex-shrink: 0;
}

.upq__percent {
  flex-shrink: 0;
  width: 32px;
  font-size: var(--fs-sm);
}

.upq__more {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  height: 18px;
  padding: 0 6px;
  border: 1px solid var(--border-subtle);
  border-radius: var(--r-pill);
  background: var(--card);
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  cursor: default;
}

.upq__empty {
  font-size: var(--fs-xs);
  color: var(--muted-foreground);
}
</style>
