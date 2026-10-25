import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import UsageCleanupDialog from '../UsageCleanupDialog.vue'
import Pagination from '@/components/common/Pagination.vue'

const { listCleanupTasks, showError } = vi.hoisted(() => ({ listCleanupTasks: vi.fn(), showError: vi.fn() }))
vi.mock('@/components/admin/usage/UsageFilters.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/admin/usage', () => ({ adminUsageAPI: { listCleanupTasks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

const page = (number: number) => ({
  items: [{ id: number, status: 'succeeded', deleted_rows: number, filters: {} }],
  total: 15,
  page: number,
  page_size: 5
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const button = (wrapper: VueWrapper, text: string) =>
  wrapper.findAll('button').find(item => item.text() === text)!

async function openDialog() {
  listCleanupTasks.mockResolvedValueOnce(page(1))
  const wrapper = mount(UsageCleanupDialog, {
    props: { show: false, filters: {}, startDate: '2026-09-01', endDate: '2026-09-02' },
    global: { stubs: { BaseDialog: { template: '<div><slot/><slot name="footer"/></div>' }, UsageFilters: true, ConfirmDialog: true, Pagination: true } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

async function startPageRequest(wrapper: VueWrapper, pageNumber: number) {
  const request = deferred<ReturnType<typeof page>>()
  listCleanupTasks.mockImplementationOnce(() => request.promise)
  wrapper.findComponent(Pagination).vm.$emit('update:page', pageNumber)
  await flushPromises()
  return request
}

beforeEach(() => {
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())

describe('cleanup task pagination', () => {
  it.each(['success', 'failure'])('ignores an older request that completes after the latest %s', async (outcome) => {
    const wrapper = await openDialog()
    const older = await startPageRequest(wrapper, 2)
    listCleanupTasks.mockResolvedValueOnce(page(3))
    wrapper.findComponent(Pagination).vm.$emit('update:page', 3)
    await flushPromises()

    if (outcome === 'success') older.resolve(page(2))
    else older.reject(new Error('old failure'))
    await flushPromises()

    expect(wrapper.findComponent(Pagination).props('page')).toBe(3)
    expect(wrapper.text()).toContain('#3')
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps the latest loading state when an older success arrives first', async () => {
    const wrapper = await openDialog()
    const older = await startPageRequest(wrapper, 2)
    const latest = await startPageRequest(wrapper, 3)

    older.resolve(page(2))
    await flushPromises()
    expect(wrapper.text()).toContain('admin.usage.cleanup.loadingTasks')
    expect(wrapper.text()).not.toContain('#2')
    expect(showError).not.toHaveBeenCalled()

    latest.resolve(page(3))
    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.usage.cleanup.loadingTasks')
    expect(wrapper.text()).toContain('#3')
  })

  it('keeps the latest loading state when an older failure arrives first', async () => {
    const wrapper = await openDialog()
    const older = await startPageRequest(wrapper, 2)
    const latest = await startPageRequest(wrapper, 3)

    older.reject(new Error('old failure'))
    await flushPromises()
    expect(wrapper.text()).toContain('admin.usage.cleanup.loadingTasks')
    expect(showError).not.toHaveBeenCalled()

    latest.resolve(page(3))
    await flushPromises()
    expect(wrapper.text()).not.toContain('admin.usage.cleanup.loadingTasks')
    expect(wrapper.text()).toContain('#3')
  })

  it('shows the current request error and clears its loading state', async () => {
    const wrapper = await openDialog()
    const latest = await startPageRequest(wrapper, 2)

    latest.reject(new Error('current failure'))
    await flushPromises()

    expect(showError).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith('admin.usage.cleanup.loadFailed')
    expect(wrapper.text()).not.toContain('admin.usage.cleanup.loadingTasks')
    expect(wrapper.text()).toContain('#1')
  })

  it('ignores a pending completion after close and preserves upstream loading behavior', async () => {
    const wrapper = await openDialog()
    const request = await startPageRequest(wrapper, 2)
    await button(wrapper, 'common.cancel').trigger('click')
    expect(wrapper.emitted('close')).toHaveLength(1)

    request.resolve(page(2))
    await flushPromises()
    expect(wrapper.text()).toContain('admin.usage.cleanup.loadingTasks')
    expect(wrapper.text()).not.toContain('#2')
  })

  it('ignores a pending result after the dialog is hidden', async () => {
    const wrapper = await openDialog()
    const request = await startPageRequest(wrapper, 2)
    await wrapper.setProps({ show: false })

    request.resolve(page(2))
    await flushPromises()
    expect(wrapper.text()).not.toContain('#2')
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores a pending rejection after unmount', async () => {
    const wrapper = await openDialog()
    const request = await startPageRequest(wrapper, 2)
    wrapper.unmount()

    request.reject(new Error('late failure'))
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
  })

  it.each(['success', 'failure'])('ignores an old request after close and reopen (%s)', async (outcome) => {
    const wrapper = await openDialog()
    const older = await startPageRequest(wrapper, 2)
    await wrapper.setProps({ show: false })
    listCleanupTasks.mockResolvedValueOnce(page(1))
    await wrapper.setProps({ show: true })
    await flushPromises()

    if (outcome === 'success') older.resolve(page(2))
    else older.reject(new Error('old failure'))
    await flushPromises()

    expect(wrapper.findComponent(Pagination).props('page')).toBe(1)
    expect(wrapper.text()).toContain('#1')
    expect(showError).not.toHaveBeenCalled()
  })
})
