import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { AccountPlatform } from '@/types'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function mountPlan(platform: AccountPlatform, planType: string) {
  return mount(PlatformTypeBadge, { props: { platform, type: 'oauth', planType } })
}

describe('PlatformTypeBadge OpenAI plan tiers', () => {
  it('normalizes Pro aliases to Pro 200', () => {
    for (const planType of ['pro', 'chatgptpro', 'PRO', 'ChatGPT Pro']) {
      expect(mountPlan('openai', planType).text()).toContain('Pro 200')
    }
  })

  it('normalizes ProLite aliases to Pro 100', () => {
    for (const planType of ['prolite', 'PROLITE', 'pro_lite']) {
      const text = mountPlan('openai', planType).text()
      expect(text).toContain('Pro 100')
      expect(text).not.toContain('Pro 200')
    }
  })

  it('normalizes Team to Business', () => {
    expect(mountPlan('openai', 'team').text()).toContain('Business')
  })

  it('normalizes self-serve business ProLite to Business Premium', () => {
    for (const planType of ['self_serve_business_prolite', 'selfservebusinessprolite']) {
      const text = mountPlan('openai', planType).text()
      expect(text).toContain('Business Premium')
      expect(text).not.toContain(planType)
    }
  })

  it('keeps Plus, Free and abnormal labels intact', () => {
    expect(mountPlan('openai', 'plus').text()).toContain('Plus')
    expect(mountPlan('openai', 'free').text()).toContain('Free')
    expect(mountPlan('openai', 'abnormal').text()).toContain('admin.accounts.subscriptionAbnormal')
  })

  it('renders the canonical label for enterprise plans', () => {
    expect(mountPlan('openai', 'enterprise').text()).toContain('Enterprise')
  })

  it('does not apply ChatGPT plan naming to other platforms', () => {
    for (const platform of ['antigravity', 'grok'] as AccountPlatform[]) {
      const text = mountPlan(platform, 'pro').text()
      expect(text).toContain('Pro')
      expect(text).not.toContain('Pro 200')
    }
    expect(mountPlan('antigravity', 'team').text()).toContain('Team')
  })
})
