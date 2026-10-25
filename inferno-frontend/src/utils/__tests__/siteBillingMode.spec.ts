import { describe, expect, it } from 'vitest'
import {
  SITE_BILLING_MODES,
  billingModeToSettings,
  resolveSiteBillingMode,
} from '@/utils/siteBillingMode'

describe('site billing mode', () => {
  it('defaults to recharge and subscription when settings are missing', () => {
    expect(resolveSiteBillingMode(undefined)).toBe('recharge_and_subscription')
    expect(resolveSiteBillingMode({})).toBe('recharge_and_subscription')
  })

  it('derives the mode from the subscription and balance switches', () => {
    expect(resolveSiteBillingMode({ subscription_enabled: false, payment_balance_disabled: false })).toBe('recharge_only')
    expect(resolveSiteBillingMode({ subscription_enabled: true, payment_balance_disabled: true })).toBe('subscription_only')
    expect(resolveSiteBillingMode({ subscription_enabled: true, payment_balance_disabled: false })).toBe('recharge_and_subscription')
  })

  it('round-trips each selectable mode without disabling both purchase options', () => {
    for (const mode of SITE_BILLING_MODES) {
      const settings = billingModeToSettings(mode)
      expect(resolveSiteBillingMode(settings)).toBe(mode)
      expect(settings.subscription_enabled || !settings.payment_balance_disabled).toBe(true)
    }
  })
})
