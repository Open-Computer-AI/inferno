import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AuthorizeConsentView from '../AuthorizeConsentView.vue'

const checkAuthorize = vi.fn()
const decideAuthorize = vi.fn()
const fetchPendingAuthorization = vi.fn()

let routeQuery: Record<string, string> = {}

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: routeQuery })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    // Mirrors real vue-i18n's own behavior for a missing key: return the key
    // itself, matching this codebase's other specs (e.g. DeviceApprovalView).
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@/api/oauth', () => ({
  checkAuthorize: (params: unknown) => checkAuthorize(params),
  decideAuthorize: (params: unknown, decision: string) => decideAuthorize(params, decision),
  fetchPendingAuthorization: (clientID: string) => fetchPendingAuthorization(clientID)
}))

const globalStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  PageHeader: { template: '<div />' }
}

const VALID_QUERY = {
  response_type: 'code',
  client_id: 'agent:abc123',
  redirect_uri: 'https://agent.example.com/auth/callback',
  scope: 'agent_dashboard:access',
  state: 'csrf-state',
  code_challenge: 'A'.repeat(43),
  code_challenge_method: 'S256'
}

const PENDING = { client_name: 'my-laptop', client_id: 'agent:abc123' }

function mountView() {
  return mount(AuthorizeConsentView, { global: { stubs: globalStubs } })
}

// jsdom's window.location.replace is not configurable, so it cannot be
// vi.spyOn'd directly -- the whole `location` object has to be replaced,
// exactly as PaymentStatusPanel.spec.ts already does for the same reason.
let originalLocation: Location
let locationReplace: ReturnType<typeof vi.fn>

describe('AuthorizeConsentView', () => {
  beforeEach(() => {
    routeQuery = { ...VALID_QUERY }
    checkAuthorize.mockReset()
    decideAuthorize.mockReset()
    fetchPendingAuthorization.mockReset()
    fetchPendingAuthorization.mockResolvedValue(PENDING)

    originalLocation = window.location
    locationReplace = vi.fn()
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { replace: locationReplace }
    })
  })

  afterEach(() => {
    Object.defineProperty(window, 'location', { configurable: true, value: originalLocation })
  })

  // The common case: the backend already decided (auto-approve, or a
  // redirect-class rejection) and this screen never needs to show anything
  // -- it just carries the browser on to the target URL.
  it('navigates straight to the target URL when the backend reports a redirect, without ever showing consent', async () => {
    checkAuthorize.mockResolvedValue({
      kind: 'redirect',
      url: 'https://agent.example.com/auth/callback?code=abc&state=csrf-state'
    })
    const wrapper = mountView()
    await flushPromises()

    expect(checkAuthorize).toHaveBeenCalledWith(VALID_QUERY)
    expect(locationReplace).toHaveBeenCalledWith(
      'https://agent.example.com/auth/callback?code=abc&state=csrf-state'
    )
    expect(fetchPendingAuthorization).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="consent"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
  })

  // RFC 6749's authorize endpoint must show who is asking and what before a
  // decision can be made -- the assertion a bare Approve/Deny pair cannot
  // pass. Mirrors DeviceApprovalView's equivalent RFC 8628 5.4 assertion.
  it('shows who is asking and what access is requested before offering a decision', async () => {
    checkAuthorize.mockResolvedValue({ kind: 'consent_required' })
    const wrapper = mountView()
    await flushPromises()

    expect(fetchPendingAuthorization).toHaveBeenCalledWith('agent:abc123')
    expect(wrapper.get('[data-test="client-name"]').text()).toBe('my-laptop')
    expect(wrapper.text()).toContain('agent:abc123')
    expect(wrapper.get('[data-test="scopes"]').text()).toContain('authorize.scopes.agentDashboardAccess')
    expect(wrapper.get('[data-test="grant-warning"]').text()).toBe('authorize.grantWarning')
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="deny"]').exists()).toBe(true)
  })

  // The phishing defence: no Approve/Deny button exists at all while the
  // request is still being checked.
  it('does not offer approve or deny while the request is still being checked', () => {
    checkAuthorize.mockReturnValue(new Promise(() => {})) // never resolves
    const wrapper = mountView()

    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="deny"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="consent"]').exists()).toBe(false)
  })

  it('submits approve and navigates to the returned target URL', async () => {
    checkAuthorize.mockResolvedValue({ kind: 'consent_required' })
    decideAuthorize.mockResolvedValue('https://agent.example.com/auth/callback?code=xyz&state=csrf-state')
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="approve"]').trigger('click')
    await flushPromises()

    expect(decideAuthorize).toHaveBeenCalledWith(VALID_QUERY, 'approve')
    expect(locationReplace).toHaveBeenCalledWith(
      'https://agent.example.com/auth/callback?code=xyz&state=csrf-state'
    )
  })

  it('submits deny and navigates to the returned target URL without approving', async () => {
    checkAuthorize.mockResolvedValue({ kind: 'consent_required' })
    decideAuthorize.mockResolvedValue('https://agent.example.com/auth/callback?error=access_denied&state=csrf-state')
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="deny"]').trigger('click')
    await flushPromises()

    expect(decideAuthorize).toHaveBeenCalledWith(VALID_QUERY, 'deny')
    expect(locationReplace).toHaveBeenCalledWith(
      'https://agent.example.com/auth/callback?error=access_denied&state=csrf-state'
    )
  })

  // An unmapped scope must fail loud, not render raw or vanish -- the backend
  // allowlist (service.ValidateScope) should make it impossible to reach
  // here, so seeing one means the two sides have drifted.
  it('warns rather than rendering an unrecognised scope', async () => {
    routeQuery = { ...VALID_QUERY, scope: 'something:new' }
    checkAuthorize.mockResolvedValue({ kind: 'consent_required' })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="scopes"]').text()).toContain('authorize.unknownScope')
    expect(wrapper.get('[data-test="scopes"]').text()).not.toContain('something:new')
  })

  it('shows a generic error state and never renders a decision when the check fails', async () => {
    checkAuthorize.mockRejectedValue({ status: 400, message: 'unknown client' })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('unknown client')
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="deny"]').exists()).toBe(false)
  })

  it('shows a generic error state when a required query parameter is missing, without calling the backend', async () => {
    routeQuery = { ...VALID_QUERY, redirect_uri: '' }
    const wrapper = mountView()
    await flushPromises()

    expect(checkAuthorize).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('authorize.errors.generic')
  })
})
