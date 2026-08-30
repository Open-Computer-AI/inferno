/**
 * Operator-configured metric thresholds -> a severity level.
 *
 * These answer "am I meeting the target the operator set", reading
 * ops_metric_thresholds. They are deliberately SEPARATE from the diagnosis
 * panel in OpsDashboardHeader, which answers "is this bad in absolute terms"
 * and keeps its own hardcoded 90/98 SLA bands. Unifying the two would silence
 * the diagnosis for an operator who configured a lax threshold -- exactly when
 * it should be loudest.
 *
 * They live here rather than inside the component because the component is a
 * ~1500-line view with a dozen children: reachable only through a heavy mount,
 * which is how the June rewrite managed to drop two of these four functions
 * without a single test noticing. sla_percent_min and
 * request_error_rate_percent_max became write-only -- the settings screen
 * accepted and persisted them and nothing read them back.
 */

export type ThresholdLevel = 'normal' | 'warning' | 'critical'

/**
 * SLA is the one that inverts: higher is better, so the floor is breached from
 * BELOW, and a narrow band just above it warns while the operator is still
 * technically passing.
 */
export function getSLAThresholdLevel(
  slaPercent: number | null | undefined,
  threshold: number | null | undefined
): ThresholdLevel {
  if (slaPercent == null) return 'normal'
  if (threshold == null) return 'normal'
  const warningBuffer = 0.1
  if (slaPercent < threshold) return 'critical'
  if (slaPercent < threshold + warningBuffer) return 'warning'
  return 'normal'
}

/**
 * The lower-is-better shape, shared by TTFT and both error rates: critical at
 * or above the ceiling, warning from 80% of it.
 */
export function getCeilingThresholdLevel(
  value: number | null | undefined,
  threshold: number | null | undefined
): ThresholdLevel {
  if (value == null) return 'normal'
  if (threshold == null) return 'normal'
  if (value >= threshold) return 'critical'
  if (value >= threshold * 0.8) return 'warning'
  return 'normal'
}
