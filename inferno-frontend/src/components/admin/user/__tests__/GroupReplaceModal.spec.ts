import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup, AdminUser } from '@/types'
import GroupReplaceModal from '../GroupReplaceModal.vue'

const { replaceGroup, showError, showSuccess } = vi.hoisted(() => ({
  replaceGroup: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { replaceGroup } }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = () => mount(GroupReplaceModal, {
  props: {
    show: true,
    user: { id: 10 } as AdminUser,
    oldGroup: { id: 1, name: 'Old' },
    allGroups: [
      {
        id: 2,
        name: 'New',
        status: 'active',
        is_exclusive: true,
        subscription_type: 'standard'
      } as AdminGroup
    ]
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      },
      Icon: true
    }
  }
})

describe('GroupReplaceModal feedback', () => {
  beforeEach(() => {
    replaceGroup.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it.each([
    [{ message: 'Group is unavailable' }, 'Group is unavailable'],
    [{ response: { data: { detail: 'Permission denied' } } }, 'Permission denied'],
    [{}, 'common.error']
  ])('shows API errors and keeps the dialog available for retry', async (error, message) => {
    replaceGroup.mockRejectedValueOnce(error)
    const wrapper = mountModal()

    await wrapper.get('input[type="radio"]').setValue()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(message)
    expect(wrapper.emitted('success')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeUndefined()
  })

  it('still reports successful migrations and closes', async () => {
    replaceGroup.mockResolvedValueOnce({ migrated_keys: 3 })
    const wrapper = mountModal()

    await wrapper.get('input[type="radio"]').setValue()
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()

    expect(replaceGroup).toHaveBeenCalledWith(10, 1, 2)
    expect(showSuccess).toHaveBeenCalledWith('admin.users.replaceGroupSuccess:{"count":3}')
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
