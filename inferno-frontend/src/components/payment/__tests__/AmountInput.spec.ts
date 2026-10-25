import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import AmountInput from '../AmountInput.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }) }))
enableAutoUnmount(afterEach)

function mountInput(value: number | null = null, amounts = [10, 20, 50]) {
  return mount(AmountInput, { props: { modelValue: value, amounts } })
}

describe('recharge amount input', () => {
  it.each(['10abc', '10.555', '-10', '1e2'])('restores the accepted amount after rejecting %s', async (value) => {
    const wrapper = mountInput(12, [10])
    const input = wrapper.get('.amt__input')
    await input.setValue(value)
    expect((input.element as HTMLInputElement).value).toBe('12')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('restores the last typed amount rather than a stale prop', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('.amt__input')
    await input.setValue('12.50')
    await input.setValue('12.500')
    expect((input.element as HTMLInputElement).value).toBe('12.50')
    expect(wrapper.emitted('update:modelValue')).toEqual([[12.5]])
  })

  it('shows a bonus badge and credited preview for qualifying preset amounts', () => {
    const wrapper = mount(AmountInput, {
      props: {
        modelValue: null,
        amounts: [10, 20],
        bonusTiers: [{ min_amount: 20, bonus_percent: 25 }],
        bonusMode: 'bonus',
        multiplier: 1.5,
        currency: 'USD',
      },
    })
    const buttons = wrapper.findAll('.amt__chip')
    expect(buttons[0]?.find('[data-testid="quick-amount-credited"]').text()).toBe('payment.rechargeBonus.creditedShort')
    expect(buttons[1]?.find('[data-testid="quick-amount-bonus-badge"]').text()).toBe('+25%')
  })

  it('shows discounted pay preview for discount-mode preset amounts', () => {
    const wrapper = mount(AmountInput, {
      props: {
        modelValue: null,
        amounts: [20],
        bonusTiers: [{ min_amount: 20, bonus_percent: 25 }],
        bonusMode: 'discount',
        currency: 'USD',
      },
    })
    expect(wrapper.get('[data-testid="quick-amount-bonus-badge"]').text()).toBe('25% OFF')
    expect(wrapper.get('[data-testid="quick-amount-credited"]').text()).toBe('payment.rechargeBonus.payShort')
  })

  it('preserves decimal editing and allows clearing the amount', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('.amt__input')
    for (const value of ['0', '0.', '0.5', '0.50', '']) await input.setValue(value)
    expect(wrapper.emitted('update:modelValue')).toEqual([[null], [null], [0.5], [0.5], [null]])
    expect((input.element as HTMLInputElement).value).toBe('')
  })
})
