import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsSystemLogTable from '../OpsSystemLogTable.vue'
import Input from '@/components/common/Input.vue'
import enLocale from '@/i18n/locales/en'
import zhLocale from '@/i18n/locales/zh'

const mockListSystemLogs = vi.fn()
const mockCleanupSystemLogs = vi.fn()
const mockGetSystemLogSinkHealth = vi.fn()
const mockGetRuntimeLogConfig = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listSystemLogs: (...args: any[]) => mockListSystemLogs(...args),
    cleanupSystemLogs: (...args: any[]) => mockCleanupSystemLogs(...args),
    getSystemLogSinkHealth: (...args: any[]) => mockGetSystemLogSinkHealth(...args),
    getRuntimeLogConfig: (...args: any[]) => mockGetRuntimeLogConfig(...args),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: {
    modelValue: {
      type: [String, Number],
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const PaginationStub = defineComponent({
  name: 'PaginationStub',
  template: '<div class="pagination-stub" />',
})

const runtimeConfig = {
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30,
}

const sinkHealth = {
  queue_depth: 0,
  queue_capacity: 5000,
  dropped_count: 0,
  write_failed_count: 0,
  written_count: 1,
  avg_write_delay_ms: 0,
}

/**
 * The host field is an `Input` now, so its label is a sibling of the control
 * rather than wrapping it. Locate the field wrapper, then its input.
 */
function fieldInput(wrapper: ReturnType<typeof mount>, labelText: string) {
  const field = wrapper.findAll('.fld').find((f) => f.text().includes(labelText))
  expect(field, `no field labelled ${labelText}`).toBeDefined()
  return field!.find('input')
}

/**
 * ConfirmDialog teleports to body, so its buttons are not inside the wrapper.
 * A native click on the real node still runs Vue's listener.
 */
async function confirmDialog() {
  const buttons = Array.from(document.querySelectorAll('.cf-btn'))
  const confirm = buttons[buttons.length - 1] as HTMLElement | undefined
  expect(confirm, 'confirm dialog was not opened').toBeDefined()
  confirm!.click()
  await flushPromises()
}

describe('OpsSystemLogTable host support', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
    mockListSystemLogs.mockResolvedValue({
      items: [
        {
          id: 1,
          created_at: '2026-07-14T00:10:01Z',
          host: 'api-node-1',
          level: 'warn',
          component: 'app',
          message: 'request failed',
        },
      ],
      total: 1,
      page: 1,
      page_size: 20,
    })
    mockCleanupSystemLogs.mockResolvedValue({ deleted: 1 })
    mockGetSystemLogSinkHealth.mockResolvedValue(sinkHealth)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
  })

  it('renders the host and sends it with list and cleanup filters', async () => {
    const wrapper = mount(OpsSystemLogTable, {
      global: {
        stubs: {
          Select: SelectStub,
          Pagination: PaginationStub,
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('api-node-1')

    await fieldInput(wrapper, 'admin.ops.systemLogs.host').setValue(' api-node-2 ')

    const searchButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.search')
    expect(searchButton).toBeDefined()
    await searchButton!.trigger('click')
    await flushPromises()

    expect(mockListSystemLogs).toHaveBeenLastCalledWith(expect.objectContaining({ host: 'api-node-2' }))

    const cleanupButton = wrapper.findAll('button').find((button) => button.text() === 'admin.ops.systemLogs.cleanCurrentFilters')
    expect(cleanupButton).toBeDefined()
    await cleanupButton!.trigger('click')
    await flushPromises()

    // The destructive action is now an in-app dialog, not window.confirm, so
    // nothing is deleted until it is confirmed.
    expect(mockCleanupSystemLogs).not.toHaveBeenCalled()
    await confirmDialog()

    expect(mockCleanupSystemLogs).toHaveBeenCalledWith(expect.objectContaining({ host: 'api-node-2' }))
  })

  it.each([
    ['zh', zhLocale],
    ['en', enLocale],
  ])('defines the Host translation for %s', (_name, locale) => {
    expect(locale.admin.ops.systemLogs.host).toBe('Host')
  })
})

describe('OpsSystemLogTable presentation', () => {
  const mountTable = async (items: unknown[], health = sinkHealth) => {
    mockListSystemLogs.mockResolvedValue({ items, total: items.length, page: 1, page_size: 20 })
    mockGetSystemLogSinkHealth.mockResolvedValue(health)
    mockGetRuntimeLogConfig.mockResolvedValue(runtimeConfig)
    const wrapper = mount(OpsSystemLogTable, {
      global: { stubs: { Select: SelectStub, Pagination: PaginationStub } },
    })
    await flushPromises()
    return wrapper
  }

  const accessLog = (over: Record<string, unknown> = {}) => ({
    id: 1,
    created_at: '2026-07-14T00:10:01Z',
    host: 'api-node-1',
    level: 'info',
    component: 'http.access',
    message: 'http request completed',
    request_id: 'ad6bcde2-80b4-460e-a56f-4c8fb4a427a8',
    extra: { status_code: '200', latency_ms: '4', method: 'GET', path: '/api/v1/auth/me' },
    ...over,
  })

  beforeEach(() => {
    vi.clearAllMocks()
    document.body.innerHTML = ''
  })

  /*
   * The detail cell used to be one joined string set with break-all. The same
   * fields are now keyed, so status/latency/path are findable rather than
   * buried mid-sentence.
   */
  it('renders the log detail as keyed fields, not one joined string', async () => {
    const wrapper = await mountTable([accessLog()])

    const facts = wrapper.findAll('.syslog__fact')
    expect(facts.map((f) => f.find('.syslog__fact-key').text())).toEqual([
      'status',
      'latency_ms',
      'method',
      'path',
    ])
    expect(facts[3].find('.syslog__fact-value').text()).toBe('/api/v1/auth/me')

    // Correlation ids sit in their own quieter row.
    expect(wrapper.findAll('.syslog__ref')).toHaveLength(1)
    expect(wrapper.find('.syslog__ref-key').text()).toBe('req')
  })

  /*
   * A 401 on /auth/me is the gateway telling a client no, which is it working.
   * Toning every 4xx would put alarm colour on a large share of ordinary rows.
   */
  it('tones only 5xx statuses, never 4xx', async () => {
    const wrapper = await mountTable([
      accessLog({ id: 1, extra: { status_code: '401' } }),
      accessLog({ id: 2, extra: { status_code: '503' } }),
    ])

    const tones = wrapper.findAll('.syslog__fact').map((f) => f.attributes('data-tone'))
    expect(tones).toEqual([undefined, 'bad'])
  })

  /*
   * `info` was a blue pill on 7,724 of 7,739 rows -- a badge on everything
   * marks nothing, and blue is not in this palette.
   */
  it('badges only warn and error levels', async () => {
    const wrapper = await mountTable([
      accessLog({ id: 1, level: 'info' }),
      accessLog({ id: 2, level: 'debug' }),
      accessLog({ id: 3, level: 'warn' }),
      accessLog({ id: 4, level: 'error' }),
    ])

    expect(wrapper.findAll('.syslog__level-plain').map((n) => n.text())).toEqual(['info', 'debug'])
    expect(wrapper.findAll('.syslog__level').map((n) => n.attributes('data-tone'))).toEqual([
      'warn',
      'error',
    ])
  })

  /* A counter at zero is the good outcome, not an alarm. */
  it('leaves the dropped and failed counters neutral until they are non-zero', async () => {
    const quiet = await mountTable([accessLog()], sinkHealth)
    expect(quiet.findAll('.syslog__pill').every((p) => p.attributes('data-tone') === undefined)).toBe(
      true
    )

    const loud = await mountTable([accessLog()], {
      ...sinkHealth,
      dropped_count: 3,
      write_failed_count: 2,
      queue_depth: 4800,
    })
    expect(loud.findAll('.syslog__pill').map((p) => p.attributes('data-tone'))).toEqual([
      'critical',
      undefined,
      'warning',
      'critical',
    ])
  })

  /*
   * Coercing on every keystroke made clearing the field snap back to the
   * fallback before the next digit landed. The draft holds the raw text and
   * commits on change.
   *
   * Driven through the Input component's own events rather than
   * DOMWrapper.setValue: VTU's input emulation does not agree with a browser
   * about when a `:value` binding repaints, and a minimal Vue repro of it does
   * not even clear the field. What is under test is this component's wiring --
   * which value reaches `runtimeConfig`, and so the API -- not VTU's ability
   * to simulate typing. The repaint itself is verified in a real browser.
   */
  const retentionField = (wrapper: ReturnType<typeof mount>) => {
    const field = wrapper
      .findAllComponents(Input)
      .find((i) => i.props('label') === 'admin.ops.systemLogs.retentionDays')
    expect(field, 'retention days field not found').toBeDefined()
    return field!
  }

  it('holds the raw text while typing and only coerces on commit', async () => {
    const wrapper = await mountTable([accessLog()])
    await wrapper.find('.syslog__config-toggle').trigger('click')

    const vm = wrapper.vm as unknown as {
      drafts: Record<string, string>
      runtimeConfig: Record<string, unknown>
    }
    const field = retentionField(wrapper)

    // Mid-edit: the field is empty and nothing has snapped back under the user.
    field.vm.$emit('update:modelValue', '')
    await nextTick()
    expect(vm.drafts.retention_days).toBe('')
    expect(vm.runtimeConfig.retention_days).toBe(30)

    // Committed on blur or Enter: normalised, never "" or NaN in the payload.
    field.vm.$emit('change', '')
    await nextTick()
    expect(vm.drafts.retention_days).toBe('30')
    expect(vm.runtimeConfig.retention_days).toBe(30)
  })

  it.each([
    ['45', 45],
    ['', 30],
    ['-4', 30],
    ['0', 30]
  ])('commits %s as %i', async (typed, expected) => {
    const wrapper = await mountTable([accessLog()])
    await wrapper.find('.syslog__config-toggle').trigger('click')

    const vm = wrapper.vm as unknown as { runtimeConfig: Record<string, unknown> }
    const field = retentionField(wrapper)

    field.vm.$emit('update:modelValue', typed)
    await nextTick()
    field.vm.$emit('change', typed)
    await nextTick()

    expect(vm.runtimeConfig.retention_days).toBe(expected)
  })

  /* Settings, not reading: closed on arrival, with its live state summarised. */
  it('keeps the runtime config collapsed but its state visible', async () => {
    const wrapper = await mountTable([accessLog()])

    expect(wrapper.find('.syslog__config-body').exists()).toBe(false)
    expect(wrapper.find('.syslog__config-toggle').attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('.syslog__config-summary').text()).toContain('info')

    await wrapper.find('.syslog__config-toggle').trigger('click')
    expect(wrapper.find('.syslog__config-body').exists()).toBe(true)
  })
})
