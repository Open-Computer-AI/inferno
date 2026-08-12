import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsAlertEventsCard from '../OpsAlertEventsCard.vue'

const mockListAlertEvents = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listAlertEvents: (...args: any[]) => mockListAlertEvents(...args),
    getAlertEvent: vi.fn(),
    createAlertSilence: vi.fn(),
    updateAlertEventStatus: vi.fn()
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const source = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../OpsAlertEventsCard.vue'),
  'utf8'
)

const HOUR = 60 * 60 * 1000

function event(over: Record<string, unknown> = {}) {
  return {
    id: 1,
    rule_id: 2,
    severity: 'P0',
    status: 'firing',
    title: 'Success rate below 90%',
    description: 'Success rate fell to 81.4%.',
    metric_value: 81.4,
    threshold_value: 90,
    dimensions: { platform: 'anthropic', group_id: 1 },
    fired_at: new Date(Date.now() - 2 * HOUR).toISOString(),
    resolved_at: null,
    email_sent: true,
    created_at: new Date(Date.now() - 2 * HOUR).toISOString(),
    ...over
  }
}

async function mountCard(rows: unknown[]) {
  mockListAlertEvents.mockResolvedValue(rows)
  const wrapper = mount(OpsAlertEventsCard)
  await flushPromises()
  return wrapper
}

describe('OpsAlertEventsCard', () => {
  beforeEach(() => {
    mockListAlertEvents.mockReset()
  })

  /*
   * The load-bearing rule of this card. Severity is a category (a property of
   * the rule that fired); whether it is still firing is the state, and ground
   * rule 5 gives colour to state. So the severity chip is only filled while
   * the event is open -- a P0 that resolved days ago must not shout as loudly
   * as one firing now.
   */
  it('fills the severity chip only while the event is still firing', async () => {
    const wrapper = await mountCard([
      event({ id: 1, severity: 'P0', status: 'firing' }),
      event({ id: 2, severity: 'P0', status: 'resolved', resolved_at: new Date().toISOString() }),
      event({
        id: 3,
        severity: 'P1',
        status: 'manual_resolved',
        resolved_at: new Date().toISOString()
      })
    ])

    const chips = wrapper.findAll('.alerts__sev')
    expect(chips).toHaveLength(3)
    expect(chips[0].attributes('data-open')).toBeDefined()
    expect(chips[1].attributes('data-open')).toBeUndefined()
    expect(chips[2].attributes('data-open')).toBeUndefined()

    // Both P0 rows still SAY P0; only the fill differs.
    expect(chips[0].text()).toBe('P0')
    expect(chips[1].text()).toBe('P0')
  })

  /*
   * The other half of the same rule, in CSS: the base chip must have no fill,
   * and every coloured variant must be gated on [data-open]. A colour that
   * leaks onto a closed row would defeat the whole treatment.
   */
  it('gates every severity colour behind [data-open]', () => {
    const styles = source.slice(source.indexOf('<style scoped>'))
    const severityRules = styles.match(/\.alerts__sev\[[^{]*\{[^}]*\}/g) ?? []
    // One per rank: P0, P1, P2, P3.
    expect(severityRules.length).toBeGreaterThanOrEqual(4)
    for (const rule of severityRules) {
      expect(rule).toContain('[data-open]')
    }
    expect(styles).toMatch(/\.alerts__sev\s*\{[^}]*background:\s*transparent/)
  })

  /*
   * Firing colours its whole label; resolved tints only its dot. Asserted on
   * the tone attribute rather than the paint, since the paint is the CSS's job
   * -- what matters is that the three statuses stay distinguishable.
   */
  it('maps each status to its own tone', async () => {
    const wrapper = await mountCard([
      event({ id: 1, status: 'firing' }),
      event({ id: 2, status: 'resolved', resolved_at: new Date().toISOString() }),
      event({ id: 3, status: 'manual_resolved', resolved_at: new Date().toISOString() }),
      event({ id: 4, status: 'something_else' })
    ])

    expect(wrapper.findAll('.alerts__status').map((s) => s.attributes('data-tone'))).toEqual([
      'firing',
      'resolved',
      'manual',
      'unknown'
    ])
  })

  /*
   * An unrecognised status used to be echoed back through toUpperCase(), which
   * shouted at the reader (ground rule 1).
   */
  it('never upper-cases an unknown status', async () => {
    const wrapper = await mountCard([event({ status: 'somebody_elses_state' })])
    expect(wrapper.find('.alerts__status').text()).toBe('somebody_elses_state')
  })

  /*
   * The duration cell shows the figure alone -- the status column two cells
   * left already supplies the word. The full phrase has to survive on the
   * title, because out of that context "38m" does not say whether it is
   * elapsed-so-far or time-to-resolve.
   */
  it('shows the bare duration but keeps the qualified phrase on the title', async () => {
    const wrapper = await mountCard([event({ fired_at: new Date(Date.now() - 3 * HOUR).toISOString() })])

    const cell = wrapper.find('.alerts__duration')
    expect(cell.text()).toBe('3h')

    const td = wrapper.findAll('.alerts__td').find((c) => c.attributes('title')?.includes('3h'))
    expect(td?.attributes('title')).toBe('admin.ops.alertEvents.status.firing 3h')
  })

  /*
   * Dimensions render one chip per pair rather than a run of key=value text,
   * and a row with no dimensions must not render an empty chip strip.
   */
  it('renders one chip per dimension, and none when there are no dimensions', async () => {
    const wrapper = await mountCard([
      event({ id: 1, dimensions: { platform: 'openai', group_id: 2, region: 'eu-west' } }),
      event({ id: 2, dimensions: null })
    ])

    const rows = wrapper.findAll('.alerts__row')
    expect(rows[0].findAll('.alerts__dim')).toHaveLength(3)
    expect(rows[1].findAll('.alerts__dim')).toHaveLength(0)
    expect(rows[1].find('.alerts__td-empty').exists()).toBe(true)
  })

  /*
   * Infinite scroll has a dead end: a first page that does not overflow its
   * box leaves nothing to scroll, so the scroll handler never fires and the
   * rest of the data is unreachable. Ten rows measured 588px in a 600px box on
   * the live page, so this is the actual boundary, not a hypothetical one.
   */
  it('keeps requesting until the list overflows its container', () => {
    expect(source).toMatch(/el\.scrollHeight\s*>\s*el\.clientHeight/)
    // Called after the first page on mount, on a filter change, and on refresh.
    expect(source.match(/await fillViewport\(\)/g) ?? []).toHaveLength(3)
    // And bounded, so a zero-height container cannot spin it.
    expect(source).toMatch(/guard\s*<\s*5/)
  })

  it('shows the empty state and no rows when nothing fired', async () => {
    const wrapper = await mountCard([])
    expect(wrapper.findAll('.alerts__row')).toHaveLength(0)
    expect(wrapper.find('.alerts__state').exists()).toBe(true)
  })
})
