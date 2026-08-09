import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

import ModelDistributionChart from '../ModelDistributionChart.vue'

const messages: Record<string, string> = {
  'admin.dashboard.modelDistribution': 'Model Distribution',
  'admin.dashboard.spendingRankingTitle': 'User Spending Ranking',
  'admin.dashboard.viewModelDistribution': 'Model Distribution',
  'admin.dashboard.viewSpendingRanking': 'User Spending Ranking',
  'admin.dashboard.spendingRankingOther': 'Others',
  'admin.dashboard.accountCost': 'Account Cost',
  'admin.dashboard.noDataAvailable': 'No data available',
  'admin.redeem.userPrefix': 'User #{id}',
  'charts.distribution.othersCount': '{count} others',
  'charts.distribution.entityModels': 'models',
  'charts.distribution.entityUsers': 'users',
  'charts.distribution.subtitleTokens': '{value} across {count} {entity}',
  'charts.distribution.subtitleCost': '{value} across {count} {entity}',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const message = messages[key] ?? key
        return params
          ? message.replace(/\{(\w+)\}/g, (_, name: string) => String(params[name]))
          : message
      },
    }),
  }
})

// The doughnut and its `Doughnut`/`chart-data` stub are gone -- distribution
// is a ranked bar list now (dist2__row / dist2__label / dist2__value /
// dist2__fill), so there is nothing in vue-chartjs left for this component to
// mock. getUserBreakdown is the one real side effect a row click causes.
const getUserBreakdown = vi.fn()
vi.mock('@/api/admin/dashboard', () => ({
  getUserBreakdown: (...args: unknown[]) => getUserBreakdown(...args),
}))

describe('ModelDistributionChart', () => {
  const modelStats = [
    {
      model: 'model-a',
      requests: 8,
      input_tokens: 100,
      output_tokens: 50,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      total_tokens: 1000,
      cost: 1.5,
      actual_cost: 0.2,
    },
    {
      model: 'model-b',
      requests: 3,
      input_tokens: 40,
      output_tokens: 20,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      total_tokens: 500,
      cost: 0.5,
      actual_cost: 1.4,
    },
  ]

  it('uses total_tokens and token ordering by default', () => {
    // Before the redesign this asserted the doughnut's `data.labels`/
    // `data.datasets[0].data` order plus a Chart.js tooltip callback that
    // formatted "label: value (pct%)". There is no doughnut and no tooltip
    // callback any more -- the same three facts (which entry is on top,
    // its formatted value, its share of the total) are now read straight off
    // the row instead of out of a mocked chart prop / a tooltip formatter.
    const wrapper = mount(ModelDistributionChart, {
      props: { modelStats },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rows = wrapper.findAll('.dist2__row')
    expect(rows).toHaveLength(2)

    // Ordering: model-a (1000 tokens) outranks model-b (500 tokens).
    expect(rows.map((r) => r.find('.dist2__label').text())).toEqual(['model-a', 'model-b'])
    // Formatted value, same formatTokens() the old tooltip used.
    expect(rows.map((r) => r.find('.dist2__value').text())).toEqual(['1.00K', '500'])
    // Model identifiers render mono (prototype's own barList example puts
    // every named row in --font-mono).
    expect(rows[0].find('.dist2__label').classes()).toContain('dist2__label--mono')

    // Share of the metric total -- what the old tooltip's "(66.7%)" was.
    // Computed the same way the component computes it, so this is an exact
    // check, not a rounded approximation.
    const total = 1000 + 500
    const pctA = (1000 / total) * 100
    const pctB = (500 / total) * 100
    expect(parseFloat(rows[0].find('.dist2__fill').element.style.width)).toBeCloseTo(pctA, 6)
    expect(parseFloat(rows[1].find('.dist2__fill').element.style.width)).toBeCloseTo(pctB, 6)
  })

  it('uses actual_cost and reorders rows in actual cost mode', () => {
    const wrapper = mount(ModelDistributionChart, {
      props: { modelStats, metric: 'actual_cost' },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rows = wrapper.findAll('.dist2__row')
    // Reordered: model-b ($1.40) outranks model-a ($0.200) in cost mode.
    expect(rows.map((r) => r.find('.dist2__label').text())).toEqual(['model-b', 'model-a'])
    expect(rows.map((r) => r.find('.dist2__value').text())).toEqual(['$1.40', '$0.200'])

    const total = 1.4 + 0.2
    expect(parseFloat(rows[0].find('.dist2__fill').element.style.width)).toBeCloseTo((1.4 / total) * 100, 6)
    expect(parseFloat(rows[1].find('.dist2__fill').element.style.width)).toBeCloseTo((0.2 / total) * 100, 6)
  })

  it('can hide account cost for user usage stats without account_cost', async () => {
    // The old assertion counted <thead th>/<tbody td> on the side-by-side
    // table that used to sit next to the doughnut -- that table is gone, the
    // bar list has no columns at all. The `showAccountCost` prop still
    // exists, but it now only reaches the one place a column set still
    // renders: UserBreakdownSubTable, shown when a row is expanded. Same
    // guarantee (hiding account cost drops one column), re-expressed against
    // where the column set actually lives now.
    getUserBreakdown.mockReset()
    getUserBreakdown.mockResolvedValue({
      users: [
        {
          user_id: 1,
          email: 'user1@example.com',
          requests: 4,
          input_tokens: 10,
          output_tokens: 5,
          cache_tokens: 0,
          total_tokens: 15,
          cost: 0.02,
          actual_cost: 0.01,
          account_cost: 0.03,
        },
      ],
      start_date: '2026-08-01',
      end_date: '2026-08-08',
    })

    const wrapper = mount(ModelDistributionChart, {
      props: { modelStats, showAccountCost: false },
      global: { stubs: { LoadingSpinner: true } },
    })

    await wrapper.findAll('.dist2__row')[0].trigger('click')
    await flushPromises()

    expect(getUserBreakdown).toHaveBeenCalledWith(
      expect.objectContaining({ model: 'model-a', model_source: 'requested' })
    )
    expect(wrapper.text()).not.toContain('Account Cost')
    expect(wrapper.findAll('thead th')).toHaveLength(5)
    expect(wrapper.findAll('tbody tr')[0].findAll('td')).toHaveLength(5)
  })

  it('uses the dashboard user label policy, collapses the remainder into one Others row styled off-brand, and emits ranking-click', async () => {
    // Same guarantees as before the redesign -- username-over-email-over-
    // "User #id" labelling, an aggregated Others row, Others visually
    // distinguished from the ranked entries -- re-expressed against the bar
    // list. The old test read a Chart.js `backgroundColor` array for the
    // "dedicated colour" guarantee (two hardcoded hex values that were never
    // June tokens to begin with); the redesign spends colour on state, not
    // category, so the real distinguishing signal now is that Others uses
    // `var(--muted-foreground)` where every ranked row uses `var(--brand)`,
    // and Others is not clickable while ranked rows are (and emit
    // `ranking-click`, a guarantee the old test never actually exercised).
    const wrapper = mount(ModelDistributionChart, {
      props: {
        modelStats: [],
        enableRankingView: true,
        rankingItems: [
          { user_id: 1, email: 'alpha@example.com', username: 'alpha', actual_cost: 12, requests: 10, tokens: 1000 },
          { user_id: 2, email: 'beta@example.com', username: '   ', actual_cost: 8, requests: 6, tokens: 600 },
          { user_id: 3, email: '   ', username: '', actual_cost: 0, requests: 0, tokens: 0 },
        ],
        rankingTotalActualCost: 30,
        rankingTotalRequests: 20,
        rankingTotalTokens: 2000,
      },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rankingButton = wrapper.findAll('button').find((button) => button.text() === 'User Spending Ranking')
    expect(rankingButton).toBeTruthy()
    await rankingButton!.trigger('click')

    const rows = wrapper.findAll('.dist2__row')
    expect(rows).toHaveLength(4)
    expect(rows.map((r) => r.find('.dist2__label').text())).toEqual([
      '#1 alpha',
      '#2 beta@example.com',
      '#3 User #3',
      'Others',
    ])
    expect(rows.map((r) => r.find('.dist2__value').text())).toEqual(['$12.00', '$8.00', '$0.0000', '$10.00'])

    const othersRow = rows[3]
    expect(othersRow.attributes('data-clickable')).toBeUndefined()
    expect(othersRow.find('.dist2__value').classes()).toContain('dist2__value--muted')
    expect(othersRow.find('.dist2__fill').attributes('style')).toContain('var(--muted-foreground)')

    const alphaRow = rows[0]
    expect(alphaRow.attributes('data-clickable')).toBeDefined()
    expect(alphaRow.find('.dist2__fill').attributes('style')).toContain('var(--brand)')

    await alphaRow.trigger('click')
    expect(wrapper.emitted('ranking-click')).toHaveLength(1)
    expect(wrapper.emitted('ranking-click')![0][0]).toMatchObject({ user_id: 1, actual_cost: 12 })
  })
})
