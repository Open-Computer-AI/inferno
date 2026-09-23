import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ModelTagInput from '../ModelTagInput.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

function pressTab(input: Element, shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true })
  input.dispatchEvent(event)
  return event
}

function pressKey(input: Element, key: string, isComposing = false) {
  const event = new KeyboardEvent('keydown', { key, isComposing, bubbles: true, cancelable: true })
  input.dispatchEvent(event)
  return event
}

describe('model tag keyboard navigation', () => {
  it.each([false, true])('allows leaving an empty input with Tab (shift: %s)', (shift) => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    expect(pressTab(wrapper.get('.mti__input').element, shift).defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('allows leaving a whitespace-only input', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: [] } })
    await wrapper.get('.mti__input').setValue('   ')
    expect(pressTab(wrapper.get('.mti__input').element).defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('commits a pending model with Tab, then allows the next Tab to leave', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    const input = wrapper.get('.mti__input')
    await input.setValue('gpt-4.1')
    expect(pressTab(input.element).defaultPrevented).toBe(true)
    await nextTick()
    expect(wrapper.emitted('update:models')).toEqual([[['gpt-4o', 'gpt-4.1']]])
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(pressTab(input.element).defaultPrevented).toBe(false)
  })

  it('does not commit or remove tags during IME composition', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    const input = wrapper.get('.mti__input')
    await input.setValue('候')

    expect(pressKey(input.element, 'Enter', true).defaultPrevented).toBe(false)
    expect(pressKey(input.element, 'Tab', true).defaultPrevented).toBe(false)
    expect(pressKey(input.element, 'Backspace', true).defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:models')).toBeUndefined()
  })

  it('removes the last tag with Backspace only when the draft is empty', async () => {
    const wrapper = mount(ModelTagInput, { props: { models: ['gpt-4o'] } })
    const input = wrapper.get('.mti__input')
    expect(pressKey(input.element, 'Backspace').defaultPrevented).toBe(false)
    await nextTick()
    expect(wrapper.emitted('update:models')).toEqual([[[]]])
  })
})
