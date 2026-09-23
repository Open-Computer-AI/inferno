import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ChannelStatusV1View from '../ChannelStatusV1View.vue'

const { list } = vi.hoisted(() => ({ list: vi.fn() }))

vi.mock('@/api/channelMonitor', () => ({ list, status: vi.fn() }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_enabled: true },
    showError: vi.fn()
  })
}))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const mountView = () => shallowMount(ChannelStatusV1View, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      MonitorHero: {
        props: ['autoRefresh'],
        emits: ['refresh'],
        template: `<div>
          <button class="interval" @click="autoRefresh.setInterval(120)">120 seconds</button>
          <button class="refresh" @click="$emit('refresh')">Refresh</button>
          <button class="disable" @click="autoRefresh.setEnabled(false)">Disable</button>
        </div>`
      },
      MonitorCardGrid: {
        props: ['countdownSeconds'],
        template: '<output data-test="countdown">{{ countdownSeconds }}</output>'
      }
    }
  }
})

let wrapper: ReturnType<typeof mountView>

beforeEach(() => {
  vi.useFakeTimers()
  localStorage.clear()
  list.mockReset().mockResolvedValue({ items: [] })
})

afterEach(() => {
  wrapper?.unmount()
  vi.useRealTimers()
  localStorage.clear()
})

describe('channel monitor refresh interval', () => {
  it('keeps automatic refresh disabled after reopening the page', async () => {
    wrapper = mountView()
    await flushPromises()
    await wrapper.get('.disable').trigger('click')
    wrapper.unmount()
    wrapper = mountView()
    await flushPromises()

    const calls = list.mock.calls.length
    await vi.advanceTimersByTimeAsync(240000)
    expect(list).toHaveBeenCalledTimes(calls)
    expect(JSON.parse(localStorage.getItem('channel-status-auto-refresh')!).enabled).toBe(false)

    await wrapper.get('.refresh').trigger('click')
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(calls + 1)
  })

  it('starts automatic refresh for a first visit', async () => {
    wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(121000)

    expect(list.mock.calls.length).toBeGreaterThan(1)
  })

  it('resets the countdown to the selected interval after manual refresh', async () => {
    wrapper = mountView()
    await flushPromises()

    await wrapper.get('.interval').trigger('click')
    expect(wrapper.get('[data-test="countdown"]').text()).toBe('120')

    await wrapper.get('.refresh').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="countdown"]').text()).toBe('120')
  })

})
