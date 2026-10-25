import { affiliatesAPI } from '../affiliates'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { post },
}))

describe('affiliate offline withdrawal API', () => {
  beforeEach(() => post.mockReset())

  it('sends the required idempotency key and reports a replayed result', async () => {
    const result = {
      ledger_id: 10,
      user_id: 42,
      amount: 3.25,
      available_quota_after: 1.75,
      frozen_quota_after: 0,
      history_quota_after: 5,
    }
    post.mockResolvedValue({ data: result, headers: { 'x-idempotency-replayed': 'true' } })

    await expect(affiliatesAPI.withdrawUserQuota(42, { amount: 3.25 }, 'retry-key')).resolves.toEqual({
      result,
      replayed: true,
    })
    expect(post).toHaveBeenCalledWith(
      '/admin/affiliates/users/42/withdraw',
      { amount: 3.25 },
      { headers: { 'Idempotency-Key': 'retry-key' } },
    )
  })

  it('treats a first successful registration as not replayed', async () => {
    post.mockResolvedValue({ data: { ledger_id: 11 }, headers: {} })
    await expect(affiliatesAPI.withdrawUserQuota(42, { amount: 1 }, 'first-key')).resolves.toEqual({
      result: { ledger_id: 11 },
      replayed: false,
    })
  })
})
