import { describe, expect, it } from 'vitest'
import {
  buildConcurrencyPolicy,
  buildOptionalConcurrencyPolicy,
  getConcurrencyMode,
  isOpenAIAuthModeLiveConcurrencyEligible,
  isOpenAILiveConcurrencyEligible,
  MAX_CONCURRENCY
} from '@/utils/concurrencyPolicy'

describe('concurrency policy contract', () => {
  it.each([
    'personalAccessToken',
    'personal_access_token',
    'codexPAT',
    'agentIdentity',
    'agent_identity'
  ])('rejects OpenAI auth modes without Live concurrency support: %s', (authMode) => {
      expect(isOpenAIAuthModeLiveConcurrencyEligible(authMode)).toBe(false)
    })

  it('keeps ordinary and unknown OpenAI OAuth auth modes eligible', () => {
    expect(isOpenAIAuthModeLiveConcurrencyEligible(undefined)).toBe(true)
    expect(isOpenAIAuthModeLiveConcurrencyEligible('codexSession')).toBe(true)
    expect(isOpenAILiveConcurrencyEligible({ platform: 'openai', type: 'oauth' })).toBe(true)
    expect(isOpenAILiveConcurrencyEligible({
      platform: 'openai',
      type: 'oauth',
      credentials: { auth_mode: 'agentIdentity' }
    })).toBe(false)
  })

  it('serializes one complete split pair and enforces the database integer bound', () => {
    expect(buildConcurrencyPolicy('split', '4', '2')).toEqual({
      mode: 'split',
      live: 4,
      requests: 2
    })
    expect(buildConcurrencyPolicy('split', 1, MAX_CONCURRENCY)).toEqual({
      mode: 'split',
      live: 1,
      requests: MAX_CONCURRENCY
    })
    expect(buildConcurrencyPolicy('split', 0, 2)).toBeNull()
    expect(buildConcurrencyPolicy('split', 2, MAX_CONCURRENCY + 1)).toBeNull()
    expect(buildConcurrencyPolicy('split', 1.5, 2)).toBeNull()
  })

  it('omits the default create policy but keeps an explicit shared reset available', () => {
    expect(buildOptionalConcurrencyPolicy('shared', '', '')).toBeUndefined()
    expect(buildConcurrencyPolicy('shared', '', '')).toEqual({ mode: 'shared' })
  })

  it('recognizes split responses while retaining shared legacy responses', () => {
    expect(getConcurrencyMode({
      concurrency_mode: 'split',
      live_concurrency: 4,
      request_concurrency: 2
    })).toBe('split')
    expect(getConcurrencyMode({
      live_concurrency: 4,
      request_concurrency: 2
    })).toBe('split')
    expect(getConcurrencyMode({
      concurrency_mode: 'shared',
      live_concurrency: null,
      request_concurrency: null
    })).toBe('shared')
  })
})
