import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

import OpsSettingsDialog from '../OpsSettingsDialog.vue'
const { opsAPI, showError } = vi.hoisted(() => ({
  opsAPI: {
    getAlertRuntimeSettings: vi.fn(), getEmailNotificationConfig: vi.fn(),
    getAdvancedSettings: vi.fn(), getMetricThresholds: vi.fn(),
    updateAlertRuntimeSettings: vi.fn(), updateEmailNotificationConfig: vi.fn(),
    updateAdvancedSettings: vi.fn(), updateMetricThresholds: vi.fn(),
  }, showError: vi.fn(),
}))
vi.mock('@/api/admin/ops', () => ({ opsAPI }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))
async function openDialog() {
  const wrapper = shallowMount(OpsSettingsDialog, {
    props: { show: false },
    global: { stubs: { BaseDialog: { template: '<div><slot name="footer" /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  return wrapper
}
type View = { saveAllSettings: () => Promise<void> }
describe('ops settings load guard', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    opsAPI.getAlertRuntimeSettings.mockResolvedValue({ evaluation_interval_seconds: 30, distributed_lock: { enabled: true, key: 'ops:lock', ttl_seconds: 30 }, silencing: { enabled: false, entries: [] } })
    opsAPI.getEmailNotificationConfig.mockResolvedValue({ alert: { enabled: false, recipients: [] }, report: { enabled: false, recipients: [] } })
    opsAPI.getAdvancedSettings.mockResolvedValue({ data_retention: { error_log_retention_days: 7, minute_metrics_retention_days: 7, hourly_metrics_retention_days: 7 }, openai_account_quota_auto_pause: { default_threshold_5h: 0, default_threshold_7d: 0 } })
    opsAPI.getMetricThresholds.mockResolvedValue({ sla_percent_min: 98 })
  })
  afterEach(() => { vi.restoreAllMocks() })
  it('blocks saving while any settings request is still loading', async () => {
    const pending = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    await (wrapper.vm as unknown as View).saveAllSettings()
    expect(opsAPI.updateMetricThresholds).not.toHaveBeenCalled()
    pending.resolve({ sla_percent_min: 98 })
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(opsAPI.updateMetricThresholds).toHaveBeenCalledWith(expect.objectContaining({ sla_percent_min: 98 }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('does not save default thresholds after a failed initial load', async () => {
    opsAPI.getMetricThresholds.mockRejectedValueOnce(new Error('load failed'))
    const wrapper = await openDialog()
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    await (wrapper.vm as unknown as View).saveAllSettings()
    expect(opsAPI.updateMetricThresholds).not.toHaveBeenCalled()
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
  it('blocks saving stale settings after a failed reopen and allows a later retry', async () => {
    const wrapper = await openDialog()
    await flushPromises()
    await wrapper.setProps({ show: false })
    opsAPI.getMetricThresholds.mockRejectedValueOnce(new Error('refresh failed'))
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    await (wrapper.vm as unknown as View).saveAllSettings()
    expect(opsAPI.updateAlertRuntimeSettings).not.toHaveBeenCalled()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
  })
  it('does not apply old success while the reopened load is pending', async () => {
    const old = deferred<object>()
    const current = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    opsAPI.getAlertRuntimeSettings.mockResolvedValueOnce({ evaluation_interval_seconds: 10 }).mockResolvedValueOnce({ evaluation_interval_seconds: 20 })
    const wrapper = await openDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    old.resolve({ sla_percent_min: 90 })
    await flushPromises()
    const view = wrapper.vm as unknown as { settingsLoaded: boolean; runtimeSettings: { evaluation_interval_seconds: number } | null }
    expect(view.settingsLoaded).toBe(false)
    expect(view.runtimeSettings).toBeNull()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    current.resolve({ sla_percent_min: 98 })
    await flushPromises()
    expect(view.settingsLoaded).toBe(true)
    expect(view.runtimeSettings?.evaluation_interval_seconds).toBe(20)
  })

  it.each([
    ['hidden', async (wrapper: ReturnType<typeof shallowMount>) => { await wrapper.setProps({ show: false }) }],
    ['unmounted', async (wrapper: ReturnType<typeof shallowMount>) => { wrapper.unmount() }],
  ])('ignores a pending load after the dialog is %s', async (_state, stop) => {
    const pending = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    await stop(wrapper)
    pending.resolve({ sla_percent_min: 90 })
    await flushPromises()
    const view = wrapper.vm as unknown as { settingsLoaded: boolean; runtimeSettings: object | null }
    expect(view.settingsLoaded).toBe(false)
    expect(view.runtimeSettings).toBeNull()
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores a pending load after unmounting', async () => {
    const pending = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(pending.promise)
    const wrapper = await openDialog()
    wrapper.unmount()
    pending.resolve({ sla_percent_min: 90 })
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores old failure after the reopened load succeeds', async () => {
    const old = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(old.promise).mockResolvedValueOnce({ sla_percent_min: 98 })
    opsAPI.getAlertRuntimeSettings.mockResolvedValueOnce({ evaluation_interval_seconds: 10 }).mockResolvedValueOnce({ evaluation_interval_seconds: 20 })
    const wrapper = await openDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    old.reject(new Error('old load failed'))
    await flushPromises()
    const view = wrapper.vm as unknown as { settingsLoaded: boolean; runtimeSettings: { evaluation_interval_seconds: number } | null }
    expect(view.settingsLoaded).toBe(true)
    expect(view.runtimeSettings?.evaluation_interval_seconds).toBe(20)
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores old success after the reopened load fails', async () => {
    const old = deferred<object>()
    opsAPI.getMetricThresholds.mockReturnValueOnce(old.promise).mockRejectedValueOnce(new Error('current load failed'))
    opsAPI.getAlertRuntimeSettings.mockResolvedValueOnce({ evaluation_interval_seconds: 10 }).mockResolvedValueOnce({ evaluation_interval_seconds: 20 })
    const wrapper = await openDialog()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(showError).toHaveBeenCalledTimes(1)
    old.resolve({ sla_percent_min: 90 })
    await flushPromises()
    const view = wrapper.vm as unknown as { settingsLoaded: boolean; runtimeSettings: { evaluation_interval_seconds: number } | null }
    expect(view.settingsLoaded).toBe(false)
    expect(view.runtimeSettings).toBeNull()
    expect(showError).toHaveBeenCalledTimes(1)
  })

})
