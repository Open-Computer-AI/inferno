import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Account } from '@/types'

vi.mock('@/api/admin/accounts', () => ({
  refreshOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.time ? `${key}:${params.time}` : key,
    }),
  }
})

function makeAccount(extra: Record<string, unknown>): Account {
  return {
    id: 1,
    name: 'acc',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    parent_account_id: null,
    extra,
  } as unknown as Account
}

// Port of upstream 6f972145b. Upstream paints this chip with Tailwind colour
// utilities; this file is June, so the same states must arrive as BEM
// modifiers over design tokens.
describe('OpenAIQuotaResetCell auto-reset state', () => {
  it('renders nothing when the scheduler has written no state', () => {
    const wrapper = mount(OpenAIQuotaResetCell, { props: { account: makeAccount({}) } })
    expect(wrapper.find('[data-testid="auto-reset-credit-state"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it.each([
    ['available', 'oqr__auto-chip--info'],
    ['success', 'oqr__auto-chip--ok'],
    ['no_credit', 'oqr__auto-chip--bad'],
    ['failed', 'oqr__auto-chip--bad'],
    ['resetting', 'oqr__auto-chip--busy'],
    ['checking', ''],
  ])('maps status %s to the June modifier %s', (status, modifier) => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { account: makeAccount({ codex_auto_reset_credit_state: { status } }) },
    })
    const chip = wrapper.get('[data-testid="auto-reset-credit-state"] .oqr__auto-chip')
    if (modifier) expect(chip.classes()).toContain(modifier)
    else expect(chip.classes()).toEqual(['oqr__auto-chip'])
    // The label must go through i18n, not a hardcoded English string.
    expect(chip.text()).toContain('admin.accounts.openaiQuotaReset.autoStatus.')
    wrapper.unmount()
  })

  it('shows the trigger window and the error code when present', () => {
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: {
        account: makeAccount({
          codex_auto_reset_credit_state: {
            status: 'failed',
            trigger_window: '7d',
            error_code: 'quota_exhausted',
          },
        }),
      },
    })
    const block = wrapper.get('[data-testid="auto-reset-credit-state"]')
    expect(block.get('.oqr__auto-window').text()).toBe('7d')
    expect(block.get('.oqr__auto-error').text()).toBe('quota_exhausted')
    wrapper.unmount()
  })
})
