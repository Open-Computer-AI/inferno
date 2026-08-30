/**
 * Toggle must emit from its prop, not from the DOM checkbox's own state.
 *
 * The June rewrite swapped the <button role="switch"> for a real
 * <input type="checkbox"> and read `event.target.checked` back. The DOM flips
 * itself during the click's pre-activation step, so the input -- not the parent
 * -- became the source of truth. A parent that refuses the change leaves
 * modelValue unchanged, Vue sees nothing to patch, and the input stays flipped:
 * the next click then emits the value the parent already holds and the toggle
 * appears dead.
 */
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import Toggle from '../Toggle.vue'

describe('Toggle in a controlled parent', () => {
  it('emits the negation of modelValue, not the input element state', async () => {
    const wrapper = mount(Toggle, { props: { modelValue: false } })
    await wrapper.find('input').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('stays usable when the parent refuses the change', async () => {
    // modelValue never moves -- the parent rejected it. A second click must
    // still ask for `true` rather than echoing the input's stale DOM state.
    const wrapper = mount(Toggle, { props: { modelValue: false } })
    const input = wrapper.find('input')
    await input.trigger('click')
    await input.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[true], [true]])
  })

  it('paints the track from modelValue', async () => {
    const wrapper = mount(Toggle, { props: { modelValue: true } })
    expect(wrapper.find('.tgl__track').attributes('data-checked')).toBeDefined()
  })
})
