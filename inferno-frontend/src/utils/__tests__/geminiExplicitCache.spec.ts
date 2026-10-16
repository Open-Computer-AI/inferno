import { describe, expect, it } from 'vitest'
import {
  applyExplicitCacheUpstreamCredentials,
  MAX_EXPLICIT_CACHE_UPSTREAM_TTL_SECONDS,
  normalizeExplicitCacheUpstreamMaxTTL
} from '../geminiExplicitCache'

describe('normalizeExplicitCacheUpstreamMaxTTL', () => {
  it('allows blank/unset and valid integer or numeric-string TTL caps', () => {
    expect(normalizeExplicitCacheUpstreamMaxTTL(null)).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL(undefined)).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL('')).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL('  ')).toBeNull()
    expect(normalizeExplicitCacheUpstreamMaxTTL(1800)).toBe(1800)
    expect(normalizeExplicitCacheUpstreamMaxTTL('1800')).toBe(1800)
    expect(normalizeExplicitCacheUpstreamMaxTTL(86400)).toBe(86400)
    expect(normalizeExplicitCacheUpstreamMaxTTL(90.7)).toBe(90)
  })

  it.each([0, -1, 0.1, '0.9', -0.9, MAX_EXPLICIT_CACHE_UPSTREAM_TTL_SECONDS + 1])(
    'rejects a cap outside 1..%s seconds', (value) => {
      expect(() => normalizeExplicitCacheUpstreamMaxTTL(value)).toThrow(RangeError)
    }
  )

  it.each([NaN, Infinity, -Infinity, Number.MAX_SAFE_INTEGER + 1, 'Infinity', '1e100']) (
    'rejects non-finite or unsafe values (%s)', (value) => {
      expect(() => normalizeExplicitCacheUpstreamMaxTTL(value)).toThrow(RangeError)
    }
  )

  it.each([true, false, [], [1], {}, 1n])('rejects values with unsupported types (%s)', (value) => {
    expect(() => normalizeExplicitCacheUpstreamMaxTTL(value)).toThrow(TypeError)
  })

  it('rejects numeric strings with unsafe magnitude', () => {
    expect(() => normalizeExplicitCacheUpstreamMaxTTL('9007199254740993')).toThrow(RangeError)
  })
})

describe('applyExplicitCacheUpstreamCredentials', () => {
  it('writes the switch and optional normalized TTL cap', () => {
    const credentials: Record<string, unknown> = {}
    applyExplicitCacheUpstreamCredentials(credentials, true, '1800')
    expect(credentials).toEqual({
      explicit_cache_upstream: true,
      explicit_cache_upstream_max_ttl_seconds: 1800
    })

    applyExplicitCacheUpstreamCredentials(credentials, true, '')
    expect(credentials).toEqual({ explicit_cache_upstream: true })
  })

  it('removes both settings when disabled without disturbing other credentials', () => {
    const credentials: Record<string, unknown> = {
      api_key: 'k',
      base_url: 'https://gateway.example.com',
      other: 'kept',
      explicit_cache_upstream: true,
      explicit_cache_upstream_max_ttl_seconds: 600
    }
    applyExplicitCacheUpstreamCredentials(credentials, false, 600)
    expect(credentials).toEqual({
      api_key: 'k',
      base_url: 'https://gateway.example.com',
      other: 'kept'
    })
  })
})
