import {
  canDismissAffiliateWithdraw,
  completeAffiliateWithdrawOperation,
  prepareAffiliateWithdrawOperation,
} from '../affiliateWithdrawOperation'

describe('affiliate offline withdrawal idempotency', () => {
  beforeEach(() => {
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 17 }))
  })

  it('does not allow dismissing an in-flight or outcome-uncertain withdrawal', () => {
    expect(canDismissAffiliateWithdraw(true, false)).toBe(false)
    expect(canDismissAffiliateWithdraw(false, true)).toBe(false)
    expect(canDismissAffiliateWithdraw(false, false)).toBe(true)
  })

  it('reuses the same key for an uncertain retry and releases it after a definitive result', () => {
    const first = prepareAffiliateWithdrawOperation(42, 1.234567891)
    expect(first.amount).toBe(1.23456789)
    expect(first.outcomeUncertain).toBe(false)

    const retry = prepareAffiliateWithdrawOperation(42, 1.23456789)
    expect(retry.key).toBe(first.key)
    expect(retry.outcomeUncertain).toBe(true)

    completeAffiliateWithdrawOperation(retry)
    const next = prepareAffiliateWithdrawOperation(42, 1.23456789)
    expect(next.key).not.toBe(first.key)
    expect(next.outcomeUncertain).toBe(false)
  })

  it('scopes a pending key to the admin, user, and normalized amount', () => {
    const first = prepareAffiliateWithdrawOperation(42, 1)
    expect(prepareAffiliateWithdrawOperation(42, 2).key).not.toBe(first.key)
    expect(prepareAffiliateWithdrawOperation(43, 1).key).not.toBe(first.key)

    localStorage.setItem('auth_user', JSON.stringify({ id: 18 }))
    expect(prepareAffiliateWithdrawOperation(42, 1).key).not.toBe(first.key)
  })
})
