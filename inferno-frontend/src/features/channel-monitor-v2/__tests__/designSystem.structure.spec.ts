/**
 * Structure contracts: channel-monitor-v2 + studio shells must use project
 * design-system utility classes rather than isolated flat RGB skins.
 */
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = resolve(__dirname, '../../..')

function read(rel: string) {
  return readFileSync(resolve(root, rel), 'utf8')
}

describe('channel-monitor-v2 design system structure', () => {
  it('user ChannelStatus V2 shell uses page-header, surface-card, shared actions, and tabs utilities', () => {
    // Route wrapper may switch V1/V2; design chrome lives on the V2 implementation.
    const src = read('views/user/ChannelStatusV2View.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('page-title')
    expect(src).toContain('class="surface-card')
    expect(src).toContain('<AppButton')
    expect(src).toContain('variant="secondary"')
    expect(src).toContain('<Segmented')
    expect(src).toContain('variant="segmented"')
    expect(src).toContain('variant="tabs"')
    expect(src).toContain('badge badge-warning')
    // Compact single-row toolbar
    expect(src).toContain('monitor-toolbar')
    expect(src).toContain('clearFilters')
    expect(src).toContain('healthModeOptions')
    expect(src).toContain("'cache'")
    // Ops elevation: rounded-3xl + ring surfaces
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-[var(--brand-line)/5]')
    // Overview-first KPI strip before primary viz
    expect(src.indexOf('summaryAria')).toBeLessThan(src.indexOf('MonitorTrendChart'))
    // No page-level fixed min-width that forces viewport horizontal scroll
    expect(src).not.toMatch(/min-width:\s*980px/)
    expect(src).not.toMatch(/min-w-\[980px\]/)
    // Dense tables scroll internally
    expect(src).toMatch(/max-h-\[min\(52vh/)
    expect(src).toContain('overflow-auto')
    // Trend view toggle (pulse matrix / line chart) + default platform/group dimension
    expect(src).toContain("trendView")
    expect(src).toContain("'platform_group'")
    expect(src).toContain('MonitorTrendChart')
  })

  it('RelayPulseMatrix uses card chrome, matrix scroll, and hover tooltips (no click modal)', () => {
    const src = read('features/channel-monitor-v2/RelayPulseMatrix.vue')
    expect(src).toContain('class="surface-card')
    expect(src).toContain('surface-card-header')
    expect(src).toContain('surface-card-body')
    expect(src).toContain('matrix-scroll')
    expect(src).toMatch(/max-h-\[min\(42vh/)
    expect(src).toContain('overflow-auto')
    expect(src).toContain('pulse-tooltip')
    expect(src).toContain('rounded-3xl')
    expect(src).toContain('ring-1 ring-[var(--brand-line)/5]')
    expect(src).not.toContain('modal-overlay')
    expect(src).not.toContain('modal-content')
  })

  it('MetricCell uses surface-card utility', () => {
    const src = read('features/channel-monitor-v2/MetricCell.vue')
    expect(src).toContain('surface-card')
    expect(src).toContain('stat-label')
    expect(src).toContain('stat-value')
    expect(src).toContain('min-h-[6.5rem]')
  })

  it('MonitorTrendChart uses Ops chart shell tokens', () => {
    const src = read('features/channel-monitor-v2/MonitorTrendChart.vue')
    /*
     * Was asserting the pre-June chrome by string: `class="card`, `rounded-3xl`,
     * `ring-1 ring-gray-900/5`, `min-h-[360px]`. The intent -- this chart keeps
     * the ops chart card shell and its minimum height -- is unchanged; only the
     * idiom is, so the assertions moved to the tokens that now express it.
     * Same count, plus one the old set could not make: that the chart is off
     * Chart.js, which is the whole point of the change.
     */
    expect(src).toContain('background: var(--card)')
    expect(src).toContain('border-radius: var(--r-lg)')
    expect(src).toContain('border: var(--border-width) solid var(--border-subtle)')
    expect(src).toContain('min-height: 360px')
    expect(src).toContain('EmptyState')
    expect(src).toContain('DitherArea')
    expect(src).not.toContain('vue-chartjs')
  })

  it('FilterMultiSelect uses rounded-xl input chrome and owned option utility', () => {
    const src = read('features/channel-monitor-v2/FilterMultiSelect.vue')
    expect(src).toContain('rounded-xl')
    expect(src).toContain('filter-dropdown')
    expect(src).toContain('select-option')
    expect(src).not.toContain('dropdown-item')
  })

  it('MonitorSettingsPanel uses page-header, surface-card, shared primary action, and tabs', () => {
    const src = read('features/channel-monitor-v2/MonitorSettingsPanel.vue')
    expect(src).toContain('page-header')
    expect(src).toContain('<AppButton')
    expect(src).toContain('variant="solid"')
    expect(src).toContain('class="surface-card')
    expect(src).toContain('<Segmented')
    expect(src).toContain('variant="segmented"')
    expect(src).not.toContain('tab-active')
    expect(src).toMatch(/max-h-\[min\(40vh/)
  })

  it('admin ChannelMonitorView V2 mode chrome uses the project segmented control', () => {
    const src = read('views/admin/ChannelMonitorView.vue')
    expect(src).toContain('monitor-mode-panel')
    expect(src).toContain('<Segmented')
    expect(src).toContain('MonitorSettingsPanel')
  })
})
