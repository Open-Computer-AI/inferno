<script setup lang="ts">
/**
 * GroupBadge — part 03, section 02.
 *
 * Migration note from the prototype:
 *   "Eleven props on a badge, because it carries platform, subscription type,
 *    base rate, a per-user override, a peak window and days remaining. All of
 *    it stays. It stops being six colours and becomes one chip: a platform
 *    dot, the name, and the rate as a muted suffix."
 *   "The six platform colour classes become six dots. Subscription groups
 *    keep their label slot but lose the separate theme colour. The only
 *    tinted variant left is a per-user rate override, because that is the
 *    only one the reader has to act on."
 *   "22px pill: 6px platform dot, name, rate as a muted suffix, then at most
 *    one glyph. The glyph is a person for a personal rate, a clock for a peak
 *    window, a calendar for a subscription. Never two glyphs."
 *
 * The prototype's own dot ramp is a generic --sr-* series (brand-derived,
 * assigned by row position, not by platform), which does not exist as a
 * token anywhere in this repo and would be a second colour mapping alongside
 * the one already approved for this migration: platformAccentColor() from
 * utils/platformColors.ts (product owner approved it for ModelTagInput). The
 * "Legacy pool" row's own note ("not a colour of its own") only makes sense
 * if the dot otherwise IS the platform's colour, so platformAccentColor() is
 * used here instead of inventing --sr-*. See the report for this call.
 *
 * The old PlatformIcon mark inside the chip is dropped: the anatomy line
 * above lists a dot, not an icon, and PlatformIcon.vue is no longer imported.
 *
 * Prop, type and default surface is unchanged from the previous version;
 * every prop in the spec's table is marked "unchanged" except platform,
 * which now selects a dot colour instead of a Tailwind theme class.
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionType, GroupPlatform } from '@/types'
import { useAppStore } from '@/stores/app'
import { formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import { platformAccentColor } from '@/utils/platformColors'

interface Props {
  name: string
  platform?: GroupPlatform
  subscriptionType?: SubscriptionType
  rateMultiplier?: number
  userRateMultiplier?: number | null // 用户专属倍率
  peakRateEnabled?: boolean
  peakStart?: string
  peakEnd?: string
  peakRateMultiplier?: number
  showRate?: boolean
  daysRemaining?: number | null // 剩余天数（订阅类型时使用）
  /**
   * 订阅分组默认在右侧 label 展示"订阅"或剩余天数；
   * 开启后订阅分组也改为显示倍率（保留订阅主题色 label，配合可用渠道这类
   * 只关心费率、不关心有效期的场景）。
   */
  alwaysShowRate?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  subscriptionType: 'standard',
  showRate: true,
  daysRemaining: null,
  userRateMultiplier: null,
  peakRateEnabled: false,
  alwaysShowRate: false
})

const { t } = useI18n()

const isSubscription = computed(() => props.subscriptionType === 'subscription')

// 是否有专属倍率（且与默认倍率不同）
const hasCustomRate = computed(() => {
  return (
    props.userRateMultiplier !== null &&
    props.userRateMultiplier !== undefined &&
    props.rateMultiplier !== undefined &&
    props.userRateMultiplier !== props.rateMultiplier
  )
})

const appStore = useAppStore()

const hasPeakRate = computed(() => {
  return Boolean(props.showRate && props.peakRateEnabled && props.peakStart && props.peakEnd)
})

// Window description for the clock glyph's tooltip. The prototype's note
// says "the rate shown is the one in force right now" during a peak window,
// which would need a live comparison against the server clock; peak-rate.ts
// only formats display strings and exposes no "is it peak right now" check,
// so the suffix keeps showing the base rate and the full window (including
// the peak multiplier) moves into this tooltip instead of guessing the time.
const peakRateText = computed(() => {
  return formatPeakRateWindow(
    {
      peak_rate_enabled: props.peakRateEnabled,
      peak_start: props.peakStart,
      peak_end: props.peakEnd,
      peak_rate_multiplier: props.peakRateMultiplier
    },
    serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)
  )
})

const peakRateTitle = computed(() => {
  return t('common.peakRateTooltip', { window: peakRateText.value })
})

// 是否显示右侧标签
const showLabel = computed(() => {
  if (!props.showRate) return false
  // 订阅类型：显示天数或"订阅"
  if (isSubscription.value) return true
  // 标准类型：显示倍率（包括专属倍率）
  return props.rateMultiplier !== undefined || hasCustomRate.value
})

// Label text
const labelText = computed(() => {
  const rateLabel = props.rateMultiplier !== undefined ? `${props.rateMultiplier}x` : ''
  if (isSubscription.value && !props.alwaysShowRate) {
    // 如果有剩余天数，显示天数
    if (props.daysRemaining !== null && props.daysRemaining !== undefined) {
      if (props.daysRemaining <= 0) {
        return t('admin.users.expired')
      }
      return t('admin.users.daysRemaining', { days: props.daysRemaining })
    }
    // 否则显示"订阅"
    return t('groups.subscription')
  }
  return rateLabel
})

// Dot and name ink both come from the platform's identity colour, the one
// approved exception to "colour never encodes category". No platform (the
// archived / legacy case) drops to neutral: it is deliberately not a colour
// of its own.
const dotColor = computed(() => (props.platform ? platformAccentColor(props.platform) : 'var(--muted-foreground)'))

const nameColor = computed(() => {
  if (hasCustomRate.value) return 'var(--warm-strong)'
  return props.platform ? 'var(--foreground)' : 'var(--muted-foreground)'
})

// Expiry urgency is STATE, not category, so the "colour never encodes category"
// rule does not reach it -- and this chip's own exception ("the only tinted
// variant left is a per-user rate override, because that is the only one the
// reader has to act on") is exactly the test an expiring subscription passes.
// Upstream carried urgency and platform theming in one function; June correctly
// dropped the platform half, and the urgency half went with it by accident.
// Restored on the state tokens, not upstream's bg-red-200/80.
const expiryInk = computed(() => {
  if (!isSubscription.value) return null
  const days = props.daysRemaining
  if (days === null || days === undefined) return null
  if (days <= 3) return 'var(--destructive)'
  if (days <= 7) return 'var(--s2a-attn)'
  return null
})

// Only the personal-rate override takes the tinted chip fill; everything
// else, including peak and subscription, stays on the neutral card fill.
const variant = computed(() => (hasCustomRate.value ? 'personal' : 'default'))

// At most one glyph, in priority order: personal rate, then peak window,
// then subscription. Each tooltip reuses copy already computed above rather
// than inventing new sentences, except the personal-rate one, which had none.
interface Glyph {
  icon: string
  ink: string
  tip: string
}

const glyph = computed<Glyph | null>(() => {
  if (hasCustomRate.value) {
    return { icon: 'hgi-user-circle', ink: 'var(--warm-strong)', tip: t('common.personalRateTooltip') }
  }
  if (hasPeakRate.value) {
    return { icon: 'hgi-clock-01', ink: 'var(--s2a-attn)', tip: peakRateTitle.value }
  }
  if (showLabel.value && isSubscription.value) {
    return { icon: 'hgi-calendar-01', ink: expiryInk.value ?? 'var(--muted-foreground)', tip: labelText.value }
  }
  return null
})
</script>

<template>
  <span class="gbadge" :data-variant="variant">
    <span class="gbadge__dot" :style="{ background: dotColor }" aria-hidden="true" />
    <span class="gbadge__name" :style="{ color: nameColor }">{{ name }}</span>
    <span v-if="showLabel" class="gbadge__rate" :style="expiryInk ? { color: expiryInk } : undefined">{{ labelText }}</span>
    <i
      v-if="glyph"
      :class="['hgi-stroke', glyph.icon]"
      class="gbadge__icon"
      :style="{ color: glyph.ink }"
      :title="glyph.tip"
      aria-hidden="true"
    />
  </span>
</template>

<style scoped>
/* 22px pill, r-pill per the prototype's literal token (not the --r-xs badge
   tier: this is a fully rounded chip, not a rectangular badge/keycap). */
.gbadge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  width: fit-content;
  max-width: 100%;
  height: 22px;
  padding: 0 9px;
  border-radius: var(--r-pill);
  font-size: var(--fs-sm);
  /* Background only, never border-color. */
  transition: background var(--motion-hover);
}

.gbadge[data-variant='default'] {
  border: 1px solid var(--border-subtle);
  background: var(--card);
}

.gbadge[data-variant='personal'] {
  border: 1px solid var(--brand-line-strong);
  background: var(--brand-tint);
}

.gbadge__dot {
  flex-shrink: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.gbadge__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.gbadge__rate {
  flex-shrink: 0;
  color: var(--muted-foreground);
}

.gbadge__icon {
  flex-shrink: 0;
  font-size: 12px; /* june-lint-disable ground-rule-4: icon glyph */
}
</style>
