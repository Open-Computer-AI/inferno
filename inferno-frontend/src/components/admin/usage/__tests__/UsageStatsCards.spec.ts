import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import UsageStatsCards from '../UsageStatsCards.vue'

const messages: Record<string, string> = {
  'usage.totalRequests': 'Total Requests',
  'usage.inSelectedRange': 'in selected range',
  'usage.totalTokens': 'Total Tokens',
  'usage.in': 'In',
  'usage.out': 'Out',
  'usage.cacheTotal': 'Cache',
  'usage.cacheBreakdown': 'Cache Token Breakdown',
  'usage.cacheCreationTokensLabel': 'Cache Creation',
  'usage.cacheReadTokensLabel': 'Cache Read',
  'usage.totalCost': 'Total Cost',
  'usage.accountCost': 'Cost',
  'usage.standardCost': 'Standard',
  'usage.avgDuration': 'Avg Duration',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const stats = {
  total_requests: 1,
  total_input_tokens: 100,
  total_output_tokens: 50,
  total_cache_tokens: 34,
  total_cache_creation_tokens: 12,
  total_cache_read_tokens: 22,
  total_tokens: 184,
  total_cost: 0.001,
  total_actual_cost: 0.001,
  total_account_cost: 0.001,
  average_duration_ms: 250,
}

describe('UsageStatsCards', () => {
  it('shows cache token breakdown values', () => {
    const wrapper = mount(UsageStatsCards, {
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const text = wrapper.text()
    expect(text).toContain('Cache: 34')
    expect(text).toContain('Cache Token Breakdown')
    expect(text).toContain('Cache Creation')
    expect(text).toContain('12')
    expect(text).toContain('Cache Read')
    expect(text).toContain('22')
  })

  // Upstream 0aef702b6 fixed a phantom horizontal scroll on /usage and
  // /admin/usage: its cache tooltip was hidden with `opacity-0`, which is
  // invisible but still occupies layout, so a fixed w-56 box centred on a
  // trigger near the right edge widened the document. Upstream's fix was to
  // swap `opacity-0` for `hidden`, and its test asserts those two classes.
  //
  // That assertion cannot be ported: the June rebuild replaced the inline span
  // with <HelpTooltip>, which hides via v-show and teleports to <body>, so
  // there is no such span and no such class. The GUARANTEE ports, and this
  // pins it in the shape our architecture actually has -- hidden must mean
  // display:none, never merely transparent.
  it('keeps the cache tooltip out of the layout while it is hidden', () => {
    mount(UsageStatsCards, {
      attachTo: document.body,
      props: {
        stats,
      },
      global: {
        stubs: {
          Icon: true,
        },
      },
    })

    const tooltip = document.body.querySelector('[role="tooltip"]') as HTMLElement | null

    expect(tooltip).not.toBeNull()
    expect(tooltip!.style.display).toBe('none')
    expect(tooltip!.className).not.toContain('opacity-0')
  })
})
