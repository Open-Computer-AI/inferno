import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import IntervalRow from '../IntervalRow.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (k: string) => k }) }
})

function mountRow(enableMultipliers: boolean) {
  return mount(IntervalRow, {
    props: {
      mode: 'token',
      interval: {
        min_tokens: 0,
        max_tokens: null,
        input_price: '1',
        output_price: '2',
        cache_write_price: '',
        cache_read_price: '',
        per_request_price: '',
        input_multiplier: '',
        output_multiplier: '',
        cache_write_multiplier: '',
        cache_read_multiplier: '',
      },
      enableMultipliers,
    } as any,
    global: { stubs: { Icon: true } },
  })
}

// 26be82cc8 moved this row into a responsive grid and folded the four per-tier
// multiplier inputs into it. Our tree had already added those four inputs as a
// separate block, so the merge left two copies; deduplicating them removes a
// duplicate, never a control. These assert exactly that -- four inputs, once.
describe('IntervalRow per-tier multipliers', () => {
  const FIELDS = ['input_multiplier', 'output_multiplier', 'cache_write_multiplier', 'cache_read_multiplier']

  it('renders each multiplier input exactly once when enabled', () => {
    const wrapper = mountRow(true)
    const labels = wrapper.findAll('label').map((l) => l.text())
    for (const key of ['inputMultiplier', 'outputMultiplier', 'cacheWriteMultiplier', 'cacheReadMultiplier']) {
      expect(labels.filter((l) => l.includes(key)), key).toHaveLength(1)
    }
    // the 6 base price/token fields plus the 4 multipliers, no duplicates
    expect(wrapper.findAll('input[type="number"]')).toHaveLength(10)
  })

  it('renders none of them when disabled, and keeps every base field', () => {
    const wrapper = mountRow(false)
    const labels = wrapper.findAll('label').map((l) => l.text()).join(' ')
    for (const key of ['inputMultiplier', 'outputMultiplier', 'cacheWriteMultiplier', 'cacheReadMultiplier']) {
      expect(labels).not.toContain(key)
    }
    expect(wrapper.findAll('input[type="number"]')).toHaveLength(6)
  })

  it('emits on each multiplier field', async () => {
    const wrapper = mountRow(true)
    const inputs = wrapper.findAll('input[type="number"]')
    for (let i = 0; i < FIELDS.length; i++) {
      await inputs[6 + i].setValue('2')
    }
    const emitted = (wrapper.emitted('update') ?? []).map((e) => Object.keys(e[0] as object)).flat()
    for (const f of FIELDS) expect(emitted, f).toContain(f)
  })
})
