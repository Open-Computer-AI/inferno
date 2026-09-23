import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserApiKeysModal from '../UserApiKeysModal.vue'
import type { AdminUser } from '@/types'

const { getKeys, getGroups } = vi.hoisted(() => ({ getKeys: vi.fn(), getGroups: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { getUserApiKeys: getKeys },
    groups: { getAll: getGroups },
    apiKeys: { updateApiKeyGroup: vi.fn() },
  },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  getKeys.mockReset()
  getGroups.mockReset().mockResolvedValue([])
  vi.spyOn(console, 'error').mockImplementation(() => {})
})

afterEach(() => vi.restoreAllMocks())

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const user = (id: number) => ({ id, email: `user${id}@example.com`, username: `user${id}` }) as AdminUser
const keys = (id: number, name: string) => ({
  items: [{ id, name, key: 'sk-example-key-value-for-tests', status: 'active', created_at: '2026-09-20', group_id: null }],
})

function mountModal() {
  return mount(UserApiKeysModal, {
    props: { show: false, user: user(1) },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        GroupBadge: true,
        GroupOptionItem: true,
      },
    },
  })
}

async function open() {
  const wrapper = mountModal()
  await wrapper.setProps({ show: true })
  return wrapper
}

async function switchUser(wrapper: Awaited<ReturnType<typeof open>>) {
  await wrapper.setProps({ show: false })
  await wrapper.setProps({ show: true, user: user(2) })
}

describe('UserApiKeysModal request scoping', () => {
  it('clears old user keys when the next user request fails', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = await open()
    await flushPromises()
    expect(wrapper.text()).toContain('first-user-key')

    await switchUser(wrapper)
    await flushPromises()
    expect(wrapper.text()).toContain('user2@example.com')
    expect(wrapper.text()).not.toContain('first-user-key')
  })

  it('does not replace current keys with a late previous-user response', async () => {
    const oldRequest = deferred<ReturnType<typeof keys>>()
    getKeys.mockReturnValueOnce(oldRequest.promise).mockResolvedValueOnce(keys(2, 'current-user-key'))
    const wrapper = await open()
    await switchUser(wrapper)
    await flushPromises()

    oldRequest.resolve(keys(1, 'old-user-key'))
    await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.text()).not.toContain('old-user-key')
  })

  it('keeps the current loading state when an obsolete request fails', async () => {
    const oldRequest = deferred<ReturnType<typeof keys>>()
    const currentRequest = deferred<ReturnType<typeof keys>>()
    getKeys.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(currentRequest.promise)
    const wrapper = await open()
    await switchUser(wrapper)

    oldRequest.reject(new Error('obsolete request'))
    await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)

    currentRequest.resolve(keys(2, 'current-user-key'))
    await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
  })

  it('reloads keys when the selected user changes while the dialog stays open', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockResolvedValueOnce(keys(2, 'second-user-key'))
    const wrapper = await open()
    await flushPromises()

    await wrapper.setProps({ user: user(2) })
    await flushPromises()
    expect(getKeys).toHaveBeenLastCalledWith(2)
    expect(wrapper.text()).toContain('second-user-key')
    expect(wrapper.text()).not.toContain('first-user-key')
  })
})
