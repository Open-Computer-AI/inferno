import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup } from '@/types'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn().mockResolvedValue({ items: [] }),
  getGroupRPMOverrides: vi.fn().mockResolvedValue([]),
  getGroupRateMultipliers: vi.fn().mockResolvedValue([])
}))

vi.mock('@/api/admin', () => ({ adminAPI: { users: mocks, groups: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe.each([
  ['RPM', GroupRPMOverridesModal],
  ['rate', GroupRateMultipliersModal]
])('%s group modal lifecycle', (_name, component) => {
  const open = () => mount(component, {
    props: {
      show: true,
      group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true,
        PlatformIcon: true,
        Pagination: true
      }
    }
  })

  it('removes its document click listener on unmount', () => {
    const add = vi.spyOn(document, 'addEventListener')
    const remove = vi.spyOn(document, 'removeEventListener')
    const wrapper = open()
    const handlers = add.mock.calls
      .filter(([event]) => event === 'click')
      .map(([, handler]) => handler)

    expect(handlers).toHaveLength(1)
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('click', handlers[0])
  })

  it('cancels a queued search when unmounted', async () => {
    const wrapper = open()
    await wrapper.get('input[type="text"]').setValue('alice')

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(mocks.list).not.toHaveBeenCalled()
  })
})
