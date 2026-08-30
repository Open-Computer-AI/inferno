import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'

const { getUsage } = vi.hoisted(() => ({ getUsage: vi.fn() }))
vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getUsage, refreshOllamaCloudUsage: vi.fn() } },
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})

/**
 * Fixtures copied verbatim from the two Grok accounts on the internal VM
 * (oc-internal, 2026-08-30). Both are real production shapes: xAI reports a
 * spend but no monthly limit, which is exactly the state fd42d3722 fixes --
 * before it, the cell rendered "0.67/0" and "0.25/0".
 */
const REAL = [
  { name: 'grok-oauth-internal-01', prepaid_balance: 0, monthly_limit: 0, monthly_used: 0.67 },
  { name: 'grok-oauth-internal-02', prepaid_balance: 0, monthly_limit: 0, monthly_used: 0.25 },
]

function account() {
  return {
    id: 900, name: 'g', platform: 'grok', type: 'oauth', status: 'active',
    schedulable: true, extra: {}, created_at: '2026-08-25T00:00:00Z', updated_at: '2026-08-25T00:00:00Z',
  } as any
}

describe('Grok money line against real oc-internal data', () => {
  it.each(REAL)('$name: no "used/0" line when xAI reports no monthly limit', async (row) => {
    getUsage.mockResolvedValue({
      subscription_tier: 'SuperGrok',
      grok_billing: {
        period_type: 'weekly', status_code: 200,
        prepaid_balance: row.prepaid_balance,
        monthly_limit: row.monthly_limit,
        monthly_used: row.monthly_used,
        used_cents: Math.round(row.monthly_used * 100),
        monthly_limit_cents: 0,
      },
    })
    const wrapper = mount(AccountUsageCell, {
      props: { account: account() },
      global: { stubs: { UsageProgressBar: true, AccountQuotaInfo: true, Icon: true } },
    })
    await flushPromises()
    const text = wrapper.text()

    // the pre-fix rendering
    expect(text).not.toContain(`${row.monthly_used}/0`)
    expect(text).not.toContain(`${row.monthly_used.toFixed(2)}/0`)
    // and neither half of the money line should be offered at all
    expect(text).not.toContain('admin.accounts.usageWindow.grokPrepaid')
    expect(text).not.toContain('admin.accounts.usageWindow.grokUsed')
    wrapper.unmount()
  })
})
