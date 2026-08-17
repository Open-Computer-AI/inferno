import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DeviceApprovalView from '../DeviceApprovalView.vue'

const approveDevice = vi.fn()
const denyDevice = vi.fn()
const fetchPendingDeviceAuthorization = vi.fn()
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
  denyDevice: (code: string) => denyDevice(code),
  fetchPendingDeviceAuthorization: (code: string) => fetchPendingDeviceAuthorization(code)
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError })
}))

const globalStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  PageHeader: { template: '<div />' }
}

const PENDING = {
  client_name: 'my-laptop',
  client_id: 'agent:abc123',
  scopes: ['inference:invoke', 'billing:manage'],
  expires_at: '2026-08-18T12:00:00Z'
}

function mountView() {
  return mount(DeviceApprovalView, { global: { stubs: globalStubs } })
}

/**
 * Drives the two-step flow to the point where Approve is offered: type the
 * code, click Continue, let the lookup resolve.
 */
async function reachConsent(wrapper: ReturnType<typeof mountView>, code = 'WXYZ-2345') {
  await wrapper.find('input').setValue(code)
  await wrapper.get('[data-test="check"]').trigger('click')
  await flushPromises()
  return wrapper
}

describe('DeviceApprovalView', () => {
  beforeEach(() => {
    routeQuery = {}
    approveDevice.mockReset()
    denyDevice.mockReset()
    fetchPendingDeviceAuthorization.mockReset()
    fetchPendingDeviceAuthorization.mockResolvedValue(PENDING)
    showError.mockReset()
  })

  it('prefills the code from the user_code query parameter', () => {
    routeQuery = { user_code: 'ABCD-EFGH' }
    const wrapper = mountView()

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('ABCD-EFGH')
  })

  it('uppercases a pasted code and inserts the missing hyphen', async () => {
    const wrapper = mountView()

    await wrapper.find('input').setValue('wxyz2345')

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('WXYZ-2345')
  })

  it('tolerates surrounding whitespace when normalizing the prefilled code', () => {
    routeQuery = { user_code: '  abcd-efgh  ' }
    const wrapper = mountView()

    expect((wrapper.find('input').element as HTMLInputElement).value).toBe('ABCD-EFGH')
  })

  // RFC 8628 section 5.4: the authorization server must show client and
  // authorization information at the verification URI. This is the assertion
  // that a bare code box cannot pass.
  it('shows who is asking, what they want, and what approval grants', async () => {
    routeQuery = { user_code: 'ABCD-EFGH' }
    const wrapper = mountView()
    await flushPromises()

    expect(fetchPendingDeviceAuthorization).toHaveBeenCalledWith('ABCD-EFGH')
    expect(wrapper.get('[data-test="client-name"]').text()).toBe('my-laptop')
    expect(wrapper.text()).toContain('agent:abc123')
    expect(wrapper.get('[data-test="scopes"]').text()).toContain('device.scopes.inferenceInvoke')
    expect(wrapper.get('[data-test="scopes"]').text()).toContain('device.scopes.billingManage')
    expect(wrapper.get('[data-test="grant-warning"]').text()).toBe('device.grantWarning')
  })

  // The phishing defence: no Approve button exists until the person has been
  // shown the requester and the scopes.
  it('does not offer approve until the authorization has been shown', async () => {
    const wrapper = mountView()

    await wrapper.find('input').setValue('WXYZ-2345')
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="consent"]').exists()).toBe(false)

    await reachConsent(wrapper)

    expect(wrapper.find('[data-test="consent"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(true)
  })

  // Reviewing one authorization and approving a different one must be
  // impossible.
  it('withdraws the approve button when the code is edited after review', async () => {
    const wrapper = await reachConsent(mountView())
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(true)

    await wrapper.find('input').setValue('WXYZ-2346')

    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="consent"]').exists()).toBe(false)
  })

  it('submits the entered code on approve', async () => {
    approveDevice.mockResolvedValue({ status: 'approved' })
    const wrapper = await reachConsent(mountView())

    await wrapper.get('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(approveDevice).toHaveBeenCalledWith('WXYZ-2345')
    expect(denyDevice).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('device.approvedTitle')
  })

  // Deferred item 20: an approval must not render in the same amber caution
  // card as "that link has already been used".
  it('renders the approved state with the success tone, not attention', async () => {
    approveDevice.mockResolvedValue({ status: 'approved' })
    const wrapper = await reachConsent(mountView())

    await wrapper.get('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-tone="success"]').exists()).toBe(true)
    expect(wrapper.find('[data-tone="attention"]').exists()).toBe(false)
  })

  // Denying is always safe, so it never requires the lookup first -- a person
  // who does not recognise the request should be able to refuse immediately.
  it('submits the entered code on deny without requiring the lookup', async () => {
    denyDevice.mockResolvedValue({ status: 'denied' })
    const wrapper = mountView()

    await wrapper.find('input').setValue('WXYZ-2345')
    await wrapper.get('[data-test="deny"]').trigger('click')
    await flushPromises()

    expect(denyDevice).toHaveBeenCalledWith('WXYZ-2345')
    expect(approveDevice).not.toHaveBeenCalled()
    expect(fetchPendingDeviceAuthorization).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('device.deniedTitle')
  })

  it('disables both actions until the code is a complete 8 character code', async () => {
    const wrapper = mountView()

    expect((wrapper.get('[data-test="check"]').element as HTMLButtonElement).disabled).toBe(true)
    expect((wrapper.get('[data-test="deny"]').element as HTMLButtonElement).disabled).toBe(true)

    await wrapper.find('input').setValue('WXYZ234') // 7 characters -- still incomplete
    expect((wrapper.get('[data-test="check"]').element as HTMLButtonElement).disabled).toBe(true)

    await wrapper.find('input').setValue('WXYZ2345') // 8 characters -- complete
    expect((wrapper.get('[data-test="check"]').element as HTMLButtonElement).disabled).toBe(false)
    expect((wrapper.get('[data-test="deny"]').element as HTMLButtonElement).disabled).toBe(false)
  })

  it('stays on the form with an inline error when the code is unknown (404)', async () => {
    fetchPendingDeviceAuthorization.mockRejectedValue({
      status: 404,
      message: 'device code not found'
    })
    const wrapper = mountView()

    await wrapper.find('input').setValue('NOPE-NOPE')
    await wrapper.get('[data-test="check"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('device.errors.notFound')
    // Still on the form -- the input is still there to correct and resubmit.
    expect(wrapper.find('input').exists()).toBe(true)
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows a terminal expired state that says to re-run the command (410)', async () => {
    fetchPendingDeviceAuthorization.mockRejectedValue({
      status: 410,
      message: 'device code expired'
    })
    const wrapper = mountView()

    await wrapper.find('input').setValue('EXPI-RED1')
    await wrapper.get('[data-test="check"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('device.expiredTitle')
    expect(wrapper.find('input').exists()).toBe(false)
  })

  // A code that already carries a decision is its own terminal state: calling
  // it "expired" would be a lie, and if the person did not make that decision
  // they need to know.
  it('shows a distinct already-decided state, not expired (409)', async () => {
    fetchPendingDeviceAuthorization.mockRejectedValue({
      status: 409,
      message: 'device code already decided'
    })
    const wrapper = mountView()

    await wrapper.find('input').setValue('USED-ONCE')
    await wrapper.get('[data-test="check"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('device.decidedTitle')
    expect(wrapper.text()).not.toContain('device.expiredTitle')
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('surfaces an unexpected failure through the toast and an inline message', async () => {
    denyDevice.mockRejectedValue({ status: 500, message: 'boom' })
    const wrapper = mountView()

    await wrapper.find('input').setValue('SERV-ER50')
    await wrapper.get('[data-test="deny"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('boom')
    expect(wrapper.text()).toContain('boom')
  })

  // An unmapped scope must fail loud rather than render raw or vanish: the
  // backend allowlist should make it impossible, so seeing one means the two
  // sides have drifted and the person cannot know what they are granting.
  it('warns rather than rendering an unrecognised scope', async () => {
    fetchPendingDeviceAuthorization.mockResolvedValue({
      ...PENDING,
      scopes: ['something:new']
    })
    const wrapper = await reachConsent(mountView())

    expect(wrapper.get('[data-test="scopes"]').text()).toContain('device.unknownScope')
    expect(wrapper.get('[data-test="scopes"]').text()).not.toContain('something:new')
  })
})
