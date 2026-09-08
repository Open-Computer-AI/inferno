/**
 * Upstream's spec, restored.
 *
 * We carry this catalog and derive every platform filter from it, where
 * upstream edits six hand-maintained arrays. That is a deliberate divergence,
 * and it means this file is MORE load-bearing for us than for them: if a
 * provider drops out of the catalog it drops out of every filter at once.
 *
 * The port took the catalog and left the spec behind. Found by
 * scripts/behaviour-parity.mjs.
 */
import { describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS } from '@/constants/platforms'

const concretePlatforms = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'kimi',
  'zhipu',
  'deepseek'
]

describe('platform option catalogs', () => {
  it('exposes every concrete account platform', () => {
    expect(CONCRETE_PLATFORM_OPTIONS.map((option) => option.value)).toEqual(concretePlatforms)
  })

  it('adds composite for group-backed filters', () => {
    expect(GROUP_PLATFORM_OPTIONS.map((option) => option.value)).toEqual([
      ...concretePlatforms,
      'composite'
    ])
  })

  it('gives every option a non-empty label', () => {
    // Ours, not upstream's: a value with no label renders an empty filter row,
    // which is the failure a derived catalog makes global rather than local.
    for (const option of GROUP_PLATFORM_OPTIONS) {
      expect(option.label.trim().length).toBeGreaterThan(0)
    }
  })
})
