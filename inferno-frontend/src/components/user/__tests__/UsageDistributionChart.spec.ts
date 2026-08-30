/**
 * The row list must not end silently at six.
 *
 * TOP_ROWS = 6 capped the row list with no way to open it, so every endpoint
 * past rank 6 left the UI entirely. The cap is compactness, not a statement
 * about what exists.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import UsageDistributionChart from '../UsageDistributionChart.vue'

const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

const rows = Array.from({ length: 10 }, (_, i) => ({
  key: `e${i}`, label: `/v1/endpoint-${i}`,
  requests: 10 - i, tokens: (10 - i) * 100, actualCost: 10 - i, standardCost: 10 - i
}))

const mountChart = (props: Record<string, unknown> = {}) =>
  mount(UsageDistributionChart, {
    props: { title: 'Endpoints', rows, ...props },
    global: { plugins: [i18n], stubs: { LoadingSpinner: true, EmptyState: true, DitherDonut: true } }
  })

describe('UsageDistributionChart row tail', () => {
  it('lists six rows and offers the rest', () => {
    const w = mountChart()
    expect(w.findAll('tbody tr').length).toBe(6)
    expect(w.find('.usage-dist__more').exists()).toBe(true)
  })

  it('reveals every remaining row when opened', async () => {
    const w = mountChart()
    await w.find('.usage-dist__more').trigger('click')
    expect(w.findAll('tbody tr').length).toBe(10)
  })

  it('offers nothing to open when everything already fits', () => {
    expect(mountChart({ rows: rows.slice(0, 4) }).find('.usage-dist__more').exists()).toBe(false)
  })
})
