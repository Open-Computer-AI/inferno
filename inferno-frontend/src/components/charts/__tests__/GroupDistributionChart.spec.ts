import { describe, expect, it, vi } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'

import GroupDistributionChart from '../GroupDistributionChart.vue'

const messages: Record<string, string> = {
  'admin.dashboard.groupDistribution': 'Group Distribution',
  'admin.dashboard.noGroup': 'No Group',
  'admin.dashboard.accountCost': 'Account Cost',
  'admin.dashboard.noDataAvailable': 'No data available',
  'charts.distribution.othersCount': '{count} others',
  'charts.distribution.entityGroups': 'groups',
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

// The doughnut is gone -- see ModelDistributionChart.spec.ts for the same
// note. getUserBreakdown is the one real side effect a row click causes.
const getUserBreakdown = vi.fn()
vi.mock('@/api/admin/dashboard', () => ({
  getUserBreakdown: (...args: unknown[]) => getUserBreakdown(...args),
}))

describe('GroupDistributionChart', () => {
  const groupStats = [
    {
      group_id: 1,
      group_name: 'group-a',
      requests: 9,
      total_tokens: 1200,
      cost: 1.8,
      actual_cost: 0.1,
    },
    {
      group_id: 2,
      group_name: 'group-b',
      requests: 4,
      total_tokens: 600,
      cost: 0.7,
      actual_cost: 0.9,
    },
  ]

  it('uses total_tokens and token ordering by default', () => {
    // Before: doughnut `data.labels`/`data.datasets[0].data` order plus a
    // Chart.js tooltip callback producing "label: value (pct%)". Now: the
    // same three facts (rank, formatted value, share of total) read directly
    // off the bar-list row, since there is no chart and no tooltip callback.
    const wrapper = mount(GroupDistributionChart, {
      props: { groupStats },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rows = wrapper.findAll('.dist2__row')
    expect(rows).toHaveLength(2)
    expect(rows.map((r) => r.find('.dist2__label').text())).toEqual(['group-a', 'group-b'])
    expect(rows.map((r) => r.find('.dist2__value').text())).toEqual(['1.20K', '600'])

    const total = 1200 + 600
    const pctA = (1200 / total) * 100
    const pctB = (600 / total) * 100
    expect(parseFloat(rows[0].find('.dist2__fill').element.style.width)).toBeCloseTo(pctA, 6)
    expect(parseFloat(rows[1].find('.dist2__fill').element.style.width)).toBeCloseTo(pctB, 6)
  })

  it('uses actual_cost and reorders rows in actual cost mode', () => {
    const wrapper = mount(GroupDistributionChart, {
      props: { groupStats, metric: 'actual_cost' },
      global: { stubs: { LoadingSpinner: true } },
    })

    const rows = wrapper.findAll('.dist2__row')
    // Reordered: group-b ($0.900) outranks group-a ($0.100) in cost mode.
    expect(rows.map((r) => r.find('.dist2__label').text())).toEqual(['group-b', 'group-a'])
    expect(rows.map((r) => r.find('.dist2__value').text())).toEqual(['$0.900', '$0.100'])

    const total = 0.9 + 0.1
    expect(parseFloat(rows[0].find('.dist2__fill').element.style.width)).toBeCloseTo((0.9 / total) * 100, 6)
    expect(parseFloat(rows[1].find('.dist2__fill').element.style.width)).toBeCloseTo((0.1 / total) * 100, 6)
  })

  it('can hide account cost for user usage stats without account_cost', async () => {
    // Same re-expression as ModelDistributionChart: the old side table's
    // <thead th>/<tbody td> count is gone with the table itself; the column
    // set `showAccountCost` gates still exists, in UserBreakdownSubTable,
    // shown once a group row is expanded.
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

    const wrapper = mount(GroupDistributionChart, {
      props: { groupStats, showAccountCost: false },
      global: { stubs: { LoadingSpinner: true } },
    })

    await wrapper.findAll('.dist2__row')[0].trigger('click')
    await flushPromises()

    expect(getUserBreakdown).toHaveBeenCalledWith(expect.objectContaining({ group_id: 1 }))
    expect(wrapper.text()).not.toContain('Account Cost')
    expect(wrapper.findAll('thead th')).toHaveLength(5)
    expect(wrapper.findAll('tbody tr')[0].findAll('td')).toHaveLength(5)
  })
})
