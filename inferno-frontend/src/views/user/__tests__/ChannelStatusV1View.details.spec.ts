import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import ChannelStatusV1View from '../ChannelStatusV1View.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), status: vi.fn() }))
vi.mock('@/api/channelMonitor', () => mocks)
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: { channel_monitor_enabled: true }, showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)
afterEach(() => { vi.useRealTimers(); localStorage.clear() })

function detail(id: number, availability15d: number, availability30d: number) {
  return { id, models: [{ model: 'model', availability_15d: availability15d, availability_30d: availability30d }] }
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.resetAllMocks()
  localStorage.clear()
  localStorage.setItem('channel-status-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 30 }))
  mocks.list.mockImplementation(async () => ({ items: [
    { id: 1, name: 'Monitor one', primary_model: 'model', primary_status: 'operational', availability_7d: 99 },
    { id: 2, name: 'Monitor two', primary_model: 'model', primary_status: 'operational', availability_7d: 98 }
  ] }))
  mocks.status.mockImplementation(async (id: number) => id === 1 ? detail(1, 90, 80) : detail(2, 91, 81))
})

function mountView() {
  return shallowMount(ChannelStatusV1View, {
    global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      MonitorHero: { props: ['loading'], emits: ['update:window', 'refresh'], template: `<div :data-loading="loading">
        <button class="fifteen" @click="$emit('update:window', '15d')">15d</button>
        <button class="thirty" @click="$emit('update:window', '30d')">30d</button>
        <button class="refresh" @click="$emit('refresh')">Refresh</button>
      </div>` },
      MonitorCardGrid: { props: ['items', 'window', 'detailCache'], template: '<output>{{ items.map(item => window === "7d" ? item.availability_7d : detailCache[item.id]?.models[0]?.[window === "15d" ? "availability_15d" : "availability_30d"]).join(",") }}</output>' },
      MonitorDetailDialog: true
    } }
  })
}

describe('channel monitor automatic detail refresh', () => {
  it.each([
    { tab: 'fifteen', initial: '90,91', refreshed: '98,99', values: { one15: 98, one30: 88, two15: 99, two30: 89 } },
    { tab: 'thirty', initial: '80,81', refreshed: '88,89', values: { one15: 98, one30: 88, two15: 99, two30: 89 } }
  ])('refreshes every cached $tab-day availability on timer ticks', async ({ tab, initial, refreshed, values }) => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.' + tab).trigger('click')
    await flushPromises()
    expect(wrapper.get('output').text()).toBe(initial)
    mocks.status.mockImplementation(async (id: number) => id === 1 ? detail(1, values.one15, values.one30) : detail(2, values.two15, values.two30))
    await vi.advanceTimersByTimeAsync(32000)
    await flushPromises()
    expect(wrapper.get('output').text()).toBe(refreshed)
    expect(mocks.status).toHaveBeenCalledTimes(4)
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
  })

  it('keeps the seven-day view on the list endpoint without extra detail requests', async () => {
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(32000)
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(mocks.status).not.toHaveBeenCalled()
    expect(wrapper.get('output').text()).toBe('99,98')
  })

  it('still refreshes all cached availability on manual reload', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.fifteen').trigger('click')
    await flushPromises()
    mocks.status.mockImplementation(async (id: number) => id === 1 ? detail(1, 98, 88) : detail(2, 99, 89))
    await wrapper.get('.refresh').trigger('click')
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('98,99')
    expect(mocks.status).toHaveBeenCalledTimes(4)
  })

  it('stops timer reloads after the page unmounts', async () => {
    const wrapper = mountView()
    await flushPromises()
    const callsBeforeUnmount = mocks.list.mock.calls.length
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(64000)
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(callsBeforeUnmount)
  })
})
