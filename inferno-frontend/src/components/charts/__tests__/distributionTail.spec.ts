/**
 * The ranked bar list must not hide the tail.
 *
 * The June conversion replaced Chart.js doughnuts with a ranked bar list --
 * that part stands. TOP_N = 6 was a separate compactness choice bolted on top,
 * and it made every model/endpoint past rank 6 unreachable: the "N others" row
 * was not clickable, so drill-down simply stopped. The bar list renders any
 * number of rows, so the cap costs nothing as long as the tail EXPANDS.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ModelDistributionChart from '../ModelDistributionChart.vue'

// This build has no runtime message compiler, so t() echoes the key back.
// Anchor on the row's own marker (dist2__value--muted / data-clickable) rather
// than on label text, or the assertions pass by matching the key itself.
const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })

const othersRow = (w: { findAll: (s: string) => any[] }) =>
  w.findAll('.dist2__row').find((r: any) => r.find('.dist2__value--muted').exists())

const modelStats = Array.from({ length: 10 }, (_, i) => ({
  model: `model-${i}`, requests: 10 - i, total_tokens: (10 - i) * 1000, actual_cost: 10 - i
}))

const mountChart = () =>
  mount(ModelDistributionChart, {
    props: { modelStats, loading: false, enableBreakdown: true },
    global: { plugins: [i18n], stubs: { LoadingSpinner: true, Icon: true, Segmented: true, UserBreakdownSubTable: true } }
  })

const labels = (w: ReturnType<typeof mountChart>) =>
  w.findAll('.dist2__label').map((n) => n.text())

describe('ModelDistributionChart tail', () => {
  it('collapses past the sixth row by default', () => {
    const l = labels(mountChart())
    expect(l).toContain('model-0')
    expect(l).not.toContain('model-9')
    expect(l).toHaveLength(7) // six models plus the collapsed tail row
    expect(othersRow(mountChart())).toBeDefined()
  })

  it('reveals every remaining model when the others row is clicked', async () => {
    const w = mountChart()
    const others = othersRow(w)
    expect(others).toBeDefined()
    await others!.trigger('click')
    const l = labels(w)
    for (const s of modelStats) expect(l).toContain(s.model)
  })

  it('keeps the others row clickable so the tail is reachable at all', () => {
    expect(othersRow(mountChart())!.attributes('data-clickable')).toBeDefined()
  })
})
