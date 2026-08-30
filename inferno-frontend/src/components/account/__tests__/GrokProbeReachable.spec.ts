/**
 * The xAI quota probe must be reachable from somewhere.
 *
 * Part 08 unhooked GrokQuotaProbeCell from AccountUsageCell on the reasoning
 * that a live xAI call should not look like a routine row action, and recorded
 * an opt-in column as owed to replace it. The column was never built, so the
 * component had no importer at all: stale Grok quota could not be refreshed on
 * demand anywhere in the app. Removing is instant and replacing needs a
 * decision, so the trade defaulted to permanent loss.
 *
 * This asserts the affordance exists and is gated the way the probe itself
 * gates -- grok + oauth, nothing else.
 */
import { describe, it, expect, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import GrokQuotaProbeCell from '../GrokQuotaProbeCell.vue'

vi.mock('@/api/admin', () => ({ adminAPI: { probeGrokQuota: vi.fn() } }))

const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

const account = (over: Record<string, unknown> = {}) =>
  ({ id: 1, platform: 'grok', type: 'oauth', name: 'x', ...over }) as never

const probeVisible = (over: Record<string, unknown> = {}) =>
  mount(GrokQuotaProbeCell, { props: { account: account(over), compact: true }, global: { plugins: [i18n] } })
    .find('button').exists()

describe('Grok quota probe reachability', () => {
  it('is imported by the cell that shows Grok quota', () => {
    // A component with no importer is a feature that does not exist.
    const src = readFileSync(resolve(__dirname, '../AccountUsageCell.vue'), 'utf8')
    expect(src).toContain("import GrokQuotaProbeCell from './GrokQuotaProbeCell.vue'")
    // Upstream renders it in BOTH branches -- with quota data and without.
    // The no-data branch is where a probe is most useful.
    expect(src.match(/<GrokQuotaProbeCell/g)?.length).toBe(2)
  })

  it('refreshes the cell after a probe, bypassing cache', () => {
    // Otherwise the operator pays for a live call and still reads stale numbers.
    const src = readFileSync(resolve(__dirname, '../AccountUsageCell.vue'), 'utf8')
    expect(src).toContain("loadUsage({ source: 'active', bypassCache: true })")
  })

  it('offers the probe for a Grok OAuth account', () => {
    expect(probeVisible()).toBe(true)
  })

  it('offers it for nothing else', () => {
    expect(probeVisible({ platform: 'openai' })).toBe(false)
    expect(probeVisible({ type: 'setup-token' })).toBe(false)
    expect(probeVisible({ platform: 'anthropic', type: 'oauth' })).toBe(false)
  })
})
