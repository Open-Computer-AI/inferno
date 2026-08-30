/**
 * Expiry urgency must still read as urgent.
 *
 * Upstream carried two jobs in one colour: expiry urgency (state) and platform
 * theming (category). June correctly dropped the platform half -- colour never
 * encodes category -- and the urgency half was dropped with it, because they
 * shared a function. This chip's own documented exception is "the only tinted
 * variant left is a per-user rate override, because that is the only one the
 * reader has to act on", and an expiring subscription passes that same test.
 *
 * State tokens, not upstream's bg-red-200/80 -- the June design stands.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createPinia } from 'pinia'
import GroupBadge from '../GroupBadge.vue'

const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false })
const mountBadge = (daysRemaining: number | null) =>
  mount(GroupBadge, {
    props: { name: 'Pro', subscriptionType: 'subscription', showRate: true, daysRemaining },
    global: { plugins: [i18n, createPinia()], stubs: { Icon: true } }
  })

const rateStyle = (w: ReturnType<typeof mountBadge>) =>
  w.find('.gbadge__rate').attributes('style') ?? ''

describe('GroupBadge expiry urgency', () => {
  it('marks an expiring-within-3-days subscription as destructive', () => {
    expect(rateStyle(mountBadge(2))).toContain('--destructive')
  })

  it('marks an expiring-within-7-days subscription as attention', () => {
    expect(rateStyle(mountBadge(6))).toContain('--s2a-attn')
  })

  it('leaves a healthy subscription untinted', () => {
    const style = rateStyle(mountBadge(30))
    expect(style).not.toContain('--destructive')
    expect(style).not.toContain('--s2a-attn')
  })

  it('leaves a subscription with no expiry untinted', () => {
    expect(rateStyle(mountBadge(null))).toBe('')
  })
})
