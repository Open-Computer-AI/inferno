import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '../ImportDataModal.vue'

const { importData, showError, showSuccess } = vi.hoisted(() => ({
  importData: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { importData } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('proxy data import partial success', () => {
  beforeEach(() => {
    importData.mockReset().mockResolvedValue({ proxy_created: 1, proxy_reused: 0, proxy_failed: 1, errors: [] })
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('refreshes the proxy list when the modal closes after partial import success', async () => {
    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        },
      },
    })
    const file = { name: 'proxies.json', text: vi.fn().mockResolvedValue('{"proxies":[]}') } as unknown as File
    const input = wrapper.get('input[type="file"]').element as HTMLInputElement
    Object.defineProperty(input, 'files', { configurable: true, value: [file] })
    await wrapper.get('input[type="file"]').trigger('change')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledOnce()
    expect(showError).toHaveBeenCalledWith('admin.proxies.dataImportCompletedWithErrors')
    expect(wrapper.emitted('imported')).toBeUndefined()

    await wrapper.findAll('button.btn-secondary').at(-1)!.trigger('click')
    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
