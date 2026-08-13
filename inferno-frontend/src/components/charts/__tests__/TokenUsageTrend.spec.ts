import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import TokenUsageTrend from '../TokenUsageTrend.vue'
import DitherArea from '../DitherArea.vue'

const messages: Record<string, string> = {
  'admin.dashboard.tokenUsageTrend': 'Token Usage Trend',
  'admin.dashboard.noDataAvailable': 'No data available',
  'admin.dashboard.input': 'Input',
  'admin.dashboard.output': 'Output',
  'admin.dashboard.cache': 'Cache',
  /* Lives under `usage.`, not `dashboard.`. The component asked for
     dashboard.cacheHitRate, which exists in neither the en nor the zh locale,
     so the raw key rendered on screen under the trend chart. This stub had
     been masking that by defining the key the component wanted rather than
     the one the app ships. */
  'usage.cacheHitRate': 'Cache hit rate',
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

/*
 * The component renders DitherArea, a canvas, which exposes nothing
 * assertable. Rather than stub it, this reads the real child's PROPS: the
 * contract under test is which series get built with which values, and props
 * are exactly that contract. A stub would also have to compile a string
 * template, which the runtime-only Vue build in this environment cannot do --
 * it silently mounts the real component instead, which is how this was
 * quietly passing nothing.
 *
 * The guarantees are unchanged from the Chart.js era: how the cache hit rate
 * is computed, and that cache creation + read merge into one series.
 */
const readChart = (wrapper: ReturnType<typeof mount>, selector: string) => {
  const host = wrapper.find(selector)
  if (!host.exists()) return null
  const chart = host.findComponent(DitherArea)
  if (!chart.exists()) return null
  const series = chart.props('series') as { label: string; values: number[] }[]
  return {
    datasets: series.map((s) => ({ label: s.label, data: s.values })),
    labels: chart.props('labels') as string[]
  }
}

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
    expect(rateChart.datasets[0].label).toBe('Cache hit rate')
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

  it('stacks the three volume series bottom-first and shares one x axis with the rate strip', () => {
    /*
     * Order is the assertion that matters. The series are now STACKED, so
     * their order is their vertical order, and cache goes first because it is
     * the floor of the stack and the largest band -- the deepest ramp step
     * belongs at the bottom. Under Chart.js they were three overlaid lines and
     * the order was arbitrary; here reordering them would silently reshape the
     * chart.
     *
     * Both plots still share x labels, which is what lets the rate strip be
     * read against the volume above it.
     */
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

    expect(mainChart.datasets.map((ds: { label: string }) => ds.label)).toEqual(['Cache', 'Input', 'Output'])
    expect(rateChart.datasets.map((ds: { label: string }) => ds.label)).toEqual(['Cache hit rate'])

    /*
     * The per-series colour assertions are gone with Chart.js. They pinned
     * each series to an index of the CATEGORICAL ramp, which walks the hue so
     * no two series look related -- correct for overlaid lines, wrong for a
     * stack, where the bands are parts of one quantity and want a sequential
     * deep-to-pale ramp instead. That ramp resolves through getComputedStyle,
     * which jsdom returns empty for, so there is nothing meaningful left to
     * assert about colour here; the browser is where that gets verified.
     *
     * `fill` is gone for the same reason: every band is filled now. It existed
     * to say "input alone carries the area", which was a property of drawing
     * three lines, not of the data.
     */
    expect(mainChart.datasets[0].data).toEqual([1500, 950]) // cache read + creation
    expect(mainChart.datasets[1].data).toEqual([500, 400]) // input
    expect(mainChart.datasets[2].data).toEqual([100, 120]) // output
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
