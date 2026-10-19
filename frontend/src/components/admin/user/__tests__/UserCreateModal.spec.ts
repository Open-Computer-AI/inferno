import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const { create, showError, run } = vi.hoisted(() => ({
  create: vi.fn(),
  showError: vi.fn(),
  run: vi.fn((action: () => unknown) => action())
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { create }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn() })
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => null
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import UserCreateModal from '../UserCreateModal.vue'

const mountModal = () => mount(UserCreateModal, {
  props: { show: true },
  global: {
    stubs: {
      BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
      Icon: true,
      TotpStepUpDialog: true
    }
  }
})

describe('UserCreateModal concurrency policy', () => {
  beforeEach(() => {
    create.mockReset().mockResolvedValue({})
    showError.mockReset()
    run.mockClear()
  })

  it('submits one complete split policy object', async () => {
    const wrapper = mountModal()

    await wrapper.get('input[type="email"]').setValue('user@example.test')
    await wrapper.get('input[type="text"][required]').setValue('password')
    await wrapper.get('[data-test="concurrency-mode"]').setValue('split')
    await wrapper.get('[data-test="live-concurrency-input"]').setValue('4')
    await wrapper.get('[data-test="request-concurrency-input"]').setValue('2')
    await wrapper.get('#create-user-form').trigger('submit')
    await flushPromises()

    expect(create).toHaveBeenCalledWith(expect.objectContaining({
      concurrency_policy: { mode: 'split', live: 4, requests: 2 }
    }))
  })

  it('prevents an incomplete split policy from reaching the API', async () => {
    const wrapper = mountModal()

    await wrapper.get('input[type="email"]').setValue('user@example.test')
    await wrapper.get('input[type="text"][required]').setValue('password')
    await wrapper.get('[data-test="concurrency-mode"]').setValue('split')
    await wrapper.get('[data-test="live-concurrency-input"]').setValue('0')
    await wrapper.get('[data-test="request-concurrency-input"]').setValue('2')
    await wrapper.get('#create-user-form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.concurrencyPolicy.invalid')
    expect(create).not.toHaveBeenCalled()
  })
})
