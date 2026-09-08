import { describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'
import { buildCompositeRoutePlatformOptions } from '../groupsCompositeRoutes'
describe('GroupsView Composite route options', () => {
  it('derives route targets from concrete options, excluding group-only options', () => {
    const compositeRoutePlatformOptions = buildCompositeRoutePlatformOptions()
    expect(compositeRoutePlatformOptions).toEqual(CONCRETE_PLATFORM_OPTIONS)
    expect(compositeRoutePlatformOptions).not.toBe(GROUP_PLATFORM_OPTIONS)
    expect(compositeRoutePlatformOptions.map((option) => option.value)).toContain('minimax')
    expect(compositeRoutePlatformOptions.map((option) => option.value)).not.toContain('composite')
  })
})
