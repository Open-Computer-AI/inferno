import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ClaudeResetCreditsCell from '../ClaudeResetCreditsCell.vue'
import type { Account } from '@/types'
import type { ClaudeResetCredits, ClaudeResetOutcome } from '@/api/admin/claudeResetCredits'

const { getCredits, redeem } = vi.hoisted(() => ({
  getCredits: vi.fn(),
  redeem: vi.fn()
}))

vi.mock('@/api/admin/claudeResetCredits', () => ({
  getClaudeResetCredits: getCredits,
  redeemClaudeResetCredit: redeem
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const account = { id: 7, name: 'Claude account', platform: 'anthropic', type: 'oauth' } as Account

const status = (overrides: Partial<ClaudeResetCredits> = {}): ClaudeResetCredits => ({
  eligible: true,
  available_count: 1,
  credits: [{
    label: '5h reset',
    resets_left: 1,
    clears: ['five_hour'],
    percent_used: { five_hour: 100 },
    blocking: ['five_hour'],
    use_requires_limit: true,
    redeemable: true
  }],
  fetched_at: '2026-10-08T00:00:00Z',
  ...overrides
})

const resetOutcome = (overrides: Partial<ClaudeResetOutcome> = {}): ClaudeResetOutcome => ({
  outcome: 'reset',
  replayed: false,
  ...overrides
})

const confirmStub = {
  props: ['show'],
  template: '<button v-if="show" data-testid="confirm-reset" @click="$emit(\'confirm\'); $emit(\'confirm\')">confirm</button>'
}

const mountCell = () => mount(ClaudeResetCreditsCell, {
  props: { account },
  global: { stubs: { ConfirmDialog: confirmStub } }
})

const loadStatus = async (wrapper: ReturnType<typeof mountCell>, result = status()) => {
  getCredits.mockResolvedValueOnce(result)
  await wrapper.get('[data-testid="claude-reset-count"]').trigger('click')
  await flushPromises()
}

const confirmReset = async (wrapper: ReturnType<typeof mountCell>) => {
  await wrapper.get('[data-testid="claude-reset-redeem"]').trigger('click')
  await wrapper.get('[data-testid="confirm-reset"]').trigger('click')
  await flushPromises()
}

describe('ClaudeResetCreditsCell', () => {
  beforeEach(() => {
    getCredits.mockReset()
    redeem.mockReset()
  })

  it('does not query or redeem until an administrator asks', () => {
    const wrapper = mountCell()

    expect(getCredits).not.toHaveBeenCalled()
    expect(redeem).not.toHaveBeenCalled()

    wrapper.unmount()
  })

  it('blocks every later POST after an unknown result until a manual GET succeeds', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    redeem.mockResolvedValueOnce(resetOutcome({ outcome: 'unknown', reason: 'upstream_unavailable' }))

    await confirmReset(wrapper)

    expect(redeem).toHaveBeenCalledTimes(1)
    const firstKey = redeem.mock.calls[0][1]
    expect(wrapper.get('[data-testid="claude-reset-redeem"]').attributes('disabled')).toBeDefined()

    getCredits.mockResolvedValueOnce(status())
    await wrapper.get('[data-testid="claude-reset-count"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="claude-reset-redeem"]').attributes('disabled')).toBeUndefined()
    redeem.mockResolvedValueOnce(resetOutcome())
    await confirmReset(wrapper)

    expect(redeem).toHaveBeenCalledTimes(2)
    expect(redeem.mock.calls[1][1]).not.toBe(firstKey)
    wrapper.unmount()
  })

  it('keeps the same key and fence after a transport failure', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    redeem.mockRejectedValueOnce(new Error('network timeout'))

    await confirmReset(wrapper)

    const firstKey = redeem.mock.calls[0][1]
    expect(wrapper.get('[data-testid="claude-reset-redeem"]').attributes('disabled')).toBeDefined()

    getCredits.mockRejectedValueOnce(new Error('query timeout'))
    await wrapper.get('[data-testid="claude-reset-count"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="claude-reset-redeem"]').attributes('disabled')).toBeDefined()
    expect(redeem).toHaveBeenCalledTimes(1)
    expect(firstKey).toMatch(/^claude-reset-7-/)
    wrapper.unmount()
  })

  it('clears stale success feedback when a manual query reports a non-successful status', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    redeem.mockResolvedValueOnce(resetOutcome())

    await confirmReset(wrapper)

    expect(wrapper.get('[data-testid="claude-reset-feedback"]').text()).toContain('outcome.reset')
    expect(getCredits).toHaveBeenCalledTimes(1)

    getCredits.mockResolvedValueOnce(status({ eligible: false, available_count: 0, credits: [] }))
    await wrapper.get('[data-testid="claude-reset-count"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="claude-reset-feedback"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('claudeResetCredits.ineligible')
    expect(getCredits).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('does not issue a duplicate POST when confirmation fires twice', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    let resolveRedeem!: (result: ClaudeResetOutcome) => void
    redeem.mockImplementationOnce(() => new Promise<ClaudeResetOutcome>(resolve => {
      resolveRedeem = resolve
    }))

    await wrapper.get('[data-testid="claude-reset-redeem"]').trigger('click')
    await wrapper.get('[data-testid="confirm-reset"]').trigger('click')

    expect(redeem).toHaveBeenCalledTimes(1)
    resolveRedeem(resetOutcome())
    await flushPromises()
    wrapper.unmount()
  })

  it('ignores a redemption response after the account changes', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    let resolveRedeem!: (result: ClaudeResetOutcome) => void
    redeem.mockImplementationOnce(() => new Promise<ClaudeResetOutcome>(resolve => {
      resolveRedeem = resolve
    }))

    await wrapper.get('[data-testid="claude-reset-redeem"]').trigger('click')
    await wrapper.get('[data-testid="confirm-reset"]').trigger('click')
    await wrapper.setProps({ account: { ...account, id: 8 } })
    resolveRedeem(resetOutcome())
    await flushPromises()

    expect(wrapper.find('[data-testid="claude-reset-feedback"]').exists()).toBe(false)
    expect(wrapper.emitted('redeemed')).toBeUndefined()
    wrapper.unmount()
  })

  it('releases the key only for a definite pre-claim refusal', async () => {
    const wrapper = mountCell()
    await loadStatus(wrapper)
    redeem.mockRejectedValueOnce({ reason: 'CLAUDE_RESET_NOT_AVAILABLE' })

    await confirmReset(wrapper)
    const firstKey = redeem.mock.calls[0][1]
    expect(wrapper.get('[data-testid="claude-reset-redeem"]').attributes('disabled')).toBeUndefined()

    redeem.mockResolvedValueOnce(resetOutcome())
    await confirmReset(wrapper)

    expect(redeem.mock.calls[1][1]).not.toBe(firstKey)
    wrapper.unmount()
  })
})
