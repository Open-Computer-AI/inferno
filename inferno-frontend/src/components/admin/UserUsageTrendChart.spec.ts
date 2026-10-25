import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserUsageTrendChart from './UserUsageTrendChart.vue'
import type { UserUsageTrendPoint } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

vi.mock('@/composables/useChartTokens', () => ({
  useChartTokens: () => ({ tokens: { ramp: ['#111', '#222', '#333', '#444', '#555', '#666', '#777', '#888'], brand: '#abc', mutedForeground: '#999', card: '#fff' } })
}))

const DitherAreaStub = {
  props: ['series', 'labels', 'restColor', 'markerColor', 'valueFormatter'],
  template: '<div data-testid="dither-area" />'
}

const points: UserUsageTrendPoint[] = [
  { date: '2026-01-01', user_id: 1, email: 'tokens@example.test', username: 'tokens', requests: 1, tokens: 1000, cost: 1, actual_cost: 0.25 },
  { date: '2026-01-01', user_id: 2, email: 'spender@example.test', username: 'spender', requests: 1, tokens: 10, cost: 8, actual_cost: 5 }
]

describe('UserUsageTrendChart metric selection', () => {
  it('ranks and plots actual spending, formats the total/readout as currency, and emits metric changes', async () => {
    const wrapper = mount(UserUsageTrendChart, {
      props: { points, metric: 'actual_cost' },
      global: { stubs: { DitherArea: DitherAreaStub, LoadingSpinner: true } }
    })

    expect(wrapper.get('.utrend__total').text()).toBe('$5.25')
    expect(wrapper.get('.utrend__sub').text()).toBe('admin.dashboard.spendingAcrossUsers')
    const area = wrapper.findComponent(DitherAreaStub)
    expect(area.props('series').map((series: { label: string; values: number[] }) => [series.label, series.values])).toEqual([
      ['spender', [5]],
      ['tokens', [0.25]]
    ])
    expect(area.props('valueFormatter')(5)).toBe('$5.00')
    expect(wrapper.get('button[aria-pressed="true"]').text()).toBe('admin.dashboard.actualSpending')

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('update:metric')).toEqual([['tokens']])
  })
})
