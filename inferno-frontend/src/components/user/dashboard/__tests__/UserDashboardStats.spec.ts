import { describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

import UserDashboardStats from '../UserDashboardStats.vue'
import type { UserDashboardStats as UserStatsType, PlatformDashboardStats } from '@/api/usage'
import type { PlatformQuotaItem } from '@/types'

function makeStats(over: Partial<UserStatsType> = {}): UserStatsType {
  return {
    total_api_keys: 1,
    active_api_keys: 1,
    total_requests: 0,
    total_input_tokens: 0,
    total_output_tokens: 0,
    total_cache_creation_tokens: 0,
    total_cache_read_tokens: 0,
    total_tokens: 0,
    total_cost: 0,
    total_actual_cost: 0,
    today_requests: 0,
    today_input_tokens: 0,
    today_output_tokens: 0,
    today_cache_creation_tokens: 0,
    today_cache_read_tokens: 0,
    today_tokens: 0,
    today_cost: 0,
    today_actual_cost: 0,
    average_duration_ms: 0,
    rpm: 0,
    tpm: 0,
    by_platform: [],
    ...over,
  }
}

function usage(platform: string, cost: number): PlatformDashboardStats {
  return {
    platform,
    total_requests: 1,
    total_tokens: 10,
    total_actual_cost: cost,
    today_requests: 1,
    today_tokens: 10,
    today_actual_cost: cost,
  }
}

function quota(over: Partial<PlatformQuotaItem> & { platform: string }): PlatformQuotaItem {
  return {
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    daily_usage_usd: 0,
    weekly_usage_usd: 0,
    monthly_usage_usd: 0,
    ...over,
  } as PlatformQuotaItem
}

function mountStats(
  stats: UserStatsType,
  platformQuotas: PlatformQuotaItem[] | null = null,
  isSimple = false,
) {
  return mount(UserDashboardStats, {
    props: { stats, balance: 0, isSimple, platformQuotas },
    global: { stubs: { Icon: true, RouterLink: true } },
  })
}

function cardPlatforms(wrapper: VueWrapper): string[] {
  return wrapper.findAll('[data-testid="platform-card"]').map((card) => card.attributes('data-platform') ?? '')
}

describe('UserDashboardStats platform breakdown', () => {
  it('renders usage platforms but ignores quota rows with no configured limit', () => {
    const wrapper = mountStats(
      makeStats({ total_actual_cost: 0.03, today_actual_cost: 0.03, by_platform: [usage('grok', 0.03)] }),
      [quota({ platform: 'anthropic' }), quota({ platform: 'openai' }), quota({ platform: 'gemini' }), quota({ platform: 'grok' })],
    )

    expect(cardPlatforms(wrapper)).toEqual(['grok'])
    expect(wrapper.text()).toContain('dashboard.platformCount:{"count":1}')
    expect(wrapper.text()).not.toContain('dashboard.platformQuota.title')
  })

  it('shows quota-only platforms and preserves the fixed platform ordering', () => {
    const wrapper = mountStats(
      makeStats({ total_actual_cost: 0.03, today_actual_cost: 0.03, by_platform: [usage('grok', 0.03)] }),
      [quota({ platform: 'openai', daily_limit_usd: 10, daily_usage_usd: 2.5 })],
    )

    expect(cardPlatforms(wrapper)).toEqual(['openai', 'grok'])
    expect(wrapper.text()).toContain('dashboard.platformQuota.title')
    expect(wrapper.text()).toContain('dashboard.platformCount:{"count":2}')
  })

  it('merges usage and quota for one platform into a single card', () => {
    const wrapper = mountStats(
      makeStats({ total_actual_cost: 1, today_actual_cost: 1, by_platform: [usage('openai', 1)] }),
      [quota({ platform: 'openai', daily_limit_usd: 10, daily_usage_usd: 1 })],
    )

    expect(cardPlatforms(wrapper)).toEqual(['openai'])
    expect(wrapper.text()).toContain('dashboard.platformQuota.title')
  })

  it('treats a zero limit as configured and renders the disabled state', () => {
    const wrapper = mountStats(makeStats(), [quota({ platform: 'gemini', weekly_limit_usd: 0 })])

    expect(cardPlatforms(wrapper)).toEqual(['gemini'])
    expect(wrapper.text()).toContain('dashboard.platformQuota.disabled')
    expect(wrapper.text()).toContain('dashboard.platformCount:{"count":1}')
  })

  it('sorts unknown platforms after the fixed platform order', () => {
    const wrapper = mountStats(
      makeStats({ total_actual_cost: 0.5, by_platform: [usage('kimi', 0.3), usage('anthropic', 0.2)] }),
    )

    expect(cardPlatforms(wrapper)).toEqual(['anthropic', 'kimi'])
  })

  it('shows unmatched spend as other without counting it as a platform', () => {
    const wrapper = mountStats(
      makeStats({ total_actual_cost: 1, by_platform: [usage('anthropic', 0.4)] }),
    )

    expect(cardPlatforms(wrapper)).toEqual(['anthropic', '__other__'])
    expect(wrapper.text()).toContain('dashboard.platformOther')
    expect(wrapper.text()).toContain('dashboard.platformCount:{"count":1}')
  })

  it('hides the breakdown when there is neither usage nor configured quota', () => {
    const wrapper = mountStats(makeStats(), [quota({ platform: 'anthropic' }), quota({ platform: 'openai' })])

    expect(wrapper.html()).not.toContain('dashboard.platformBreakdown')
    expect(cardPlatforms(wrapper)).toEqual([])
  })

  it('hides the breakdown in simple mode', () => {
    const wrapper = mountStats(makeStats({ by_platform: [usage('openai', 1)] }), null, true)

    expect(wrapper.html()).not.toContain('dashboard.platformBreakdown')
  })
})
