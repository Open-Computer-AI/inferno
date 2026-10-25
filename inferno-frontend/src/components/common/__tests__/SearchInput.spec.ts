import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import SearchInput from '../SearchInput.vue'

enableAutoUnmount(afterEach)

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

const mountInput = (modelValue = '') => mount(SearchInput, {
  props: { modelValue, debounceMs: 300 },
})

describe('SearchInput', () => {
  it('waits for committed IME text before updating or searching', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')

    await input.trigger('compositionstart')
    input.element.value = 'zhong'
    await input.trigger('input')
    await vi.advanceTimersByTimeAsync(500)

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('search')).toBeUndefined()

    input.element.value = '中文'
    await input.trigger('compositionend')

    expect(wrapper.emitted('update:modelValue')).toEqual([['中文']])
    expect(wrapper.find('.srch__spinner').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toEqual([['中文']])
  })

  it('updates plain text immediately and debounces searches', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')

    input.element.value = 'a'
    await input.trigger('input')
    await vi.advanceTimersByTimeAsync(200)
    input.element.value = 'ab'
    await input.trigger('input')

    expect(wrapper.emitted('update:modelValue')).toEqual([['a'], ['ab']])
    expect(wrapper.find('.srch__spinner').exists()).toBe(true)

    await vi.advanceTimersByTimeAsync(299)
    expect(wrapper.emitted('search')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(1)

    expect(wrapper.emitted('search')).toEqual([['ab']])
    expect(wrapper.find('.srch__spinner').exists()).toBe(false)
  })

  it('clears immediately and cancels the pending search', async () => {
    const wrapper = mountInput('existing')
    const input = wrapper.get('input')

    input.element.value = 'updated'
    await input.trigger('input')
    await wrapper.get('button[aria-label="Clear search"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([['updated'], ['']])
    expect(wrapper.emitted('search')).toEqual([['']])
    expect(wrapper.find('.srch__spinner').exists()).toBe(false)

    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toEqual([['']])
  })

  it('reflects external value changes without issuing a search', async () => {
    const wrapper = mountInput()

    await wrapper.setProps({ modelValue: 'restored' })

    expect(wrapper.get('input').element.value).toBe('restored')
    await vi.advanceTimersByTimeAsync(500)
    expect(wrapper.emitted('search')).toBeUndefined()
  })
})
