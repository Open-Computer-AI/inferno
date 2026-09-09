import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
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

  it('connects the concrete route helper to the production GroupsView computed option', () => {
    const source = readFileSync(resolve('src/views/admin/GroupsView.vue'), 'utf8')

    expect(source).toContain('import { buildCompositeRoutePlatformOptions } from "./groupsCompositeRoutes";')
    expect(source).toMatch(
      /const\s+compositeRoutePlatformOptions\s*=\s*computed\(\(\)\s*=>\s*buildCompositeRoutePlatformOptions\(\)\s*\);/
    )
    expect(source).toMatch(
      /:options="compositeRoutePlatformOptions"/
    )
  })
})
