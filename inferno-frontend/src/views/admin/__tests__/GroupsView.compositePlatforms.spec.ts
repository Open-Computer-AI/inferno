import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

// Upstream asserts on the literal source text of compositeRoutePlatformOptions.
// Our tree derives that list from CONCRETE_PLATFORM_OPTIONS instead -- precisely
// so a newly added provider cannot silently go missing from a hand-maintained
// copy, which is how it drifted before. A source grep cannot check a derived
// list, so this asserts the two halves that actually carry the guarantee:
// the catalog holds the platforms, and the view still derives from it.
describe('GroupsView Composite route options', () => {
  it('offers Kimi, Zhipu GLM, and DeepSeek as route targets', () => {
    const values = CONCRETE_PLATFORM_OPTIONS.map((o) => o.value)
    expect(values).toContain('kimi')
    expect(values).toContain('zhipu')
    expect(values).toContain('deepseek')

    const labels = Object.fromEntries(CONCRETE_PLATFORM_OPTIONS.map((o) => [o.value, o.label]))
    expect(labels.kimi).toBe('Kimi')
    expect(labels.zhipu).toBe('Zhipu GLM')
    expect(labels.deepseek).toBe('DeepSeek')
  })

  it('derives the composite route options from the catalog rather than a literal', () => {
    const source = readFileSync(resolve('src/views/admin/GroupsView.vue'), 'utf8')
    const options = source.slice(
      source.indexOf('const compositeRoutePlatformOptions'),
      source.indexOf('const compositeRouteEndpointOptions')
    )
    expect(options).toContain('CONCRETE_PLATFORM_OPTIONS')
    // composite is not a routable target from inside a composite group
    expect(options).not.toContain('GROUP_PLATFORM_OPTIONS')
  })
})
