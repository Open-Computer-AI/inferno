/**
 * Upstream's spec, restored and widened.
 *
 * Every admin platform filter must derive from the shared catalog rather than
 * hand-list its values. Upstream maintains six separate arrays; we derive, so a
 * provider added to the catalog appears everywhere at once -- and a view that
 * quietly reverts to a literal list would silently stop gaining new providers,
 * which is exactly the bug 1f2a87adb fixed upstream.
 *
 * A source-text assertion is the right shape here: the property is "this view
 * imports from the catalog", which no rendered output can prove. Found missing
 * by scripts/behaviour-parity.mjs.
 */
import { readFileSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const read = (p: string) => readFileSync(resolve(SRC, p), 'utf8')

/** Views that select a CONCRETE platform (no composite). */
const CONCRETE_CONSUMERS = [
  'components/admin/account/AccountTableFilters.vue',
  'components/admin/ErrorPassthroughRulesModal.vue',
  'views/admin/ops/components/OpsDashboardHeader.vue'
]

describe('admin platform filters derive from the shared catalog', () => {
  it('uses the group platform catalog on the subscriptions page', () => {
    const source = read('views/admin/SubscriptionsView.vue')
    expect(source).toContain("from '@/constants/platforms'")
    expect(source).toContain('GROUP_PLATFORM_OPTIONS')
  })

  it('uses both catalogs on the groups page', () => {
    // Groups own a platform (composite included) but route to concrete targets,
    // so this view is the one place that legitimately needs both.
    const source = read('views/admin/GroupsView.vue')
    expect(source).toContain('GROUP_PLATFORM_OPTIONS')
    expect(source).toContain('CONCRETE_PLATFORM_OPTIONS')
  })

  it.each(CONCRETE_CONSUMERS)('uses the concrete catalog in %s', (path) => {
    const source = read(path)
    expect(source).toContain("from '@/constants/platforms'")
    expect(source).toContain('CONCRETE_PLATFORM_OPTIONS')
  })

  it('never hand-lists the platforms it could derive', () => {
    // Ours, not upstream's. The regression this guards is a view drifting back
    // to a literal array -- which type-checks, renders, and silently stops
    // gaining new providers.
    for (const path of [...CONCRETE_CONSUMERS, 'views/admin/SubscriptionsView.vue']) {
      const source = read(path)
      expect(source).not.toMatch(/value:\s*['"]kimi['"]/)
      expect(source).not.toMatch(/value:\s*['"]deepseek['"]/)
    }
  })
})
