import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getGrokMediaEligibility,
  refreshCredentials,
  refreshOpenAIReferrals,
  sendOpenAIReferralInvite,
  updateGrokMediaEligibility
} from '@/api/admin/accounts'

describe('admin account referral and eligibility APIs', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('uses the referral refresh and invite contracts', async () => {
    const eligibility = { should_show: true, available_invites: 2, program_id: 'codex_referral_consumer' }
    get.mockResolvedValue({ data: eligibility })
    post
      .mockResolvedValueOnce({ data: { eligibility, cache_persisted: true } })
      .mockResolvedValueOnce({
        data: { sent: true, email: 'friend@example.com', eligibility, cache_persisted: true, refresh_failed: false }
      })

    await expect(refreshOpenAIReferrals(7)).resolves.toEqual({ eligibility, cache_persisted: true })
    await expect(sendOpenAIReferralInvite(7, {
      email: 'friend@example.com',
      program_id: 'codex_referral_consumer',
      confirmed: true
    })).resolves.toMatchObject({ sent: true, email: 'friend@example.com' })

    expect(post).toHaveBeenNthCalledWith(1, '/admin/openai/accounts/7/referrals/refresh')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/openai/accounts/7/referrals/invite', {
      email: 'friend@example.com',
      program_id: 'codex_referral_consumer',
      confirmed: true
    }, { timeout: 90_000 })
  })

  it('uses the Grok media eligibility GET/PUT contract', async () => {
    const state = { account_id: 7, mode: 'auto', eligible: true, reason: 'billing_ok' }
    get.mockResolvedValueOnce({ data: state })
    put.mockResolvedValueOnce({ data: { ...state, mode: 'enabled' } })

    await expect(getGrokMediaEligibility(7)).resolves.toEqual(state)
    await expect(updateGrokMediaEligibility(7, 'enabled')).resolves.toMatchObject({ mode: 'enabled' })

    expect(get).toHaveBeenCalledWith('/admin/accounts/7/grok-media-eligibility')
    expect(put).toHaveBeenCalledWith('/admin/accounts/7/grok-media-eligibility', { mode: 'enabled' })
  })

  it('normalizes both legacy raw-account and warning-wrapped refresh responses', async () => {
    const account = { id: 7, name: 'account', platform: 'openai', type: 'oauth' }
    post
      .mockResolvedValueOnce({ data: account })
      .mockResolvedValueOnce({ data: {
        account,
        message: 'Credentials refreshed; temporary project still missing.',
        warning: 'missing_project_id_temporary'
      } })

    await expect(refreshCredentials(7)).resolves.toEqual({ account })
    await expect(refreshCredentials(7)).resolves.toEqual({
      account,
      message: 'Credentials refreshed; temporary project still missing.',
      warning: 'missing_project_id_temporary'
    })
  })
})
