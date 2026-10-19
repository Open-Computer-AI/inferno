import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const mockConcurrency = vi.fn()
const mockAvailability = vi.fn()
const mockUserConcurrency = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getConcurrencyStats: (...args: any[]) => mockConcurrency(...args),
    getAccountAvailabilityStats: (...args: any[]) => mockAvailability(...args),
    getUserConcurrencyStats: (...args: any[]) => mockUserConcurrency(...args)
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

describe('OpsConcurrencyCard strict concurrency diagnostics', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders unknown and hides the load bar when strict concurrency is unavailable', async () => {
    mockConcurrency.mockResolvedValue({
      enabled: true,
      account: {
        7: {
          account_name: 'acct-7',
          platform: 'anthropic',
          group_id: 1,
          group_name: 'default',
          current_in_use: 4,
          current_concurrency: null,
          concurrency_read_error: 'unavailable',
          max_capacity: 4,
          waiting_in_queue: 0,
          load_percentage: 100
        }
      }
    })
    mockAvailability.mockResolvedValue({
      enabled: true,
      account: { 7: { account_name: 'acct-7', platform: 'anthropic', group_id: 1 } }
    })

    const wrapper = mount(OpsConcurrencyCard, {
      props: { platformFilter: 'anthropic', groupIdFilter: 1, refreshToken: 0 }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('common.unknown/4')
    expect(wrapper.find('.h-full.rounded-full').exists()).toBe(false)
  })
})
