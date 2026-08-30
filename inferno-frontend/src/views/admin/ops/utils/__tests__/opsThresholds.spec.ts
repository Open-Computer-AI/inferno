/**
 * The verdicts themselves, with the real numbers from a live install
 * (ops_metric_thresholds = sla_percent_min 99.5, request_error_rate_percent_max 5,
 * ttft_p99_ms_max 500, upstream_error_rate_percent_max 5).
 *
 * The June rewrite dropped two of the four verdict functions and nothing
 * caught it, because they were only reachable through a heavy mount of a
 * ~1500-line view. These are pure, so there is no excuse now.
 */
import { describe, it, expect } from 'vitest'
import { getSLAThresholdLevel, getCeilingThresholdLevel } from '../opsThresholds'

describe('getSLAThresholdLevel (higher is better)', () => {
  const floor = 99.5

  it('is critical below the configured floor', () => {
    // The exact regression: an operator set 99.5 and saw nothing until the
    // diagnosis panel's absolute 98 -- four times looser than they asked for.
    expect(getSLAThresholdLevel(98.5, floor)).toBe('critical')
    expect(getSLAThresholdLevel(99.49, floor)).toBe('critical')
  })

  it('warns inside the 0.1 buffer above the floor, while still passing', () => {
    expect(getSLAThresholdLevel(99.5, floor)).toBe('warning')
    expect(getSLAThresholdLevel(99.55, floor)).toBe('warning')
  })

  it('is normal once clear of the buffer', () => {
    expect(getSLAThresholdLevel(99.6, floor)).toBe('normal')
    expect(getSLAThresholdLevel(99.7, floor)).toBe('normal')
    expect(getSLAThresholdLevel(100, floor)).toBe('normal')
  })

  it('judges nothing when no threshold is configured', () => {
    expect(getSLAThresholdLevel(12, null)).toBe('normal')
    expect(getSLAThresholdLevel(12, undefined)).toBe('normal')
  })

  it('judges nothing when there is no SLA to judge', () => {
    expect(getSLAThresholdLevel(null, floor)).toBe('normal')
  })
})

describe('getCeilingThresholdLevel (lower is better)', () => {
  it('is critical at or above the ceiling', () => {
    expect(getCeilingThresholdLevel(5, 5)).toBe('critical')
    expect(getCeilingThresholdLevel(6.2, 5)).toBe('critical')
    expect(getCeilingThresholdLevel(500, 500)).toBe('critical')
  })

  it('warns from 80% of the ceiling', () => {
    expect(getCeilingThresholdLevel(4, 5)).toBe('warning')
    expect(getCeilingThresholdLevel(400, 500)).toBe('warning')
  })

  it('is normal below 80% of the ceiling', () => {
    expect(getCeilingThresholdLevel(3.99, 5)).toBe('normal')
    expect(getCeilingThresholdLevel(0, 5)).toBe('normal')
  })

  it('judges nothing without a threshold or a value', () => {
    expect(getCeilingThresholdLevel(999, null)).toBe('normal')
    expect(getCeilingThresholdLevel(null, 5)).toBe('normal')
  })
})
