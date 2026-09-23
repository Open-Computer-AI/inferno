import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpenCodeGoUsageCell from '../OpenCodeGoUsageCell.vue'
import type { Account, OpenCodeGoUsageState } from '@/types'

const { refreshOpenCodeGoUsage } = vi.hoisted(() => ({
  refreshOpenCodeGoUsage: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { refreshOpenCodeGoUsage }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        const labels: Record<string, string> = {
          'admin.accounts.opencodeGo.rollingShort': '5h',
          'admin.accounts.opencodeGo.weeklyShort': '7d',
          'admin.accounts.opencodeGo.monthlyShort': '1m',
          'admin.accounts.opencodeGo.unauthorized': 'unauthorized',
          'admin.accounts.opencodeGo.failed': 'failed',
          'admin.accounts.opencodeGo.ok': 'ok',
          'admin.accounts.usageWindow.activeQuery': 'query',
          'userSubscriptions.resetIn': `reset in ${params?.time ?? ''}`
        }
        return labels[key] ?? key
      }
    })
  }
})

const usageState = (overrides: Partial<OpenCodeGoUsageState> = {}): OpenCodeGoUsageState => ({
  account_id: 7,
  eligible: true,
  auto_refresh_enabled: false,
  snapshot: {
    status: 'ok',
    data: {
      rolling: { percent: 5.6, resets_at: '2099-07-23T03:00:00Z' },
      weekly: { percent: 14.2, resets_at: '2099-07-29T00:00:00Z' },
      monthly: { percent: 33.3, resets_at: '2099-08-01T00:00:00Z' }
    }
  },
  ...overrides
})

const account = (state = usageState()): Account => ({
  id: 7,
  name: 'opencode',
  platform: 'opencode_go',
  type: 'apikey',
  opencode_go_usage: state,
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
})

describe('OpenCodeGoUsageCell', () => {
  beforeEach(() => {
    refreshOpenCodeGoUsage.mockReset()
  })

  it('renders all available usage windows using June capacity bars', () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })

    expect(wrapper.find('[data-testid="opencode-go-usage-cell"]').exists()).toBe(true)
    expect(wrapper.findAll('.cap-bar')).toHaveLength(3)
    expect(wrapper.get('[data-testid="opencode-go-rolling"]').text()).toContain('6%')
    expect(wrapper.get('[data-testid="opencode-go-weekly"]').text()).toContain('14%')
    expect(wrapper.get('[data-testid="opencode-go-monthly"]').text()).toContain('33%')
  })

  it('renders a dash for a non-eligible account', () => {
    const wrapper = mount(OpenCodeGoUsageCell, {
      props: { account: account(usageState({ eligible: false })) }
    })

    expect(wrapper.find('[data-testid="opencode-go-usage-cell"]').exists()).toBe(false)
    expect(wrapper.text()).toBe('-')
  })

  it('shows an attention badge for unauthorized snapshots', () => {
    const state = usageState()
    state.snapshot!.status = 'unauthorized'
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account(state) } })
    const badge = wrapper.get('[data-testid="opencode-go-status-badge"]')

    expect(badge.text()).toBe('unauthorized')
    expect(badge.attributes('data-tone')).toBe('attn')
  })

  it('shows a danger badge for failed snapshots', () => {
    const state = usageState()
    state.snapshot!.status = 'failed'
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account(state) } })
    const badge = wrapper.get('[data-testid="opencode-go-status-badge"]')

    expect(badge.text()).toBe('failed')
    expect(badge.attributes('data-tone')).toBe('danger')
  })

  it('refreshes on demand and emits the returned state', async () => {
    const next = usageState({ auto_refresh_enabled: true })
    refreshOpenCodeGoUsage.mockResolvedValueOnce(next)
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })

    await wrapper.get('[data-testid="opencode-go-usage-query"]').trigger('click')
    await flushPromises()

    expect(refreshOpenCodeGoUsage).toHaveBeenCalledWith(7)
    expect(wrapper.emitted<OpenCodeGoUsageState[]>('updated')?.[0]?.[0]).toEqual(next)
    expect(wrapper.get('[data-testid="opencode-go-usage-query"]').attributes('disabled')).toBeUndefined()
  })

  it('keeps the cell usable when refresh fails', async () => {
    refreshOpenCodeGoUsage.mockRejectedValueOnce(new Error('refresh failed'))
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })

    await wrapper.get('[data-testid="opencode-go-usage-query"]').trigger('click')
    await flushPromises()

    expect(wrapper.emitted('updated')).toBeUndefined()
    expect(wrapper.get('[data-testid="opencode-go-usage-query"]').attributes('disabled')).toBeUndefined()
  })

  it('reacts to a replacement account usage snapshot', async () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account() } })
    const next = usageState()
    next.snapshot!.data!.rolling!.percent = 43

    await wrapper.setProps({ account: account(next) })

    expect(wrapper.get('[data-testid="opencode-go-rolling"]').text()).toContain('43%')
  })
})
