<script setup lang="ts">
/**
 * UpstreamBillingRateCell — table cells part 08, kind B ("Quantity").
 *
 * Migration note from the prototype (restCells, "UpstreamBillingRateCell"):
 *   "Shows the override in --warm-strong only when it differs from the base
 *    rate."
 *
 * All derivation (staleness, clock-skew tolerance, peak-window arithmetic)
 * is untouched; only the presentation moves off Tailwind utilities and
 * Icon.vue's inline-svg set onto June tokens and the Hugeicons glyph the
 * rest of the app uses. The manual-probe button was a bespoke <button>
 * before; it is IconButton now, composed rather than reimplemented, with a
 * scoped `:deep()` rule to spin its glyph while a probe is in flight since
 * IconButton has no loading state of its own to extend.
 *
 * `--warm-strong` on the rate value is the one approved colour-by-state
 * exception here — the group/user rate mismatch is exactly the case
 * GroupBadge already tints the same way, so this reuses that precedent
 * rather than adding a second "personal override" colour. Stale/failed
 * status text uses the shared attention/destructive tokens; the probe glyph
 * itself stays neutral chrome.
 *
 * Props and the single `probe` emit are unchanged.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import IconButton from '@/components/common/IconButton.vue'
import { formatMultiplier } from '@/utils/formatters'
import type { Account, UpstreamBillingProbeSnapshot } from '@/types'

const props = withDefaults(defineProps<{
  account: Account
  now: number
  probing?: boolean
  globalProbeEnabled?: boolean
}>(), {
  globalProbeEnabled: true
})

defineEmits<{
  (event: 'probe'): void
}>()

const { t } = useI18n()
const CLOCK_SKEW_TOLERANCE_MS = 5 * 60 * 1000
// 探测资格已放宽到全部 API-key 平台（上游是 sub2api 即可应答）。
const eligible = computed(() => props.account.type === 'apikey')
const snapshot = computed<UpstreamBillingProbeSnapshot | undefined>(() => props.account.extra?.upstream_billing_probe)
const data = computed(() => snapshot.value?.data)
const probeEnabled = computed(() => props.account.extra?.upstream_billing_probe_enabled === true)
const nextProbeAt = computed(() => {
  const value = snapshot.value?.next_probe_at
  return typeof value === 'string' && Number.isFinite(Date.parse(value)) ? value : ''
})
const receivedAt = computed(() => typeof snapshot.value?.received_at === 'string' ? Date.parse(snapshot.value.received_at) : Number.NaN)
const freshUntil = computed(() => {
  if (typeof snapshot.value?.fresh_until === 'string') return Date.parse(snapshot.value.fresh_until)
  if (snapshot.value?.status !== 'ok' || typeof snapshot.value.next_probe_at !== 'string') return Number.NaN
  const nextProbeAt = Date.parse(snapshot.value.next_probe_at)
  return Number.isFinite(nextProbeAt) && nextProbeAt > receivedAt.value
    ? receivedAt.value + 2 * (nextProbeAt - receivedAt.value)
    : Number.NaN
})
const validTimestamps = computed(() => {
  if (!Number.isFinite(receivedAt.value) || receivedAt.value > props.now + CLOCK_SKEW_TOLERANCE_MS) return false
  return Number.isFinite(freshUntil.value) && freshUntil.value > receivedAt.value
})
const stale = computed(() => {
  if (!snapshot.value) return false
  if (!Number.isFinite(receivedAt.value)) return snapshot.value.status === 'ok'
  if (!validTimestamps.value) return true
  return props.now > freshUntil.value
})
const parseMinute = (value?: string) => {
  if (typeof value !== 'string') return null
  const match = /^(\d{2}):(\d{2})$/.exec(value)
  if (!match) return null
  const hour = Number(match[1])
  const minute = Number(match[2])
  return hour < 24 && minute < 60 ? hour * 60 + minute : null
}
const minuteInTimeZone = (timestamp: number, timeZone?: string) => {
  if (!timeZone) return null
  try {
    const parts = new Intl.DateTimeFormat('en-GB', {
      timeZone,
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23'
    }).formatToParts(new Date(timestamp))
    const hour = Number(parts.find(part => part.type === 'hour')?.value)
    const minute = Number(parts.find(part => part.type === 'minute')?.value)
    return Number.isInteger(hour) && Number.isInteger(minute) ? hour * 60 + minute : null
  } catch {
    return null
  }
}
const currentEffectiveRate = computed(() => {
  const billing = data.value
  if (!billing) return null
  if (billing.billing_scope !== 'token') return null
  const base = billing.resolved_rate_multiplier
  if (typeof base !== 'number' || !Number.isFinite(base) || base < 0) return null
  if (typeof billing.peak_rate_enabled !== 'boolean') return null
  if (!billing.peak_rate_enabled) return base
  const start = parseMinute(billing.peak_start)
  const end = parseMinute(billing.peak_end)
  const minute = minuteInTimeZone(props.now, billing.timezone)
  const peak = billing.peak_rate_multiplier
  if (start == null || end == null || minute == null || start >= end || typeof peak !== 'number' || !Number.isFinite(peak) || peak < 0) return null
  const value = minute >= start && minute < end ? base * peak : base
  return Number.isFinite(value) ? value : null
})
const lastDetectedRate = computed(() => {
  const value = data.value?.effective_rate_multiplier
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? Number(value.toPrecision(12))
    : null
})
const elapsedSinceLastSuccess = computed(() => {
  if (!Number.isFinite(receivedAt.value)) return '-'
  const elapsedMinutes = Math.max(0, Math.floor((props.now - receivedAt.value) / 60_000))
  if (elapsedMinutes < 1) return t('admin.accounts.upstreamBilling.justNow')
  if (elapsedMinutes < 60) return t('admin.accounts.upstreamBilling.minutesAgo', { count: elapsedMinutes })
  const elapsedHours = Math.floor(elapsedMinutes / 60)
  if (elapsedHours < 24) return t('admin.accounts.upstreamBilling.hoursAgo', { count: elapsedHours })
  return t('admin.accounts.upstreamBilling.daysAgo', { count: Math.floor(elapsedHours / 24) })
})
const effectiveRate = computed(() => {
  if (!validTimestamps.value || stale.value || !['ok', 'failed'].includes(snapshot.value?.status ?? '')) return '-'
  const value = currentEffectiveRate.value
  return value == null ? '-' : `${formatMultiplier(value)}x`
})
const statusLabel = computed(() => {
  if (!snapshot.value) return t('admin.accounts.upstreamBilling.notProbed')
  if (snapshot.value.status === 'unsupported') return t('admin.accounts.upstreamBilling.unsupported')
  if (stale.value) return t('admin.accounts.upstreamBilling.stale')
  if (snapshot.value.status === 'failed') return t('admin.accounts.upstreamBilling.failed')
  return ''
})
// State ink, not category ink: neutral when there is nothing to report,
// attention when the read is stale, destructive when the probe failed.
const statusInk = computed(() => {
  if (!snapshot.value) return 'var(--muted-foreground)'
  if (snapshot.value.status === 'unsupported') return 'var(--muted-foreground)'
  if (stale.value) return 'var(--s2a-attn)'
  if (snapshot.value.status === 'failed') return 'var(--destructive)'
  return 'var(--muted-foreground)'
})
const hasEffectiveRate = computed(() => effectiveRate.value !== '-')
const primaryValue = computed(() => hasEffectiveRate.value ? effectiveRate.value : statusLabel.value || '-')
// Personal-rate override: the one state this cell is allowed to colour, per
// the same rule GroupBadge already applies to a per-user rate.
const hasOverride = computed(() => {
  const billing = data.value
  return Boolean(
    billing
    && billing.user_rate_multiplier != null
    && billing.user_rate_multiplier !== billing.group_rate_multiplier
  )
})
const primaryInk = computed(() => {
  if (!hasEffectiveRate.value) return statusInk.value
  return hasOverride.value ? 'var(--warm-strong)' : 'var(--foreground)'
})
// Subtitle: the base rate the primary figure is measured against, only when
// there is a primary figure to give context to.
const baseRateLabel = computed(() => {
  if (!hasEffectiveRate.value || data.value?.group_rate_multiplier == null) return ''
  return t('admin.accounts.upstreamBilling.groupRate', { value: data.value.group_rate_multiplier })
})
const formatDate = (value?: string) => value
  ? new Date(value).toLocaleString(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  : '-'
</script>

<template>
  <div v-if="eligible" class="ubr">
    <HelpTooltip class="ubr__tooltip" width-class="w-max max-w-[calc(100vw-2rem)]" data-testid="upstream-billing-details">
      <template #trigger>
        <span class="ubr__stack">
          <span class="ubr__primary" data-testid="upstream-billing-rate" :style="{ color: primaryInk }">{{ primaryValue }}</span>
          <span v-if="baseRateLabel" class="ubr__secondary">{{ baseRateLabel }}</span>
        </span>
      </template>
      <div class="ubr__tip">
        <template v-if="hasEffectiveRate && data">
          <p>{{ t('admin.accounts.upstreamBilling.groupRate', { value: data.group_rate_multiplier }) }}</p>
          <p v-if="data.user_rate_multiplier != null">
            {{ t('admin.accounts.upstreamBilling.userRate', { value: data.user_rate_multiplier }) }}
          </p>
          <p>
            {{
              data.peak_rate_enabled
                ? t('admin.accounts.upstreamBilling.peakRate', {
                    start: data.peak_start,
                    end: data.peak_end,
                    value: data.peak_rate_multiplier,
                    timezone: data.timezone
                  })
                : t('admin.accounts.upstreamBilling.noPeakRate')
            }}
          </p>
          <p>{{ t('admin.accounts.upstreamBilling.effectiveRate', { value: currentEffectiveRate ?? '-' }) }}</p>
          <p>{{ t('admin.accounts.upstreamBilling.updatedAt', { value: formatDate(snapshot?.received_at) }) }}</p>
        </template>
        <template v-else-if="stale && lastDetectedRate != null">
          <p data-testid="upstream-billing-last-rate">
            {{ t('admin.accounts.upstreamBilling.lastDetectedRate', { value: lastDetectedRate }) }}
          </p>
          <p data-testid="upstream-billing-last-time">
            {{ t('admin.accounts.upstreamBilling.lastDetectedAt', { value: formatDate(snapshot?.received_at) }) }}
          </p>
          <p data-testid="upstream-billing-elapsed">
            {{ t('admin.accounts.upstreamBilling.elapsedSince', { value: elapsedSinceLastSuccess }) }}
          </p>
        </template>
        <p v-else>{{ statusLabel || '-' }}</p>
        <p
          v-if="probeEnabled && globalProbeEnabled !== false && nextProbeAt"
          data-testid="upstream-billing-next-probe"
        >
          {{ t('admin.accounts.upstreamBilling.nextProbeAt', { value: formatDate(nextProbeAt) }) }}
        </p>
        <p class="ubr__tip-rule" data-testid="upstream-billing-probe-state">
          {{ t('admin.accounts.upstreamBilling.accountProbeState') }}
          <span :style="{ color: probeEnabled ? 'var(--success)' : 'var(--destructive)' }">
            {{ probeEnabled ? t('admin.accounts.upstreamBilling.enabled') : t('admin.accounts.upstreamBilling.disabled') }}
          </span>
        </p>
        <p v-if="globalProbeEnabled === false" class="ubr__tip-note" data-testid="upstream-billing-global-probe-state">
          {{ t('admin.accounts.upstreamBilling.globalProbeState') }}
          <span style="color: var(--destructive)">{{ t('admin.accounts.upstreamBilling.disabled') }}</span>
        </p>
      </div>
    </HelpTooltip>
    <span v-if="hasEffectiveRate && statusLabel" class="ubr__status" :style="{ color: statusInk }">
      {{ statusLabel }}
    </span>
    <IconButton
      icon="hgi-refresh"
      :label="t('admin.accounts.upstreamBilling.manualProbe')"
      size="xs"
      :disabled="probing"
      :class="{ 'ubr__probe--spin': probing }"
      data-testid="upstream-billing-probe"
      @click="$emit('probe')"
    />
  </div>
  <span v-else class="ubr__empty">-</span>
</template>

<style scoped>
.ubr {
  display: flex;
  align-items: center;
  gap: 4px;
  min-height: 24px;
  min-width: 112px;
}

.ubr__tooltip {
  min-width: 0;
}

.ubr__stack {
  display: flex;
  flex-direction: column;
  min-width: 0;
  cursor: help;
  border-bottom: 1px dotted var(--border);
}

.ubr__primary {
  overflow: hidden;
  font-size: var(--fs-sm);
  font-weight: var(--fw-medium);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ubr__secondary {
  overflow: hidden;
  color: var(--muted-foreground);
  font-size: var(--fs-2xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ubr__status {
  flex-shrink: 0;
  font-size: var(--fs-2xs);
  font-weight: var(--fw-medium);
  white-space: nowrap;
}

.ubr__empty {
  font-size: var(--fs-sm);
  color: var(--muted-foreground);
}

/* IconButton has no loading state of its own; spin its glyph from here
   rather than adding one, since only this cell needs it. */
.ubr__probe--spin :deep(i) {
  animation: ubr-spin 0.7s linear infinite;
}
@media (prefers-reduced-motion: reduce) {
  .ubr__probe--spin :deep(i) {
    animation-duration: 0.01ms;
  }
}
@keyframes ubr-spin {
  to {
    transform: rotate(360deg);
  }
}

.ubr__tip {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

/* HelpTooltip's popup is fixed dark-on-white regardless of site theme (its
   own component, not restyled here), so the rule matches that fixed
   surface rather than a theme token. */
.ubr__tip-rule {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(255, 255, 255, 0.15);
}

.ubr__tip-note {
  margin-top: 4px;
}
</style>
