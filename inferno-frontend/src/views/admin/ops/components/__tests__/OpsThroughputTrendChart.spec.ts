import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import OpsThroughputTrendChart from '../OpsThroughputTrendChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../OpsThroughputTrendChart.vue'),
  'utf8'
)

const mountChart = (points: unknown[] = []) =>
  mount(OpsThroughputTrendChart, {
    props: { points, loading: false, timeRange: '1h' } as never,
    global: { stubs: { EmptyState: true, HelpTooltip: true, LoadingSpinner: true } },
  })

describe('OpsThroughputTrendChart', () => {
  /*
   * This used to assert Tailwind class names on the header -- flex-col,
   * sm:flex-row, w-full, shrink-0 -- which stopped existing when the component
   * moved to scoped CSS, and which were testing the implementation rather than
   * the guarantee. The guarantee is that the toolbar wraps instead of
   * overflowing its card, so that is what is asserted now: the rule exists in
   * the component's own styles, and the toolbar is still addressable.
   */
  it('lets the header toolbar wrap rather than overflow', () => {
    const wrapper = mountChart()
    const toolbar = wrapper.get('[data-testid="throughput-chart-toolbar"]')
    expect(toolbar.findAll('button').length).toBeGreaterThan(0)

    // Chips are the other row that must wrap, and they can be many.
    expect(source).toMatch(/\.opsthru__chips\s*\{[^}]*flex-wrap:\s*wrap/)
    // Buttons must not shrink into ellipsis when the row gets tight.
    expect(source).toMatch(/\.opsthru__actions\s*\{[^}]*flex:\s*none/)
  })

  /*
   * QPS and TPS are different units, so they get one strip each with its own
   * scale rather than one chart with two y axes. Two axes let a reader compare
   * the heights of two lines that share no scale, and any crossover between
   * them is an artefact of where the axes were placed.
   */
  it('renders throughput as two separate strips, never one dual-axis chart', () => {
    const wrapper = mountChart([
      { bucket_start: '2026-05-08T00:00:00Z', request_count: 10, qps: 0.2, tps: 1200 },
      { bucket_start: '2026-05-08T00:01:00Z', request_count: 20, qps: 0.4, tps: 2400 },
    ])
    expect(wrapper.findAll('.opsthru__strip')).toHaveLength(2)
    expect(source).not.toContain('yAxisID')
  })

  it('shows the empty state and no strips without traffic', () => {
    const wrapper = mountChart()
    expect(wrapper.findAll('.opsthru__strip')).toHaveLength(0)
    expect(wrapper.find('.opsthru__state').exists()).toBe(true)
  })
})
