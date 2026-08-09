import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenUsageTrend from '../TokenUsageTrend.vue'

const messages: Record<string, string> = {
  'admin.dashboard.tokenUsageTrend': 'Token Usage Trend',
  'admin.dashboard.noDataAvailable': 'No data available',
  'admin.dashboard.input': 'Input',
  'admin.dashboard.output': 'Output',
  'admin.dashboard.cache': 'Cache',
  'dashboard.cacheHitRate': 'Cache Hit Rate',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>',
  },
}))

// jsdom cannot resolve color-mix()/oklch(), so the real useChartTokens()
// resolves every token to '' under test -- structurally valid, but useless
// for asserting "the right series is wired to the right ramp step". Stub the
// composable with fixed, distinguishable literals instead (the same shape
// `resolveAll()` produces) and assert against those literals -- structure and
// wiring, never a resolved colour.
vi.mock('@/composables/useChartTokens', () => ({
  useChartTokens: () => ({
    tokens: {
      ramp: ['ramp-0', 'ramp-1', 'ramp-2', 'ramp-3', 'ramp-4', 'ramp-5', 'ramp-6', 'ramp-7'],
      brand: 'stub-brand',
      brandTint: 'stub-brand-tint',
      foreground: 'stub-fg',
      bodyCopy: 'stub-body',
      mutedForeground: 'stub-muted',
      border: 'stub-border',
      borderSubtle: 'stub-border-subtle',
      card: 'stub-card',
      surfaceSubtle: 'stub-surface-subtle',
      attention: 'stub-attn',
      success: 'stub-success',
      destructive: 'stub-destructive',
    },
    prefersReducedMotion: { value: false },
  }),
}))

const readChart = (wrapper: ReturnType<typeof mount>, selector: string) =>
  JSON.parse(wrapper.find(`${selector} .chart-data`).text())

describe('TokenUsageTrend', () => {
  it('calculates cache hit rate against all prompt tokens, on the dedicated rate-strip chart', () => {
    // Before the redesign this was one dual-axis chart and the test read the
    // "Cache Hit Rate" dataset out of its single mocked `data` prop. Now the
    // rate lives on its own Chart.js instance (`.trend2__rate-chart`, a 34px
    // strip with its own 0-100 scale) -- the calculation guarantee is
    // unchanged, only which chart carries it.
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 500,
            output_tokens: 100,
            cache_creation_tokens: 0,
            cache_read_tokens: 1500,
            cost: 0.01,
            actual_cost: 0.005,
          },
        ],
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rateChart = readChart(wrapper, '.trend2__rate-chart')
    expect(rateChart.datasets).toHaveLength(1)
    expect(rateChart.datasets[0].label).toBe('Cache Hit Rate')
    // Hit rate = 1500 / (500 + 1500 + 0) * 100 = 75%
    expect(rateChart.datasets[0].data[0]).toBe(75)
  })

  it('returns 0 hit rate when all prompt tokens are zero', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 0,
            input_tokens: 0,
            output_tokens: 0,
            cache_creation_tokens: 0,
            cache_read_tokens: 0,
            cost: 0,
            actual_cost: 0,
          },
        ],
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rateChart = readChart(wrapper, '.trend2__rate-chart')
    expect(rateChart.datasets[0].data[0]).toBe(0)
  })

  it('includes cache_creation_tokens in the rate denominator, and merges cache creation + read into one series on the main chart', () => {
    // Two guarantees in one scenario: the rate formula (unchanged, just
    // re-targeted at the rate-strip chart, same as the other two tests), and
    // the migration note's main new behaviour -- "cache creation and cache
    // read merged into one series... both numbers still in the tooltip." The
    // merge is asserted on the main chart's data; the tooltip half of that
    // claim lives in a Chart.js callback function, which a JSON-serialized
    // stub prop cannot capture (functions drop out of JSON.stringify), so it
    // is not re-asserted here -- see the report for why that is a stated gap
    // rather than a silent one.
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 200,
            output_tokens: 50,
            cache_creation_tokens: 300,
            cache_read_tokens: 500,
            cost: 0.02,
            actual_cost: 0.01,
          },
        ],
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rateChart = readChart(wrapper, '.trend2__rate-chart')
    // Hit rate = 500 / (200 + 500 + 300) * 100 = 50%
    expect(rateChart.datasets[0].data[0]).toBe(50)

    const mainChart = readChart(wrapper, '.trend2__main')
    const cacheSeries = mainChart.datasets.find((ds: { label: string }) => ds.label === 'Cache')
    expect(cacheSeries).toBeTruthy()
    expect(cacheSeries.data[0]).toBe(300 + 500)
  })

  it('renders two independent Chart.js line instances sharing one x axis, each wired to its own ramp step', () => {
    // The structural claim the redesign makes and the old test never
    // covered (it only had one chart to check): "two Chart.js Line instances
    // instead of one dual-axis chart... share x labels". Regression coverage
    // for that split, plus proof each series reads its own ramp index
    // (input ramp[0], output ramp[2], cache ramp[7], rate ramp[2]) rather
    // than asserting any resolved colour (jsdom can't produce one -- see the
    // composable stub above).
    const wrapper = mount(TokenUsageTrend, {
      props: {
        trendData: [
          {
            date: '2026-05-08',
            requests: 1,
            input_tokens: 500,
            output_tokens: 100,
            cache_creation_tokens: 0,
            cache_read_tokens: 1500,
            cost: 0.01,
            actual_cost: 0.005,
          },
          {
            date: '2026-05-09',
            requests: 2,
            input_tokens: 400,
            output_tokens: 120,
            cache_creation_tokens: 50,
            cache_read_tokens: 900,
            cost: 0.02,
            actual_cost: 0.011,
          },
        ],
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const mainChart = readChart(wrapper, '.trend2__main')
    const rateChart = readChart(wrapper, '.trend2__rate-chart')

    expect(mainChart.labels).toEqual(['2026-05-08', '2026-05-09'])
    expect(rateChart.labels).toEqual(mainChart.labels)

    expect(mainChart.datasets.map((ds: { label: string }) => ds.label)).toEqual(['Input', 'Output', 'Cache'])
    expect(rateChart.datasets.map((ds: { label: string }) => ds.label)).toEqual(['Cache Hit Rate'])

    expect(mainChart.datasets[0].borderColor).toBe('ramp-0') // Input
    expect(mainChart.datasets[1].borderColor).toBe('ramp-2') // Output
    expect(mainChart.datasets[2].borderColor).toBe('ramp-7') // Cache, dashed
    expect(rateChart.datasets[0].borderColor).toBe('ramp-2') // shares Output's step

    // Only the Input series carries the area fill (migration note: "Input
    // alone carries the area fill").
    expect(mainChart.datasets[0].fill).toBe(true)
    expect(mainChart.datasets[1].fill).toBe(false)
    expect(mainChart.datasets[2].fill).toBe(false)
  })

  it('shows the empty state and renders neither chart when there is no trend data', () => {
    const wrapper = mount(TokenUsageTrend, {
      props: { trendData: [] },
      global: { stubs: { LoadingSpinner: true } },
    })

    expect(wrapper.find('.chart-data').exists()).toBe(false)
    expect(wrapper.text()).toContain('No data available')
  })
})
