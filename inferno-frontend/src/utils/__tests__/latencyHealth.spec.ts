import { describe, expect, it } from 'vitest'

import { durationSeverity, firstTokenSeverity, formatUsageOutputRate } from '../latencyHealth'

describe('latencyHealth', () => {
  it('classifies first-token latency at 10s/30s/60s boundaries', () => {
    expect(firstTokenSeverity(0)).toBe('good')
    expect(firstTokenSeverity(9_999)).toBe('good')
    expect(firstTokenSeverity(10_000)).toBe('warn')
    expect(firstTokenSeverity(29_999)).toBe('warn')
    expect(firstTokenSeverity(30_000)).toBe('slow')
    expect(firstTokenSeverity(59_999)).toBe('slow')
    expect(firstTokenSeverity(60_000)).toBe('critical')
  })

  it('classifies total duration at 1min/3min/5min boundaries', () => {
    expect(durationSeverity(0)).toBe('good')
    expect(durationSeverity(59_999)).toBe('good')
    expect(durationSeverity(60_000)).toBe('warn')
    expect(durationSeverity(179_999)).toBe('warn')
    expect(durationSeverity(180_000)).toBe('slow')
    expect(durationSeverity(299_999)).toBe('slow')
    expect(durationSeverity(300_000)).toBe('critical')
  })
})


describe('formatUsageOutputRate', () => {
  const row = { output_tokens: 1000, duration_ms: 20_000, image_count: 0, image_output_tokens: 0, billing_mode: 'token' as const }
  it('uses total duration and one decimal place', () => {
    expect(formatUsageOutputRate({ ...row, request_type: 'stream' })).toBe('50.0 tok/s')
    expect(formatUsageOutputRate({ ...row, output_tokens: 101, duration_ms: 3000, request_type: 'sync' })).toBe('33.7 tok/s')
  })
  it.each([null, 0, -1, Number.NaN, Number.POSITIVE_INFINITY])('omits missing or invalid durations (%s)', duration_ms => {
    expect(formatUsageOutputRate({ ...row, duration_ms, request_type: 'stream' })).toBe('—')
  })
  it.each([0, -1, Number.NaN, Number.POSITIVE_INFINITY])('omits invalid output-token counts (%s)', output_tokens => {
    expect(formatUsageOutputRate({ ...row, output_tokens, request_type: 'stream' })).toBe('—')
  })
  it.each([
    { image_count: 1 }, { image_output_tokens: 10 }, { billing_mode: 'image' as const },
    { request_type: 'live' as const }, { request_type: 'unknown' as const }, { request_type: undefined },
  ])('omits image and non-text records: %j', overrides => {
    expect(formatUsageOutputRate({ ...row, request_type: 'stream', ...overrides })).toBe('—')
  })
})
