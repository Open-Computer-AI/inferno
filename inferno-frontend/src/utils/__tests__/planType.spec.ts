import { describe, expect, it } from 'vitest'
import {
  normalizePlanType,
  openAIPlanTypeKey,
  openAIPlanTypeLabel,
  openAIPlanTypes
} from '../planType'

describe('OpenAI plan type catalog', () => {
  it('normalizes separators and aliases for comparison', () => {
    expect(normalizePlanType('  Pro Lite ')).toBe('prolite')
    expect(openAIPlanTypeKey('chatgptpro')).toBe('pro')
    expect(openAIPlanTypeKey('PRO')).toBe('pro')
  })

  it('keeps the supported wire SKU catalog ordered and unique', () => {
    expect(openAIPlanTypes).toEqual([
      'free',
      'go',
      'plus',
      'prolite',
      'pro',
      'promax',
      'team',
      'self_serve_business_usage_based',
      'self_serve_business_prolite',
      'business',
      'enterprise',
      'ent26',
      'enterprise_cbp_usage_based',
      'enterprise_cbp_automation',
      'edu',
      'edu_plus',
      'edu_pro',
      'unknown'
    ])
    expect(new Set(openAIPlanTypes).size).toBe(openAIPlanTypes.length)
  })

  it('provides distinct status and analytics labels where needed', () => {
    expect(openAIPlanTypeLabel('pro')).toBe('Pro 200')
    expect(openAIPlanTypeLabel('self_serve_business_prolite')).toBe('Business Premium')
    expect(openAIPlanTypeLabel('business')).toBe('Enterprise')
    expect(openAIPlanTypeLabel('business', 'analytics')).toBe('Business')
    expect(openAIPlanTypeLabel('edu_plus', 'analytics')).toBe('Education')
  })
})
