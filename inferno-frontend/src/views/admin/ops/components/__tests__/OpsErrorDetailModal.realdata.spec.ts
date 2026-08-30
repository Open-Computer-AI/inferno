import { describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'

const { getRequestErrorDetail, listRequestErrorUpstreamErrors } = vi.hoisted(() => ({ getRequestErrorDetail: vi.fn(), listRequestErrorUpstreamErrors: vi.fn() }))
vi.mock('@/api/admin/ops', () => ({ opsAPI: { getRequestErrorDetail, getUpstreamErrorDetail: vi.fn(), listRequestErrorUpstreamErrors } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => {
  const a = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...a, useI18n: () => ({ t: (k: string) => k }) }
})

/**
 * Fixture copied verbatim from ops_error_logs id=53 on the internal VM
 * (oc-internal, 2026-08-30). error_message and upstream_error_message are
 * byte-identical there, which is exactly the duplicate e4f869e0c collapses;
 * error_body and upstream_error_detail are distinct JSON and must both survive.
 */
const REAL = {
  "error_message": "The image data you provided does not represent a valid image. Please check your input and try again.",
  "upstream_error_message": "The image data you provided does not represent a valid image. Please check your input and try again.",
  "error_body": "{\"error\":{\"message\":\"The image data you provided does not represent a valid image. Please check your input and try again.\",\"type\":\"invalid_request_error\"}}",
  "upstream_error_detail": "{\"error\":{\"code\":\"invalid_value\",\"message\":\"The image data you provided does not represent a valid image. Please check your input and try again.\",\"param\":\"input\",\"type\":\"invalid_request_error\"}}"
}

describe('OpsErrorDetailModal against a real production error', () => {
  it('renders each distinct payload once and leads with the upstream root cause', async () => {
    listRequestErrorUpstreamErrors.mockResolvedValue({ items: [] });
    getRequestErrorDetail.mockResolvedValue({ id: 53, status_code: 400, ...REAL })
    const wrapper = shallowMount(OpsErrorDetailModal, {
      props: { show: true, errorId: 53, errorType: 'request' },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, Icon: true } },
    })
    await flushPromises()

    // error_body, upstream_error_message and upstream_error_detail are three
    // distinct strings on this row, so all three get their own pane. Before
    // e4f869e0c the modal showed a single "response body" pane and the other
    // two payloads were simply not visible.
    expect(wrapper.findAll('pre')).toHaveLength(3)

    // root cause prefers upstream_error_message over the local message
    expect(wrapper.text()).toContain('does not represent a valid image')

    // each payload is labelled rather than dumped as one blob
    expect(wrapper.text()).toContain('admin.ops.errorDetail.payloads.client')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.payloads.upstream_message')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.payloads.upstream_detail')
  })
})
