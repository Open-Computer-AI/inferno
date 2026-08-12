import type { TrendDataPoint } from '@/types'

export type Delta = { pct: number; direction: 'up' | 'down' } | null

export interface WindowTotals {
  requests: number
  tokens: number
  cost: number
}

function finite(value: unknown): number {
  const n = Number(value)
  return Number.isFinite(n) ? n : 0
}

/**
 * Sums the hourly buckets belonging to one day, up to and including one hour.
 *
 * THE HOUR BOUND IS THE POINT. Comparing today's running total against
 * yesterday's finished total is the standard dashboard bug: at 14:00 today is
 * 58% of a day and yesterday is 100% of one, so every metric shows a permanent
 * "down 40%" that only reaches zero at midnight. Bounding both sides at the
 * same hour is what makes the two numbers comparable at all.
 *
 * Buckets arrive as "YYYY-MM-DD HH24:00" (the backend's dateFormatWhitelist).
 * Parsed by slicing rather than `new Date`, for two reasons:
 *
 *   1. A space-separated datetime is outside the ECMAScript date-parsing spec.
 *      Chrome accepts it; Safari has historically returned Invalid Date.
 *   2. Converting to Date at all would drag the browser's timezone into
 *      buckets the backend already emitted in its own, silently shifting rows
 *      across the day boundary for anyone not on the server's clock.
 *
 * Comparing "same hour, adjacent days" needs neither, so it does neither.
 */
export function sumWindow(
  points: TrendDataPoint[],
  day: string,
  throughHour: number
): WindowTotals {
  const totals: WindowTotals = { requests: 0, tokens: 0, cost: 0 }
  for (const point of points) {
    const date = point?.date
    if (typeof date !== 'string' || date.slice(0, 10) !== day) continue
    const hour = Number(date.slice(11, 13))
    if (!Number.isFinite(hour) || hour > throughHour) continue
    totals.requests += finite(point.requests)
    totals.tokens += finite(point.total_tokens)
    totals.cost += finite(point.actual_cost)
  }
  return totals
}

/**
 * The latest hour that actually has a bucket on `day`, or -1 if none does.
 *
 * WHY THE BROWSER CLOCK IS NOT ENOUGH. Buckets are stamped by TO_CHAR in the
 * DATABASE's timezone, but `new Date().getHours()` is the browser's. A dev DB
 * on Asia/Shanghai serving a browser on IST is 2.5 hours ahead, so bounding at
 * the browser's hour silently discards the two most recent buckets, and
 * today's side of the comparison undercounts with no error anywhere -- which
 * inflates yesterday's relative share and understates the delta.
 *
 * Callers should bound at max(browserHour, this). Max is the safe direction:
 * over-including hours only appends zeros, while under-including drops real
 * data. That self-corrects whichever way the two clocks are offset, and needs
 * no timezone from the backend.
 */
export function latestHourOn(points: TrendDataPoint[], day: string): number {
  let latest = -1
  for (const point of points) {
    const date = point?.date
    if (typeof date !== 'string' || date.slice(0, 10) !== day) continue
    const hour = Number(date.slice(11, 13))
    if (Number.isFinite(hour) && hour > latest) latest = hour
  }
  return latest
}

/**
 * Percentage change against a baseline, or null when there is no honest one.
 *
 * Returns null rather than a number in two cases, both deliberate:
 *
 *   previous <= 0   Going from nothing to something is not "up 100%", it is a
 *                   first occurrence. Dividing by it yields Infinity.
 *   |change| < 0.5  Rounds to "0%", which reads as a broken chip rather than
 *                   as a flat day. Showing nothing is the honest render.
 */
export function toDelta(current: number, previous: number): Delta {
  if (!(previous > 0)) return null
  const change = ((finite(current) - previous) / previous) * 100
  if (!Number.isFinite(change) || Math.abs(change) < 0.5) return null
  return { pct: Math.round(Math.abs(change)), direction: change > 0 ? 'up' : 'down' }
}
