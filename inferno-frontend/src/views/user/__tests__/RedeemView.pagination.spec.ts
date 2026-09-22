import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import RedeemView from '../RedeemView.vue'

const { authState, getHistory, redeem, getPublicSettings, refreshUser, fetchSubscriptions, showError, showSuccess, showWarning } = vi.hoisted(() => ({
  authState: { user: null as Record<string, unknown> | null },
  getHistory: vi.fn(),
  redeem: vi.fn(),
  getPublicSettings: vi.fn(),
  refreshUser: vi.fn(),
  fetchSubscriptions: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn()
}))

vi.mock('@/api', () => ({
  redeemAPI: { getHistory, redeem },
  authAPI: { getPublicSettings }
}))

vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ ...authState, refreshUser }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess, showWarning }) }))
vi.mock('@/stores/subscriptions', () => ({ useSubscriptionStore: () => ({ fetchActiveSubscriptions: fetchSubscriptions }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: () => 'date' }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const PaginationStub = defineComponent({
  name: 'Pagination',
  props: {
    page: { type: Number, required: true },
    total: { type: Number, required: true },
    pageSize: { type: Number, required: true },
    pageSizeOptions: { type: Array, required: true }
  },
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div data-testid="pagination">
      <span>{{ page }} / {{ total }} / {{ pageSize }}</span>
      <button data-testid="next" @click="$emit('update:page', page + 1)">next</button>
      <button data-testid="size" @click="$emit('update:pageSize', 50)">size</button>
    </div>
  `
})

const item = {
  id: 1,
  code: 'REDEEM-01',
  type: 'balance',
  value: 20,
  status: 'used',
  used_at: '2026-09-01T00:00:00Z',
  created_at: '2026-09-01T00:00:00Z'
}

function mountRedeemView() {
  return mount(RedeemView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
        Pagination: PaginationStub
      }
    }
  })
}

describe('user redemption history pagination', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    getHistory.mockImplementation(async (page: number, pageSize: number) => ({
      items: [item],
      total: 61,
      page,
      page_size: pageSize,
      pages: Math.ceil(61 / pageSize)
    }))
    redeem.mockResolvedValue({ type: 'balance', value: 20, message: 'Redeemed' })
    getPublicSettings.mockResolvedValue({ contact_info: '' })
    refreshUser.mockResolvedValue(undefined)
    fetchSubscriptions.mockResolvedValue([])
    authState.user = { balance: 20, concurrency: 2 }
  })

  afterEach(() => vi.restoreAllMocks())

  it('loads and navigates server pages, then resets to page one after redeeming', async () => {
    const wrapper = mountRedeemView()
    await flushPromises()

    expect(getHistory).toHaveBeenLastCalledWith(1, 20)
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 20')
    await wrapper.get('[data-testid="next"]').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(2, 20)
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('2 / 61 / 20')

    await wrapper.get('[data-testid="size"]').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 50')

    await wrapper.get('#code').setValue('NEW-CODE')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 50')
    wrapper.unmount()
  })

  it('ignores stale results and retains the last successful page after a request failure', async () => {
    const wrapper = mountRedeemView()
    await flushPromises()

    let resolveStale!: (result: { items: Array<typeof item>; total: number; page: number; page_size: number; pages: number }) => void
    getHistory.mockImplementationOnce(() => new Promise((resolve) => { resolveStale = resolve }))
    await wrapper.get('[data-testid="next"]').trigger('click')
    expect(getHistory).toHaveBeenLastCalledWith(2, 20)

    await wrapper.get('[data-testid="size"]').trigger('click')
    await flushPromises()
    expect(getHistory).toHaveBeenLastCalledWith(1, 50)
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 50')

    resolveStale({ items: [], total: 0, page: 2, page_size: 20, pages: 0 })
    await flushPromises()
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 50')

    getHistory.mockRejectedValueOnce(new Error('request failed'))
    await wrapper.get('[data-testid="next"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="pagination"]').text()).toContain('1 / 61 / 50')
    expect(wrapper.text()).toContain('REDEEM-0...')
    expect(showError).toHaveBeenCalledWith('redeem.historyLoadFailed')
    wrapper.unmount()
  })
})
