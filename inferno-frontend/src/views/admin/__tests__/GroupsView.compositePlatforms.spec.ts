import { describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'

describe('GroupsView Composite route options', () => {
  it('derives route targets from concrete platforms, excluding group-only platforms', () => {
    const concrete = CONCRETE_PLATFORM_OPTIONS.map((option) => option.value)
    const groupOnly = GROUP_PLATFORM_OPTIONS.map((option) => option.value).filter(
      (platform) => !concrete.includes(platform as (typeof concrete)[number])
    )
    expect(concrete).toContain('minimax')
    expect(groupOnly).toEqual(['composite'])
    expect(concrete).not.toContain('composite')
  })
})
