import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DeviceApprovalView from '../DeviceApprovalView.vue'

const approveDevice = vi.fn()
const denyDevice = vi.fn()
const showError = vi.fn()

let routeQuery: Record<string, string> = {}

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // Mirrors real vue-i18n's own behavior for a missing key: return the key
    // itself, so assertions can check for the i18n key directly (the
    // convention this codebase's other specs already use, e.g.
    // OidcCallbackView.spec.ts).
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/api/oauth', () => ({
  approveDevice: (code: string) => approveDevice(code),
  denyDevice: (code: string) => denyDevice(code)
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError })
}))

const globalStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  PageHeader: { template: '<div />' }
}

describe('DeviceApprovalView', () => {
  beforeEach(() => {
    routeQuery = {}
    approveDevice.mockReset()
    denyDevice.mockReset()
    showError.mockReset()
  })

  it('prefills the code from the user_code query parameter', () => {
    routeQuery = { user_code: 'ABCD-EFGH' }
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('ABCD-EFGH')
  })

  it('uppercases a pasted code and inserts the missing hyphen', async () => {
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('wxyz2345')

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('WXYZ-2345')
  })

  it('tolerates surrounding whitespace when normalizing the prefilled code', () => {
    routeQuery = { user_code: '  abcd-efgh  ' }
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('ABCD-EFGH')
  })

  it('submits the entered code on approve', async () => {
    approveDevice.mockResolvedValue({ status: 'approved' })
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('WXYZ-2345')
    await wrapper.find('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(approveDevice).toHaveBeenCalledWith('WXYZ-2345')
    expect(denyDevice).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('device.approvedTitle')
  })

  it('submits the entered code on deny', async () => {
    denyDevice.mockResolvedValue({ status: 'denied' })
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('WXYZ-2345')
    await wrapper.find('[data-test="deny"]').trigger('click')
    await flushPromises()

    expect(denyDevice).toHaveBeenCalledWith('WXYZ-2345')
    expect(approveDevice).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('device.deniedTitle')
  })

  it('disables both actions until the code is a complete 8 character code', async () => {
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    const approveButton = wrapper.get('[data-test="approve"]')
    expect((approveButton.element as HTMLButtonElement).disabled).toBe(true)

    await wrapper.find('input').setValue('WXYZ234') // 7 characters -- still incomplete
    expect((wrapper.get('[data-test="approve"]').element as HTMLButtonElement).disabled).toBe(true)

    await wrapper.find('input').setValue('WXYZ2345') // 8 characters -- complete
    expect((wrapper.get('[data-test="approve"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('stays on the form with an inline error when the code is unknown (404)', async () => {
    approveDevice.mockRejectedValue({ status: 404, message: 'device code not found' })
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('NOPE-NOPE')
    await wrapper.find('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('device.errors.notFound')
    // Still on the form -- the input is still there to correct and resubmit.
    expect(wrapper.find('input').exists()).toBe(true)
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows a terminal expired state that says to re-run the command (410)', async () => {
    approveDevice.mockRejectedValue({ status: 410, message: 'device code expired' })
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('EXPI-RED1')
    await wrapper.find('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('device.expiredTitle')
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('surfaces an unexpected failure through the toast and an inline message', async () => {
    denyDevice.mockRejectedValue({ status: 500, message: 'boom' })
    const wrapper = mount(DeviceApprovalView, { global: { stubs: globalStubs } })

    await wrapper.find('input').setValue('SERV-ER50')
    await wrapper.find('[data-test="deny"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('boom')
    expect(wrapper.text()).toContain('boom')
  })
})
