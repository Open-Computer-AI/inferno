import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { OpsDashboardOverview } from '@/api/admin/ops'
import { flushPromises, mount, shallowMount } from '@vue/test-utils'
import OpsStatCard from '../OpsStatCard.vue'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'
import { formatDateTimeLocalInput } from '@/utils/format'
import OpsRequestDetailsModal from '../OpsRequestDetailsModal.vue'
import { resetAccountNameCacheForTests } from '../../utils/accountNameMap'

const { listRequestDetails, listAccounts, viewport } = vi.hoisted(() => ({
  listRequestDetails: vi.fn(),
  listAccounts: vi.fn(),
  viewport: { desktop: true },
}))

vi.mock('@vueuse/core', () => ({ useMediaQuery: () => ref(viewport.desktop) }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { listRequestDetails } }))
vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { list: listAccounts } }))
vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
  useAdminSettingsStore: () => ({ opsRealtimeMonitoringEnabled: false }),
}))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

async function openDetails(sort: 'duration_desc' | 'ttft_desc') {
  const wrapper = mount(OpsRequestDetailsModal, {
    props: { modelValue: false, timeRange: '1h', preset: { title: 'Details', sort } },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Pagination: true } },
  })
  await wrapper.setProps({ modelValue: true })
  await flushPromises()
  return wrapper
}

describe('Ops request latency details', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resetAccountNameCacheForTests()
    viewport.desktop = true
    listRequestDetails.mockResolvedValue({
      items: [
        { kind: 'success', created_at: '2026-09-10T00:00:00Z', duration_ms: 12000, first_token_ms: 800, account_id: 101 },
        { kind: 'success', created_at: '2026-09-10T00:00:01Z', duration_ms: 9000, first_token_ms: 0, account_id: 102 },
        { kind: 'success', created_at: '2026-09-10T00:00:02Z', duration_ms: 5000, first_token_ms: null, account_id: 999 },
      ],
      total: 3,
    })
    listAccounts.mockResolvedValue({ items: [{ id: 101, name: 'acc-alpha' }, { id: 102, name: 'acc-beta' }], total: 2 })
  })

  it('opens the TTFT details with successful requests sorted by first-token latency', async () => {
    const wrapper = shallowMount(OpsDashboardHeader, {
      props: { overview: {} as OpsDashboardOverview, platform: '', groupId: null, timeRange: '1h', queryMode: 'auto', loading: false, lastUpdated: null },
    })
    await flushPromises()
    const ttftCard = wrapper.findAllComponents(OpsStatCard).find((card) => card.props('title') === 'admin.ops.ttftLabel')
    expect(ttftCard).toBeDefined()
    ttftCard!.vm.$emit('open-details')
    expect(wrapper.emitted('openRequestDetails')).toEqual([[
      { title: 'admin.ops.ttftLabel', kind: 'success', sort: 'ttft_desc' },
    ]])
    wrapper.unmount()
  })

  it.each([true, false])('shows TTFT rather than total duration (desktop: %s)', async (desktop) => {
    viewport.desktop = desktop
    const wrapper = await openDetails('ttft_desc')
    expect(listRequestDetails).toHaveBeenCalledWith(expect.objectContaining({ sort: 'ttft_desc' }))
    expect(wrapper.text()).toContain('admin.ops.ttftLabel')
    expect(wrapper.text()).toContain('800 ms')
    expect(wrapper.text()).toContain('0 ms')
    expect(wrapper.text()).not.toContain('12000 ms')
    expect(wrapper.text()).not.toContain('9000 ms')
    expect(wrapper.text()).not.toContain('5000 ms')
    if (desktop) expect(wrapper.findAll('tbody tr')[2].findAll('td')[5].text()).toBe('-')
    else expect(wrapper.text()).toContain('admin.ops.ttftLabel: -')
    wrapper.unmount()
  })

  it('shows account names with an id fallback in the account column', async () => {
    const wrapper = await openDetails('ttft_desc')
    expect(wrapper.text()).toContain('admin.ops.requestDetails.table.account')
    const accountCells = wrapper.findAll('tbody tr').map(tr => tr.findAll('td')[4].text())
    expect(accountCells).toEqual(['acc-alpha', 'acc-beta', '#999'])
    expect(listAccounts).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('initializes the custom range using local datetime-local values', async () => {
    const wrapper = shallowMount(OpsDashboardHeader, {
      props: { overview: {} as OpsDashboardOverview, platform: '', groupId: null, timeRange: '1h', queryMode: 'auto', loading: false, lastUpdated: null },
    })
    const vm = wrapper.vm as unknown as {
      handleTimeRangeChange: (value: string) => void
      customStartTimeInput: string
      customEndTimeInput: string
    }
    const before = new Date()
    vm.handleTimeRangeChange('custom')
    const start = vm.customStartTimeInput
    const end = vm.customEndTimeInput
    const after = new Date()
    expect(end).toBe(formatDateTimeLocalInput(Math.floor(after.getTime() / 1000)))
    expect(start).toBe(formatDateTimeLocalInput(Math.floor((before.getTime() - 60 * 60 * 1000) / 1000)))
    expect(start).not.toContain('Z')
    expect(end).not.toContain('Z')
    wrapper.unmount()
  })

  it('keeps total duration for duration details', async () => {
    const wrapper = await openDetails('duration_desc')
    expect(listRequestDetails).toHaveBeenCalledWith(expect.objectContaining({ sort: 'duration_desc' }))
    expect(wrapper.text()).toContain('admin.ops.requestDetails.table.duration')
    expect(wrapper.text()).toContain('12000 ms')
    expect(wrapper.text()).not.toContain('800 ms')
    wrapper.unmount()
  })
})
