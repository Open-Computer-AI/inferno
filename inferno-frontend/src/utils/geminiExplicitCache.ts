export const MAX_EXPLICIT_CACHE_UPSTREAM_TTL_SECONDS = 24 * 60 * 60

/** Validate an optional Gemini upstream TTL cap in seconds. */
export function normalizeExplicitCacheUpstreamMaxTTL(value: unknown): number | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'string' && value.trim() === '') return null
  if (typeof value !== 'string' && typeof value !== 'number') {
    throw new TypeError('TTL cap must be a number or numeric string')
  }
  const seconds = typeof value === 'string' ? Number(value.trim()) : value
  if (!Number.isFinite(seconds) || Math.abs(seconds) > Number.MAX_SAFE_INTEGER) {
    throw new RangeError('TTL cap must be a finite safe number')
  }
  const normalized = Math.floor(seconds)
  if (!Number.isSafeInteger(normalized) || normalized < 1 || normalized > MAX_EXPLICIT_CACHE_UPSTREAM_TTL_SECONDS) {
    throw new RangeError(`TTL cap must be between 1 and ${MAX_EXPLICIT_CACHE_UPSTREAM_TTL_SECONDS} seconds`)
  }
  return normalized
}

/** Add or remove the explicit-cache upstream settings from Gemini credentials. */
export function applyExplicitCacheUpstreamCredentials(
  credentials: Record<string, unknown>,
  enabled: boolean,
  maxTTL: unknown
): void {
  if (!enabled) {
    delete credentials.explicit_cache_upstream
    delete credentials.explicit_cache_upstream_max_ttl_seconds
    return
  }
  credentials.explicit_cache_upstream = true
  const seconds = normalizeExplicitCacheUpstreamMaxTTL(maxTTL)
  if (seconds === null) {
    delete credentials.explicit_cache_upstream_max_ttl_seconds
  } else {
    credentials.explicit_cache_upstream_max_ttl_seconds = seconds
  }
}
