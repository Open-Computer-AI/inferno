import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'

// t() 回显 key，便于断言文案键
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import UserPlatformQuotaCell from '../UserPlatformQuotaCell.vue'
import type { PlatformQuotaItem } from '@/api/admin/users'

function item(over: Partial<PlatformQuotaItem> & { platform: PlatformQuotaItem['platform'] }): PlatformQuotaItem {
  return {
    daily_limit_usd: null, weekly_limit_usd: null, monthly_limit_usd: null,
    daily_usage_usd: 0, weekly_usage_usd: 0, monthly_usage_usd: 0,
    ...over,
  } as PlatformQuotaItem
}

describe('UserPlatformQuotaCell', () => {
  it('quotas 为 undefined 时渲染加载占位（flat skeleton，取代省略号字符）', () => {
    const w = mount(UserPlatformQuotaCell, { props: { quotas: undefined } })
    // June's loading rule: a flat static skeleton, not a ticking ellipsis
    // glyph -- the old "…" character cannot appear anymore. `.upq--loading`
    // / `.upq__skel` are this component's own scoped hooks for that state
    // (there is no data-testid on it).
    expect(w.find('.upq--loading').exists()).toBe(true)
    expect(w.find('.upq__skel').exists()).toBe(true)
    expect(w.html()).not.toContain('admin.users.platformQuota.cellNotConfigured')
  })

  it('空数组渲染「未配置」', () => {
    const w = mount(UserPlatformQuotaCell, { props: { quotas: [] } })
    expect(w.html()).toContain('admin.users.platformQuota.cellNotConfigured')
  })

  it('平台有记录但全部 limit 为 null 时视为未配置', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: { quotas: [item({ platform: 'openai', daily_usage_usd: 5 })] },
    })
    expect(w.html()).toContain('admin.users.platformQuota.cellNotConfigured')
  })

  // DROPPED GUARANTEE: the old cell rendered one "used/limit" row per
  // window (daily/weekly/monthly), with a null limit shown as "—" and
  // amounts kept undistorted (90.5, not 90.50). The migration note in
  // UserPlatformQuotaCell.vue says this explicitly: five stacked ratio rows
  // don't fit a 36px table row, so -- the same rule AccountUsageCell
  // applies to its windows -- only the single window closest to its limit
  // renders, as a percent, not a used/limit fraction. There is no "—"
  // placeholder and no used/limit text left to assert on for the windows
  // that aren't shown; the full breakdown is left to the row's detail
  // modal (outside this component). What's still real and testable: given
  // three windows, the one closest to its limit is correctly identified.
  it('已配置平台渲染最接近限额的窗口（daily/weekly/monthly 取比例最高者，比例正确换算）', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: {
        quotas: [
          item({ platform: 'anthropic', daily_limit_usd: 100, daily_usage_usd: 30,
                 weekly_limit_usd: null, weekly_usage_usd: 0,
                 monthly_limit_usd: 2000, monthly_usage_usd: 90.5 }),
        ],
      },
    })
    // daily = 30/100 = 30%, weekly has no limit (excluded), monthly =
    // 90.5/2000 = 4.525% -- daily is closest to its limit.
    expect(w.get('.upq__platform').text()).toBe('anthropic')
    expect(w.get('.upq__window').text()).toBe('admin.users.platformQuota.windowDaily')
    expect(w.get('.upq__percent').text()).toBe('30%')
    expect(w.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('30')
    // Only one platform is configured, so there's nothing for "+N" to hide.
    expect(w.find('.upq__more').exists()).toBe(false)
  })

  it('多平台并列时按 anthropic→openai→gemini→antigravity 顺序取胜，未配置限额的平台完全不计入', () => {
    const w = mount(UserPlatformQuotaCell, {
      props: {
        quotas: [
          item({ platform: 'gemini', monthly_limit_usd: 50 }),
          item({ platform: 'anthropic', daily_limit_usd: 10 }),
          item({ platform: 'openai', daily_usage_usd: 9 }), // no limit on any window -> not configured
        ],
      },
    })
    // anthropic and gemini both compute to 0% (no usage); PLATFORM_ORDER
    // breaks the tie in anthropic's favour for the one bar shown.
    expect(w.get('.upq__platform').text()).toBe('anthropic')
    // openai has no limit configured on any window, so it isn't a second
    // configured platform -- it doesn't count toward "+N" and isn't in the
    // overflow title either.
    expect(w.get('.upq__more').text()).toBe('+1')
    expect(w.get('.upq__more').attributes('title')).toBe('gemini')
    expect(w.text()).not.toContain('openai')
  })
})
