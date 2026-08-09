import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OllamaCloudUsageCell from '../OllamaCloudUsageCell.vue'
import type { Account, OllamaCloudUsageState } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const usageState = (): OllamaCloudUsageState => ({
  account_id: 7,
  eligible: true,
  configured: true,
  auto_refresh_enabled: false,
  encryption_key_configured: true,
  snapshot: {
    status: 'ok',
    fetched_at: '2026-07-22T12:00:00Z',
    last_attempt_at: '2026-07-22T12:00:00Z',
    next_refresh_at: '2026-07-22T13:00:00Z',
    data: {
      plan: 'max',
      five_hour: { used_percent: 5.6, reset_at: '2026-07-23T03:00:00Z' },
      seven_day: { used_percent: 14.2, reset_at: '2026-07-29T00:00:00Z' },
      balance: '$0',
      models: [
        { model: 'gpt-oss:120b-cloud', window: 'five_hour', requests: 2 },
        { model: 'gpt-oss:120b-cloud', window: 'seven_day', requests: 12 }
      ]
    }
  }
})

const account = (state = usageState()): Account => ({
  id: 7,
  name: 'ollama',
  platform: 'openai',
  type: 'apikey',
  ollama_cloud_usage: state,
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: false,
  created_at: '2026-07-22T00:00:00Z',
  updated_at: '2026-07-22T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null
})

describe('OllamaCloudUsageCell', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // Before both windows' reset_at, so the "resets in" countdown renders
    // deterministically instead of silently disappearing once real wall
    // time passes the fixture's reset timestamps.
    vi.setSystemTime(new Date('2026-07-22T12:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders only native 5h and 7d windows in a shrinkable mobile-safe cell', () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() } })
    const cell = wrapper.get('[data-testid="ollama-cloud-usage-cell"]')
    // CONVENTIONS rule 5 forbids Tailwind utilities in migrated components,
    // so the old min-w-0/max-w-full/min-w-[12rem] literal-class assertions
    // can never pass and be correct. The "doesn't force a wide minimum
    // width" guarantee now lives in the component's own scoped `min-width:
    // 0` rule on `.ocu`, not a utility class -- only that one class renders.
    expect(cell.classes()).toEqual(['ocu'])

    // Two windows (5h, 7d), each its own row with a CapacityBar meter, a
    // rounded percent, and a reset countdown -- what UsageProgressBar used
    // to render, now composed from CapacityBar plus this cell's own markup.
    expect(wrapper.findAll('[role="progressbar"]')).toHaveLength(2)
    const fiveHour = wrapper.get('[data-testid="ollama-cloud-five-hour"]')
    const sevenDay = wrapper.get('[data-testid="ollama-cloud-seven-day"]')
    expect(fiveHour.text()).toContain('admin.accounts.ollamaCloud.fiveHourShort')
    expect(fiveHour.text()).toContain('6%') // Math.round(5.6)
    expect(fiveHour.text()).toContain('misc.resetIn')
    expect(sevenDay.text()).toContain('admin.accounts.ollamaCloud.sevenDayShort')
    expect(sevenDay.text()).toContain('14%') // Math.round(14.2)
    expect(sevenDay.text()).toContain('misc.resetIn')

    expect(wrapper.find('[data-testid="ollama-cloud-usage-details"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="ollama-cloud-usage-refresh"]').exists()).toBe(false)
    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.text()).not.toContain('max')
    expect(wrapper.text()).not.toContain('$0')
    expect(wrapper.text()).not.toContain('gpt-oss:120b-cloud')
  })

  it('reacts to an account snapshot update without a list-cell refresh action', async () => {
    const wrapper = mount(OllamaCloudUsageCell, { props: { account: account() } })
    const next = usageState()
    next.snapshot!.data!.five_hour!.used_percent = 43

    await wrapper.setProps({ account: account(next) })

    expect(wrapper.get('[data-testid="ollama-cloud-five-hour"]').text()).toContain('43%')
    expect(wrapper.findAll('button')).toHaveLength(0)
  })
})
