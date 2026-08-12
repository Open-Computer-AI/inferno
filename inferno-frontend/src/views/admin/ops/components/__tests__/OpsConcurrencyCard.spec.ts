import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsConcurrencyCard from '../OpsConcurrencyCard.vue'

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../OpsConcurrencyCard.vue'),
  'utf8'
)

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

/**
 * The account view is only reachable with a group filter, and its rows come
 * from an in-memory concurrency registry rather than any seeded table -- so on
 * a dev box with no live traffic it renders empty and cannot be checked by
 * looking at it. These are the assertions that would otherwise have no home.
 */
const concurrencyFor = (id: string) => ({
  enabled: true,
  account: {
    [id]: {
      account_name: `acct-${id}`,
      platform: 'anthropic',
      group_id: 1,
      group_name: 'default',
      current_in_use: 1,
      max_capacity: 4,
      waiting_in_queue: 0,
      load_percentage: 25
    }
  }
})

async function mountWithAccount(avail: Record<string, unknown>) {
  mockConcurrency.mockResolvedValue(concurrencyFor('7'))
  mockAvailability.mockResolvedValue({
    enabled: true,
    account: { 7: { account_name: 'acct-7', platform: 'anthropic', group_id: 1, ...avail } }
  })
  const wrapper = mount(OpsConcurrencyCard, {
    props: { platformFilter: 'anthropic', groupIdFilter: 1, refreshToken: 0 }
  })
  await flushPromises()
  return wrapper
}

describe('OpsConcurrencyCard account state', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  /*
   * Five sibling spans became one derived chip. The branch ORDER is the part
   * worth pinning: an account can be both rate limited and errored, and the
   * old markup resolved that with v-if/v-else-if ordering that is easy to
   * reshuffle by accident.
   */
  it.each([
    ['available', { is_available: true }, 'ok', 'admin.ops.accountAvailability.available'],
    ['rate limited', { is_rate_limited: true, rate_limit_remaining_sec: 90 }, 'warning', '1m'],
    ['overloaded', { is_overloaded: true, overload_remaining_sec: 30 }, 'critical', '30s'],
    ['errored', { has_error: true }, 'critical', 'admin.ops.accountAvailability.accountError'],
    ['unavailable', {}, 'muted', 'admin.ops.accountAvailability.unavailable']
  ])('renders %s with its own tone', async (_name, avail, tone, label) => {
    const wrapper = await mountWithAccount(avail)
    const chip = wrapper.get('.conc__state')
    expect(chip.attributes('data-tone')).toBe(tone)
    expect(chip.text()).toBe(label)
  })

  it('keeps available ahead of rate limited when both are set', async () => {
    const wrapper = await mountWithAccount({
      is_available: true,
      is_rate_limited: true,
      rate_limit_remaining_sec: 90
    })
    expect(wrapper.get('.conc__state').attributes('data-tone')).toBe('ok')
  })

  it('keeps rate limited ahead of overloaded and errored', async () => {
    const wrapper = await mountWithAccount({
      is_rate_limited: true,
      rate_limit_remaining_sec: 90,
      is_overloaded: true,
      has_error: true
    })
    expect(wrapper.get('.conc__state').text()).toBe('1m')
  })

  /* Available is the state nearly every account is in. A chip on every healthy
     row is colour that marks nothing, so it carries no fill of its own. */
  it('gives the healthy state no colour of its own', async () => {
    const wrapper = await mountWithAccount({ is_available: true })
    expect(wrapper.html()).toContain('data-tone="ok"')

    // Only warning and critical get a fill; `ok` and `muted` fall through to
    // the neutral base.
    const toned = source.match(/\.conc__state\[data-tone='([a-z]+)'\]/g) ?? []
    expect(new Set(toned)).toEqual(
      new Set(["conc__state[data-tone='warning']", "conc__state[data-tone='critical']"].map((s) => `.${s}`))
    )
  })

  /* Nothing in this component may reach for the dead Tailwind palettes. */
  it('carries no legacy colour utilities', () => {
    expect(
      source.match(/\b(?:bg|text|border|ring|divide)-(?:gray|red|green|blue|amber|slate|dark|rose)-\d+/g)
    ).toBeNull()
  })
})
