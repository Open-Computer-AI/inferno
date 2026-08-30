import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpsTrafficSplit from '../OpsTrafficSplit.vue'

vi.mock('vue-i18n', async () => {
  const a = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...a, useI18n: () => ({ t: (k: string) => k }) }
})

function mountSplit(sla: number | null, requestCountSla: number) {
  return mount(OpsTrafficSplit, {
    props: { successCount: 0, errorCountSla: 0, businessLimitedCount: 0, sla, requestCountSla },
    global: { stubs: { HelpTooltip: true, SplitBar: true, Icon: true } },
  })
}

// 0d5e3ca9b: an empty window reports sla = 0. Showing "0.000%" reads as a total
// outage; the card must show "-" until there is something to measure.
describe('OpsTrafficSplit SLA headline', () => {
  it('shows a dash when the window had no SLA-eligible requests', () => {
    const w = mountSplit(0, 0)
    expect(w.get('.traffic__sla').text()).toContain('-')
    expect(w.get('.traffic__sla').text()).not.toContain('0.000%')
    w.unmount()
  })

  it('shows the real percentage once there are requests', () => {
    const w = mountSplit(0.99912, 1000)
    expect(w.get('.traffic__sla').text()).toContain('99.912%')
    w.unmount()
  })

  it('still shows a dash when sla itself is null', () => {
    const w = mountSplit(null, 1000)
    expect(w.get('.traffic__sla').text()).toContain('-')
    w.unmount()
  })
})
