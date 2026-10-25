import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import UserAttributeForm from '../UserAttributeForm.vue'

const mocks = vi.hoisted(() => ({ listEnabledDefinitions: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { userAttributes: mocks } }))
enableAutoUnmount(afterEach)

beforeEach(() => {
  mocks.listEnabledDefinitions.mockResolvedValue([
    { id: 1, name: 'Count', type: 'number', validation: { min: -10, max: 100 } }
  ])
})

const mountForm = async () => {
  const wrapper = mount(UserAttributeForm, {
    props: { modelValue: {} },
    global: { stubs: { Select: true, Checkbox: true } }
  })
  await flushPromises()
  return wrapper
}

describe('UserAttributeForm numeric values', () => {
  it.each(['42', '0', '-3'])('emits %s as a string', async (value) => {
    const wrapper = await mountForm()
    await wrapper.get('input[type="number"]').setValue(value)

    expect(wrapper.emitted('update:modelValue')!.at(-1)).toEqual([{ 1: value }])
    expect(typeof wrapper.emitted('update:modelValue')!.at(-1)![0][1]).toBe('string')
  })

  it('emits an empty string when the number is cleared', async () => {
    const wrapper = await mountForm()
    const input = wrapper.get('input[type="number"]')

    await input.setValue('42')
    await input.setValue('')

    expect(wrapper.emitted('update:modelValue')!.at(-1)).toEqual([{ 1: '' }])
    expect(input.attributes()).toMatchObject({ min: '-10', max: '100', type: 'number' })
  })
})
