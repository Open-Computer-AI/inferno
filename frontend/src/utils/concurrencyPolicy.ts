import type { ConcurrencyMode, ConcurrencyPolicyPatch } from '@/types'

export const MAX_CONCURRENCY = 2_147_483_647

export type ConcurrencyLimitInput = string | number | null | undefined

const normalizeAuthMode = (value: unknown): string =>
  typeof value === 'string' ? value.trim().toLowerCase().replace(/[\s_-]+/g, '') : ''

export const isOpenAIAuthModeLiveConcurrencyEligible = (authMode: unknown): boolean => {
  const normalized = normalizeAuthMode(authMode)
  return normalized !== 'personalaccesstoken' &&
    normalized !== 'codexpat' &&
    normalized !== 'agentidentity'
}

export const isOpenAILiveConcurrencyEligible = (value: {
  platform?: string | null
  type?: string | null
  credentials?: Record<string, unknown> | null
} | null | undefined): boolean => {
  if (value?.platform !== 'openai' || value.type !== 'oauth') {
    return false
  }
  return isOpenAIAuthModeLiveConcurrencyEligible(value.credentials?.auth_mode)
}

export const isPositiveConcurrencyLimit = (value: ConcurrencyLimitInput): boolean => {
  if (typeof value === 'string' && value.trim() === '') {
    return false
  }

  const parsed = typeof value === 'number' ? value : Number(value)
  return Number.isInteger(parsed) && parsed > 0 && parsed <= MAX_CONCURRENCY
}

export const parseConcurrencyLimit = (value: ConcurrencyLimitInput): number | null => {
  if (!isPositiveConcurrencyLimit(value)) {
    return null
  }
  return typeof value === 'number' ? value : Number(value)
}

/**
 * Builds the backend patch for a selected policy mode. A null result means a
 * split policy was selected without two valid positive limits.
 */
export const buildConcurrencyPolicy = (
  mode: ConcurrencyMode,
  live: ConcurrencyLimitInput,
  requests: ConcurrencyLimitInput
): ConcurrencyPolicyPatch | null => {
  if (mode === 'shared') {
    return { mode: 'shared' }
  }

  const liveLimit = parseConcurrencyLimit(live)
  const requestLimit = parseConcurrencyLimit(requests)
  if (liveLimit === null || requestLimit === null) {
    return null
  }

  return {
    mode: 'split',
    live: liveLimit,
    requests: requestLimit
  }
}

/**
 * Create forms omit the default shared policy. Edit and bulk forms should use
 * buildConcurrencyPolicy directly when an explicit shared reset is intended.
 */
export const buildOptionalConcurrencyPolicy = (
  mode: ConcurrencyMode,
  live: ConcurrencyLimitInput,
  requests: ConcurrencyLimitInput
): ConcurrencyPolicyPatch | null | undefined =>
  mode === 'shared' ? undefined : buildConcurrencyPolicy(mode, live, requests)

export const getConcurrencyMode = (value: {
  concurrency_mode?: ConcurrencyMode | null
  live_concurrency?: number | null
  request_concurrency?: number | null
}): ConcurrencyMode => {
  if (value.concurrency_mode === 'split' || value.concurrency_mode === 'shared') {
    return value.concurrency_mode
  }

  return isPositiveConcurrencyLimit(value.live_concurrency) &&
    isPositiveConcurrencyLimit(value.request_concurrency)
    ? 'split'
    : 'shared'
}

export const concurrencyInputValue = (value: number | null | undefined): string | number =>
  value == null ? '' : value
