import { describe, expect, it } from 'vitest'

import { sumWindow, toDelta } from '../dayOverDay'
import type { TrendDataPoint } from '@/types'

function point(date: string, requests: number, tokens = 0, cost = 0): TrendDataPoint {
  return {
    date,
    requests,
    input_tokens: 0,
    output_tokens: 0,
    cache_creation_tokens: 0,
    cache_read_tokens: 0,
    total_tokens: tokens,
    cost: 0,
    actual_cost: cost
  }
}

describe('sumWindow', () => {
  const trend = [
    point('2026-08-10 08:00', 10, 100, 1),
    point('2026-08-10 14:00', 20, 200, 2),
    point('2026-08-10 22:00', 500, 5000, 50), // yesterday evening: must be excluded at 14:00
    point('2026-08-11 08:00', 30, 300, 3),
    point('2026-08-11 14:00', 40, 400, 4)
  ]

  /*
   * The whole reason this function exists. Without the hour bound, yesterday
   * totals 530 requests against today's 70 and every tile reads "down 87%"
   * all day long, reaching parity only at midnight.
   */
  it('excludes buckets later than the bound, so a partial day compares to a partial day', () => {
    expect(sumWindow(trend, '2026-08-10', 14)).toEqual({ requests: 30, tokens: 300, cost: 3 })
    expect(sumWindow(trend, '2026-08-11', 14)).toEqual({ requests: 70, tokens: 700, cost: 7 })
  })

  it('includes the bounding hour itself', () => {
    expect(sumWindow(trend, '2026-08-11', 8).requests).toBe(30)
    expect(sumWindow(trend, '2026-08-11', 7).requests).toBe(0)
  })

  it('ignores other days entirely', () => {
    expect(sumWindow(trend, '2026-08-09', 23)).toEqual({ requests: 0, tokens: 0, cost: 0 })
  })

  /*
   * Guards the parsing choice. `new Date('2026-08-10 14:00')` is outside the
   * ECMAScript spec and has historically been Invalid Date in Safari, which
   * would make every bucket fail its hour test and zero the comparison for
   * those users only -- a bug invisible in Chrome-based verification.
   */
  it('parses the space-separated bucket format without Date', () => {
    expect(sumWindow([point('2026-08-11 23:00', 7)], '2026-08-11', 23).requests).toBe(7)
  })

  it('survives malformed rows rather than producing NaN totals', () => {
    const dirty = [
      point('2026-08-11 09:00', Number.NaN, Number.POSITIVE_INFINITY, 5),
      { ...point('2026-08-11 10:00', 3), date: undefined as unknown as string }
    ]
    expect(sumWindow(dirty, '2026-08-11', 23)).toEqual({ requests: 0, tokens: 0, cost: 5 })
  })
})

describe('toDelta', () => {
  it('reports whole-percent change with a direction', () => {
    expect(toDelta(121, 100)).toEqual({ pct: 21, direction: 'up' })
    expect(toDelta(88, 100)).toEqual({ pct: 12, direction: 'down' })
  })

  it('returns null with no baseline, rather than Infinity or "up 100%"', () => {
    // First traffic ever is a first occurrence, not a percentage increase.
    expect(toDelta(500, 0)).toBeNull()
    expect(toDelta(0, 0)).toBeNull()
    expect(toDelta(5, -1)).toBeNull()
  })

  it('returns null for changes that would round to 0%', () => {
    // "0%" reads as a broken chip; nothing reads as a flat day.
    expect(toDelta(100.4, 100)).toBeNull()
    expect(toDelta(100.6, 100)).toEqual({ pct: 1, direction: 'up' })
  })

  it('handles a drop to zero', () => {
    expect(toDelta(0, 250)).toEqual({ pct: 100, direction: 'down' })
  })
})
