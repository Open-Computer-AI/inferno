import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { AdminGroup } from '@/types'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), getGroupRateMultipliers: vi.fn(), batchSetGroupRateMultipliers: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { list: mocks.list }, groups: mocks } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => vi.useRealTimers())
beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers()
  mocks.getGroupRateMultipliers.mockResolvedValue([])
  mocks.list.mockResolvedValue({ items: [{ id: 7, email: 'user@example.com', status: 'active' }] })
  mocks.batchSetGroupRateMultipliers.mockResolvedValue(undefined)
})

async function selectUser() {
  const wrapper = mount(GroupRateMultipliersModal, {
    props: { show: false, group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      Icon: true, PlatformIcon: true, Pagination: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  await wrapper.get('input[type="text"]').setValue('user')
  await vi.advanceTimersByTimeAsync(300)
  await flushPromises()
  await wrapper.findAll('button').find(b => b.text().includes('user@example.com'))!.trigger('click')
  return wrapper
}

async function mountWithExistingRates(rates: Array<{ user_id: number; rate_multiplier: number }>) {
  mocks.getGroupRateMultipliers.mockResolvedValue(rates.map(({ user_id, rate_multiplier }) => ({
    user_id,
    user_name: `User ${user_id}`,
    user_email: `user${user_id}@example.com`,
    user_notes: '',
    user_status: 'active',
    rate_multiplier,
    rpm_override: null
  })))
  const wrapper = mount(GroupRateMultipliersModal, {
    props: { show: false, group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      Icon: true, PlatformIcon: true, Pagination: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('GroupRateMultipliersModal new override validation', () => {
  it.each(['', '0', '-1'])('does not add invalid rate %j', async (value) => {
    const wrapper = await selectUser()
    const input = wrapper.get('input[placeholder="1.0"]')
    await input.setValue('100')
    await input.setValue(value)
    const add = wrapper.findAll('button').find(b => b.text() === 'common.add')!
    expect(add.attributes('disabled')).toBeDefined()
    await add.trigger('click')
    expect(wrapper.find('tbody tr').exists()).toBe(false)
    expect(mocks.batchSetGroupRateMultipliers).not.toHaveBeenCalled()
  })

  it.each([0.25, 1])('saves positive rate %s', async (value) => {
    const wrapper = await selectUser()
    await wrapper.get('input[placeholder="1.0"]').setValue(String(value))
    await wrapper.findAll('button').find(b => b.text() === 'common.add')!.trigger('click')
    await wrapper.findAll('button').find(b => b.text() === 'common.save')!.trigger('click')
    await flushPromises()
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [{ user_id: 7, rate_multiplier: value }])
  })

  it.each([
    ['NaN', Number.NaN],
    ['Infinity', Number.POSITIVE_INFINITY],
    ['-Infinity', Number.NEGATIVE_INFINITY],
  ])('rejects non-finite rate %s even if supplied programmatically', async (_label, value) => {
    const wrapper = await selectUser()
    const vm = wrapper.vm as unknown as { newRate: number | null; handleAddLocal: () => void }
    vm.newRate = value
    await nextTick()
    const add = wrapper.findAll('button').find(b => b.text() === 'common.add')!
    expect(add.attributes('disabled')).toBeDefined()
    vm.handleAddLocal()
    expect(wrapper.find('tbody tr').exists()).toBe(false)
    expect(mocks.batchSetGroupRateMultipliers).not.toHaveBeenCalled()
  })
})

describe('GroupRateMultipliersModal existing override validation', () => {
  it.each(['1e999', 'Infinity'])('ignores non-finite existing rate %s', async (value) => {
    const wrapper = await mountWithExistingRates([{ user_id: 7, rate_multiplier: 2 }])
    const vm = wrapper.vm as unknown as {
      localEntries: Array<{ rate_multiplier: number | null }>
      updateLocalRate: (userId: number, value: string) => void
      handleSave: () => Promise<void>
    }

    vm.updateLocalRate(7, '3')
    vm.updateLocalRate(7, value)

    expect(vm.localEntries[0].rate_multiplier).toBe(3)
    await vm.handleSave()
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [{ user_id: 7, rate_multiplier: 3 }])
  })

  it('rejects non-finite batch factors and disables their apply button', async () => {
    const wrapper = await mountWithExistingRates([{ user_id: 7, rate_multiplier: 2 }])
    const vm = wrapper.vm as unknown as {
      batchFactor: number | null
      localEntries: Array<{ rate_multiplier: number | null }>
      applyBatchFactor: () => void
    }
    vm.batchFactor = Number.POSITIVE_INFINITY
    await nextTick()

    const apply = wrapper.findAll('button').find(b => b.text() === 'admin.groups.applyMultiplier')!
    expect(apply.attributes('disabled')).toBeDefined()
    vm.applyBatchFactor()

    expect(vm.localEntries[0].rate_multiplier).toBe(2)
    expect(vm.batchFactor).toBe(Number.POSITIVE_INFINITY)
  })

  it('does not partially apply a finite batch factor that overflows a row', async () => {
    const wrapper = await mountWithExistingRates([
      { user_id: 7, rate_multiplier: 2 },
      { user_id: 8, rate_multiplier: Number.MAX_VALUE }
    ])
    const vm = wrapper.vm as unknown as {
      batchFactor: number | null
      localEntries: Array<{ rate_multiplier: number | null }>
      applyBatchFactor: () => void
    }
    vm.batchFactor = 10

    vm.applyBatchFactor()

    expect(vm.localEntries.map(entry => entry.rate_multiplier)).toEqual([2, Number.MAX_VALUE])
    expect(vm.batchFactor).toBe(10)
  })
})
