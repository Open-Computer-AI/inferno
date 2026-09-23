import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import DateRangePicker from '../DateRangePicker.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } })
}))

let wrapper: ReturnType<typeof mount> | undefined

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date(2026, 8, 30, 23, 59))
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  vi.useRealTimers()
})

async function reopenNextDay() {
  wrapper = mount(DateRangePicker, {
    attachTo: document.body,
    props: { startDate: '2026-09-30', endDate: '2026-09-30' }
  })
  await wrapper.get('.dr-trigger').trigger('click')
  await wrapper.get('.dr-trigger').trigger('click')
  vi.setSystemTime(new Date(2026, 9, 1, 0, 1))
  await wrapper.get('.dr-trigger').trigger('click')
  await nextTick()
  return wrapper
}

describe('DateRangePicker after midnight', () => {
  it.each([
    ['dates.today', '2026-10-01'],
    ['dates.last7Days', '2026-09-25'],
    ['dates.thisMonth', '2026-10-01']
  ])('refreshes %s when the page stays mounted overnight', async (label, startDate) => {
    const picker = await reopenNextDay()
    const preset = Array.from(document.body.querySelectorAll<HTMLElement>('.dr-pop__preset'))
      .find(button => button.textContent?.trim() === label)
    expect(preset).toBeDefined()
    await preset!.click()
    await nextTick()
    await document.body.querySelector<HTMLElement>('.dr-pop__apply')!.click()
    await nextTick()

    expect(picker.emitted('change')?.[0]?.[0]).toMatchObject({
      startDate,
      endDate: '2026-10-01'
    })
  })

  it('refreshes the maximum accepted date when reopened', async () => {
    wrapper = mount(DateRangePicker, {
      attachTo: document.body,
      props: { startDate: '2026-09-30', endDate: '2026-09-30' }
    })
    await wrapper.get('.dr-trigger').trigger('click')
    const endInput = document.body.querySelectorAll<HTMLInputElement>('.dr-pop__field-input')[1]
    endInput.value = '2026-10-02'
    endInput.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()
    document.body.querySelector<HTMLElement>('.dr-pop__apply')!.click()
    await nextTick()
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual(['2026-09-30'])

    vi.setSystemTime(new Date(2026, 9, 1, 0, 1))
    await wrapper.get('.dr-trigger').trigger('click')
    const reopenedEndInput = document.body.querySelectorAll<HTMLInputElement>('.dr-pop__field-input')[1]
    reopenedEndInput.value = '2026-10-02'
    reopenedEndInput.dispatchEvent(new Event('change', { bubbles: true }))
    await nextTick()

    expect(reopenedEndInput.value).toBe('2026-10-02')
  })
})
