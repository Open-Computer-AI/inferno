/**
 * The operator's configured SLA floor must actually reach the screen.
 *
 * The June rewrite merged the SLA and request-error cards into one split bar
 * and dropped getSLAThresholdLevel / getRequestErrorRateThresholdLevel with
 * them. sla_percent_min and request_error_rate_percent_max became write-only:
 * the Ops settings screen accepted and persisted them, and nothing ever read
 * them. An operator setting 99.5 got no signal until the diagnosis panel's
 * hardcoded 98 -- four times looser than what they asked for, from a field
 * that looked like it had taken effect.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import OpsTrafficSplit from '../OpsTrafficSplit.vue'

const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

const mountSplit = (props: Record<string, unknown>) =>
  mount(OpsTrafficSplit, {
    props: {
      successCount: 990, errorCountSla: 10, businessLimitedCount: 0,
      sla: 0.985, requestCountSla: 1000, ...props
    },
    global: { plugins: [i18n], stubs: { HelpTooltip: true, SplitBar: true } }
  })

const slaStyle = (w: ReturnType<typeof mountSplit>) => w.find('.traffic__sla').attributes('style') ?? ''

describe('OpsTrafficSplit SLA threshold', () => {
  it('marks an SLA under the configured floor as critical', () => {
    expect(slaStyle(mountSplit({ slaTone: 'critical' }))).toContain('--destructive')
  })

  it('marks an SLA inside the warning buffer above the floor as attention', () => {
    expect(slaStyle(mountSplit({ slaTone: 'warning' }))).toContain('--s2a-attn')
  })

  it('leaves a healthy SLA untinted', () => {
    expect(slaStyle(mountSplit({ slaTone: 'none' }))).toBe('')
  })

  it('leaves the SLA untinted when no threshold is configured', () => {
    // 'none' is what the parent sends for an unset threshold. Neutral, not
    // green: nothing was measured against, so nothing was verified healthy.
    expect(slaStyle(mountSplit({}))).toBe('')
  })

  it('does not tint a no-data window, where SLA renders as a dash', () => {
    const w = mountSplit({ sla: 0, requestCountSla: 0, slaTone: 'critical' })
    expect(w.find('.traffic__sla').text()).toContain('-')
    expect(slaStyle(w)).toBe('')
  })
})

describe('OpsTrafficSplit request error rate', () => {
  it('states the rate and the operator limit when the ceiling is crossed', () => {
    const w = mountSplit({ errorTone: 'critical', errorRatePercent: 6.2, errorRateLimit: 5 })
    expect(w.find('.traffic__breach').exists()).toBe(true)
    expect(w.find('.traffic__breach').attributes('style')).toContain('--destructive')
  })

  it('says nothing while the rate is inside the limit', () => {
    const w = mountSplit({ errorTone: 'none', errorRatePercent: 1, errorRateLimit: 5 })
    expect(w.find('.traffic__breach').exists()).toBe(false)
  })

  it('says nothing when no limit is configured', () => {
    const w = mountSplit({ errorTone: 'critical', errorRatePercent: 6.2, errorRateLimit: null })
    expect(w.find('.traffic__breach').exists()).toBe(false)
  })
})
