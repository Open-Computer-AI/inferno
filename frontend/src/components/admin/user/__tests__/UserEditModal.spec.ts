import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserEditModal from '../UserEditModal.vue'

const { update, updateUserAttributeValues, showSuccess, showError } = vi.hoisted(() => ({
  update: vi.fn(),
  updateUserAttributeValues: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update },
    userAttributes: { updateUserAttributeValues }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard: vi.fn() })
}))

// useStepUp pulls in the API client, which needs the real i18n instance.
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = (concurrency: number, userOverrides: Record<string, unknown> = {}) => mount(UserEditModal, {
  props: {
    show: true,
    user: {
      id: 7,
      email: 'user@example.test',
      username: 'user',
      notes: '',
      role: 'user',
      concurrency,
      rpm_limit: 0,
      ...userOverrides
    } as never
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Select: true,
      Icon: true,
      UserAttributeForm: true,
      TotpStepUpDialog: true
    }
  }
})

describe('UserEditModal concurrency', () => {
  beforeEach(() => {
    update.mockReset()
    updateUserAttributeValues.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    update.mockResolvedValue({})
  })

  // Regression coverage for issue #5977: the gateway treats concurrency <= 0 as
  // unlimited (AcquireUserSlot) and both the batch limits endpoint and the bulk
  // edit modal accept 0, so this dialog must not be the only place that rejects
  // it — doing so blocked every other edit on such a user.
  it('saves an unlimited (0) concurrency instead of blocking the whole form', async () => {
    const wrapper = mountModal(0)

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({ concurrency: 0 }))
    expect(wrapper.emitted('success')).toBeTruthy()
  })

  it('still rejects a negative concurrency', async () => {
    const wrapper = mountModal(3)

    await wrapper.get('[data-test="concurrency-input"]').setValue('-1')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.users.concurrencyNonNegative')
    expect(update).not.toHaveBeenCalled()
  })

  it('preserves an untouched split policy by omitting concurrency_policy', async () => {
    const wrapper = mountModal(3, {
      concurrency_mode: 'split',
      live_concurrency: 4,
      request_concurrency: 2
    })

    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(update.mock.calls[0]?.[1]).not.toHaveProperty('concurrency_policy')
  })

  it('submits a changed split policy and an explicit shared reset', async () => {
    const wrapper = mountModal(3)

    await wrapper.get('[data-test="concurrency-mode"]').setValue('split')
    await wrapper.get('[data-test="live-concurrency-input"]').setValue('4')
    await wrapper.get('[data-test="request-concurrency-input"]').setValue('2')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({
      concurrency_policy: { mode: 'split', live: 4, requests: 2 }
    }))

    update.mockReset().mockResolvedValue({})
    const resetWrapper = mountModal(3, {
      concurrency_mode: 'split',
      live_concurrency: 4,
      request_concurrency: 2
    })
    await resetWrapper.get('[data-test="concurrency-mode"]').setValue('shared')
    await resetWrapper.get('form').trigger('submit')
    await flushPromises()

    expect(update).toHaveBeenCalledWith(7, expect.objectContaining({
      concurrency_policy: { mode: 'shared' }
    }))
  })

  it('blocks invalid split limits before update', async () => {
    const wrapper = mountModal(3)

    await wrapper.get('[data-test="concurrency-mode"]').setValue('split')
    await wrapper.get('[data-test="live-concurrency-input"]').setValue('0')
    await wrapper.get('[data-test="request-concurrency-input"]').setValue('2')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.concurrencyPolicy.invalid')
    expect(update).not.toHaveBeenCalled()
  })
})
