import { describe, expect, it } from 'vitest'
import {
  SETTINGS_GROUPS,
  SETTINGS_SECTIONS,
  isSettingsSectionKey,
} from '@/components/admin/settings/settingsRegistry'

describe('settings registry', () => {
  it('covers every routed page exactly once and preserves the June grouping', () => {
    expect(SETTINGS_SECTIONS.map((section) => section.key)).toEqual([
      'general',
      'appearance',
      'security',
      'users',
      'agreement',
      'features',
      'gateway',
      'payment',
      'email',
      'backup',
    ])
    expect(SETTINGS_GROUPS.flatMap((group) => group.sections)).toHaveLength(10)
    expect(new Set(SETTINGS_GROUPS.flatMap((group) => group.sections.map((section) => section.key))).size).toBe(10)
  })

  it('rejects empty and unknown route sections', () => {
    expect(isSettingsSectionKey('payment')).toBe(true)
    expect(isSettingsSectionKey(undefined)).toBe(false)
    expect(isSettingsSectionKey('legacy-tab')).toBe(false)
  })
})
