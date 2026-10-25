import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import RechargeBonusTierEditor from '../RechargeBonusTierEditor.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

describe('RechargeBonusTierEditor', () => {
  it('adds tiers and emits mode and notice edits', async () => {
    const wrapper = mount(RechargeBonusTierEditor, {
      props: { modelValue: [{ min_amount: 100, bonus_percent: 20 }], mode: 'bonus', notice: '' },
      global: { mocks: { $t: (key: string) => key } },
    })
    await wrapper.get('[data-testid="recharge-bonus-tier-add"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([
      { min_amount: 100, bonus_percent: 20 },
      { min_amount: null, bonus_percent: null },
    ])
    await wrapper.get('[data-testid="recharge-bonus-mode-discount"]').trigger('click')
    expect(wrapper.emitted('update:mode')?.[0]?.[0]).toBe('discount')
    await wrapper.get('[data-testid="recharge-bonus-notice-input"]').setValue('Limited offer')
    expect(wrapper.emitted('update:notice')?.[0]?.[0]).toBe('Limited offer')
  })

  it('shows validation when a discount tier is 100 percent', () => {
    const wrapper = mount(RechargeBonusTierEditor, {
      props: { modelValue: [{ min_amount: 100, bonus_percent: 100 }], mode: 'discount', notice: '' },
      global: { mocks: { $t: (key: string) => key } },
    })
    expect(wrapper.get('[data-testid="recharge-bonus-tier-error"]').text()).toContain('invalidDiscountPercent')
  })
})
