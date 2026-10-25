import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import { getClaudeResetCredits, redeemClaudeResetCredit } from '@/api/admin/claudeResetCredits'

describe('Claude reset credits API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('reads reset credits without redeeming automatically', async () => {
    const status = { eligible: true, available_count: 1, credits: [], fetched_at: '2026-10-08T00:00:00Z' }
    get.mockResolvedValue({ data: status })

    await expect(getClaudeResetCredits(7)).resolves.toEqual(status)

    expect(get).toHaveBeenCalledWith('/admin/accounts/7/claude/reset-credits')
    expect(post).not.toHaveBeenCalled()
  })

  it('passes the same operation key through repeated idempotent requests', async () => {
    const result = { outcome: 'reset', replayed: false }
    post.mockResolvedValue({ data: result })

    await redeemClaudeResetCredit(7, 'claude-reset-7-stable-key')
    await redeemClaudeResetCredit(7, 'claude-reset-7-stable-key')

    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/claude/reset-credits/redeem', undefined, {
      headers: { 'Idempotency-Key': 'claude-reset-7-stable-key' }
    })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/7/claude/reset-credits/redeem', undefined, {
      headers: { 'Idempotency-Key': 'claude-reset-7-stable-key' }
    })
  })
})
