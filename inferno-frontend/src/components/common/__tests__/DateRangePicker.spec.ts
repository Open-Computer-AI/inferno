import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick, ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

// Teleported popovers outlive the wrapper, so clear them or the next test
// finds stale .date-picker-preset nodes from the previous mount.
afterEach(() => {
  document.body.innerHTML = ''
})

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('DateRangePicker', () => {
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    // The trigger stays inside the component's own tree, but the popover is
    // teleported to <body> (same mechanism Select.vue uses, so the panel is not
    // clipped by an overflow ancestor in a filter bar or modal). So the presets
    // and Apply are queried off document.body, exactly as Select.spec.ts does
    // for .select-dropdown-portal. wrapper.findAll() cannot see teleported nodes.
    await wrapper.find('.date-picker-trigger').trigger('click')

    const presetButton = Array.from(
      document.body.querySelectorAll<HTMLElement>('.date-picker-preset')
    ).find((node) => node.textContent?.includes('Last 24 Hours'))
    expect(presetButton).toBeDefined()

    presetButton!.click()
    await nextTick()

    const applyButton = document.body.querySelector<HTMLElement>('.date-picker-apply')
    expect(applyButton).toBeDefined()
    applyButton!.click()
    await nextTick()

    const nowAfterClick = new Date()
    const yesterdayAfterClick = new Date(nowAfterClick.getTime() - 24 * 60 * 60 * 1000)
    const expectedStart = formatLocalDate(yesterdayAfterClick)
    const expectedEnd = formatLocalDate(nowAfterClick)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })
})
