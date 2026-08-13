import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsSettingsDialog from '../OpsSettingsDialog.vue'

const mockGetAlertRuntimeSettings = vi.fn()
const mockUpdateAlertRuntimeSettings = vi.fn()
const mockGetEmailNotificationConfig = vi.fn()
const mockUpdateEmailNotificationConfig = vi.fn()
const mockGetAdvancedSettings = vi.fn()
const mockUpdateAdvancedSettings = vi.fn()
const mockGetMetricThresholds = vi.fn()
const mockUpdateMetricThresholds = vi.fn()
const mountedWrappers: Array<{ unmount: () => void }> = []

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAlertRuntimeSettings: (...args: any[]) => mockGetAlertRuntimeSettings(...args),
    updateAlertRuntimeSettings: (...args: any[]) => mockUpdateAlertRuntimeSettings(...args),
    getEmailNotificationConfig: (...args: any[]) => mockGetEmailNotificationConfig(...args),
    updateEmailNotificationConfig: (...args: any[]) => mockUpdateEmailNotificationConfig(...args),
    getAdvancedSettings: (...args: any[]) => mockGetAdvancedSettings(...args),
    updateAdvancedSettings: (...args: any[]) => mockUpdateAdvancedSettings(...args),
    getMetricThresholds: (...args: any[]) => mockGetMetricThresholds(...args),
    updateMetricThresholds: (...args: any[]) => mockUpdateMetricThresholds(...args)
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function runtime(overrides: Record<string, unknown> = {}) {
  return {
    evaluation_interval_seconds: 30,
    distributed_lock: {
      enabled: true,
      key: 'ops:alert:evaluator:leader',
      ttl_seconds: 30
    },
    silencing: {
      enabled: false,
      global_until_rfc3339: '',
      global_reason: '',
      entries: []
    },
    thresholds: {
      sla_percent_min: 99.5,
      ttft_p99_ms_max: 500,
      request_error_rate_percent_max: 5,
      upstream_error_rate_percent_max: 5
    },
    ...overrides
  }
}

function emailConfig() {
  return {
    alert: { enabled: false, recipients: [], min_severity: 'critical' },
    report: {
      enabled: false,
      recipients: [],
      daily_summary_enabled: false,
      daily_summary_schedule: '0 9 * * *',
      weekly_summary_enabled: false,
      weekly_summary_schedule: '0 9 * * 1'
    }
  }
}

function advancedSettings() {
  return {
    data_retention: {
      cleanup_enabled: true,
      cleanup_schedule: '0 2 * * *',
      error_log_retention_days: 30,
      minute_metrics_retention_days: 7,
      hourly_metrics_retention_days: 30
    },
    aggregation: { aggregation_enabled: true },
    openai_account_quota_auto_pause: { default_threshold_5h: 0, default_threshold_7d: 0 },
    ignore_count_tokens_errors: false,
    ignore_context_canceled: false,
    ignore_no_available_accounts: false,
    ignore_invalid_api_key_errors: false,
    ignore_insufficient_balance_errors: false,
    display_openai_token_stats: true,
    display_alert_events: true,
    auto_refresh_enabled: true,
    auto_refresh_interval_seconds: 30
  }
}

async function mountDialog(runtimePayload: Record<string, unknown> = runtime()) {
  mockGetAlertRuntimeSettings.mockResolvedValue(runtimePayload)
  mockGetEmailNotificationConfig.mockResolvedValue(emailConfig())
  mockGetAdvancedSettings.mockResolvedValue(advancedSettings())
  mockGetMetricThresholds.mockResolvedValue({})

  const wrapper = mount(OpsSettingsDialog, { props: { show: false }, attachTo: document.body })
  mountedWrappers.push(wrapper)
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

function bodyElement(selector: string): HTMLElement {
  const element = document.body.querySelector<HTMLElement>(selector)
  expect(element).not.toBeNull()
  return element!
}

function bodyButton(text: string): HTMLButtonElement {
  const button = [...document.body.querySelectorAll<HTMLButtonElement>('button')].find((candidate) => candidate.textContent === text)
  expect(button).toBeDefined()
  return button!
}

function setInput(selector: string, value: string) {
  const input = bodyElement(selector) as HTMLInputElement
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('OpsSettingsDialog runtime controls', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUpdateAlertRuntimeSettings.mockResolvedValue(runtime())
    mockUpdateEmailNotificationConfig.mockResolvedValue(emailConfig())
    mockUpdateAdvancedSettings.mockResolvedValue(advancedSettings())
    mockUpdateMetricThresholds.mockResolvedValue(undefined)
  })

  afterEach(() => {
    mountedWrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  })

  it('normalizes older runtime payloads so the consolidated controls remain editable', async () => {
    await mountDialog({
      evaluation_interval_seconds: 30,
      distributed_lock: { enabled: true },
      silencing: { enabled: false },
      thresholds: {}
    })

    expect(bodyElement('[data-testid="ops-runtime-controls"]')).toBeDefined()
    expect(bodyElement('[data-testid="ops-silencing-enabled"]')).toBeDefined()
    expect(bodyElement('[data-testid="ops-lock-enabled"]')).toBeDefined()
    expect((bodyElement('[data-testid="ops-lock-key"] input') as HTMLInputElement).value).toBe('ops:alert:evaluator:leader')
  })

  it('round-trips edited silence and lock settings through the runtime endpoint', async () => {
    await mountDialog()

    bodyElement('[data-testid="ops-silencing-enabled"]').click()
    await flushPromises()
    setInput('[data-testid="ops-lock-key"] input', 'ops:maintenance:leader')
    setInput('[data-testid="ops-silencing-global-until"] input', '2026-08-14T00:00:00Z')
    bodyButton('admin.ops.runtime.silencing.entries.add').click()
    await flushPromises()
    setInput('[data-testid="ops-silencing-entry-until"] input', '2026-08-14T00:00:00Z')
    await flushPromises()

    const saveButton = bodyButton('common.save')
    expect(saveButton.disabled).toBe(false)
    saveButton.click()
    await flushPromises()

    expect(mockUpdateAlertRuntimeSettings).toHaveBeenCalledWith(expect.objectContaining({
      distributed_lock: expect.objectContaining({ key: 'ops:maintenance:leader' }),
      silencing: expect.objectContaining({
        enabled: true,
        global_until_rfc3339: '2026-08-14T00:00:00Z',
        entries: [expect.objectContaining({ until_rfc3339: '2026-08-14T00:00:00Z' })]
      })
    }))
  })

  it('blocks saving an enabled targeted silence without a valid expiry', async () => {
    await mountDialog()

    bodyElement('[data-testid="ops-silencing-enabled"]').click()
    await flushPromises()
    bodyButton('admin.ops.runtime.silencing.entries.add').click()
    await flushPromises()
    setInput('[data-testid="ops-silencing-entry-until"] input', 'not-a-timestamp')

    bodyButton('common.save').click()
    await flushPromises()

    expect(mockUpdateAlertRuntimeSettings).not.toHaveBeenCalled()
    expect(document.body.textContent).toContain('admin.ops.runtime.silencing.entries.validation.untilFormat')
  })
})
