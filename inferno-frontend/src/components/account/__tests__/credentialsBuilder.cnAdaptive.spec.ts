import { describe, expect, it } from 'vitest'

import { cnBalanceCellVisible, cnQuotaCellVisible, cnSupportsNativeResponses, defaultCNAdaptiveBaseUrls, defaultCNBaseUrl, providerUsageBaseUrl } from '../credentialsBuilder'

describe('cnSupportsNativeResponses', () => {
  it('is true for DeepSeek, Kimi, and MiniMax', () => {
    expect(cnSupportsNativeResponses('deepseek')).toBe(true)
    expect(cnSupportsNativeResponses('kimi')).toBe(true)
    expect(cnSupportsNativeResponses('minimax')).toBe(true)
    expect(cnSupportsNativeResponses('command_code')).toBe(true)
    expect(cnSupportsNativeResponses('zhipu')).toBe(false)
    expect(cnSupportsNativeResponses('openai')).toBe(false)
  })
})

describe('defaultCNAdaptiveBaseUrls', () => {
  it('resolves Kimi endpoints by account mode', () => {
    expect(defaultCNAdaptiveBaseUrls('kimi', 'payg')).toEqual({
      chat_completions: 'https://api.moonshot.cn/v1',
      anthropic: 'https://api.moonshot.cn/anthropic',
      responses: 'https://api.moonshot.cn/v1'
    })
    expect(defaultCNAdaptiveBaseUrls('kimi', 'coding')).toEqual({
      chat_completions: 'https://api.kimi.com/coding/v1',
      anthropic: 'https://api.kimi.com/coding',
      responses: 'https://api.kimi.com/coding/v1'
    })
  })

  it('resolves GLM endpoints by account mode', () => {
    expect(defaultCNAdaptiveBaseUrls('zhipu', 'payg')).toEqual({
      chat_completions: 'https://open.bigmodel.cn/api/paas/v4',
      anthropic: 'https://open.bigmodel.cn/api/anthropic',
      responses: ''
    })
    expect(defaultCNAdaptiveBaseUrls('zhipu', 'coding')).toEqual({
      chat_completions: 'https://open.bigmodel.cn/api/coding/paas/v4',
      anthropic: 'https://open.bigmodel.cn/api/anthropic',
      responses: ''
    })
  })

  it('includes all three native DeepSeek endpoints', () => {
    expect(defaultCNAdaptiveBaseUrls('deepseek', 'payg')).toEqual({
      chat_completions: 'https://api.deepseek.com',
      anthropic: 'https://api.deepseek.com/anthropic',
      responses: 'https://api.deepseek.com'
    })
  })

  it('uses the same MiniMax endpoints for payg and coding modes', () => {
    const expected = {
      chat_completions: 'https://api.minimaxi.com/v1',
      anthropic: 'https://api.minimaxi.com/anthropic',
      responses: 'https://api.minimaxi.com/v1'
    }
    expect(defaultCNAdaptiveBaseUrls('minimax', 'payg')).toEqual(expected)
    expect(defaultCNAdaptiveBaseUrls('minimax', 'coding')).toEqual(expected)
  })
})


describe('Command Code and Cline provider support', () => {
  const account = (platform: string, credentials: Record<string, unknown>, type = 'apikey') => ({
    platform,
    type,
    credentials
  })

  it('uses pinned official defaults and records Cline as fixed Chat Completions only', () => {
    expect(defaultCNAdaptiveBaseUrls('command_code', 'payg')).toEqual({
      chat_completions: 'https://api.commandcode.ai/provider/v1',
      anthropic: 'https://api.commandcode.ai/provider',
      responses: 'https://api.commandcode.ai/provider/v1'
    })
    expect(defaultCNBaseUrl('cline', 'payg', 'chat_completions')).toBe('https://api.cline.bot/api/v1')
    expect(defaultCNBaseUrl('cline', 'payg', 'responses')).toBe('')
    expect(defaultCNBaseUrl('cline', 'payg', 'anthropic')).toBe('')
  })

  it.each([
    ['http scheme', 'https://api.commandcode.ai/v1', 'http://api.commandcode.ai/v1', false],
    ['malformed URL', 'https://api.commandcode.ai/v1', 'https://[broken', false],
    ['relative URL', 'https://api.commandcode.ai/v1', '/v1', false],
    ['lookalike hostname', 'https://api.commandcode.ai/v1', 'https://api.commandcode.ai.evil.test/v1', false],
    ['userinfo targeting evil host', 'https://api.commandcode.ai/v1', 'https://api.commandcode.ai@evil.test/v1', false],
    ['uppercase official host', 'https://api.commandcode.ai/v1', 'https://API.COMMANDCODE.AI/v1', true],
    ['non-default HTTPS port accepted by backend host policy', 'https://api.commandcode.ai/v1', 'https://api.commandcode.ai:8443/v1', true],
  ])('matches provider account eligibility for %s', (_name, _fallback, baseUrl, eligible) => {
    const candidate = account('command_code', { base_url: baseUrl })
    expect(cnQuotaCellVisible('command_code', 'payg', candidate)).toBe(eligible)
    expect(cnBalanceCellVisible('command_code', 'payg', candidate)).toBe(eligible)
  })

  it('uses backend-effective URL selection for active, stale, and disagreeing adaptive maps', () => {
    const activeOfficialMap = account('command_code', {
      api_protocol: 'adaptive',
      base_url: 'https://relay.example/v1',
      api_base_urls: { chat_completions: 'https://api.commandcode.ai/provider/v1' }
    })
    const activeRelayMap = account('command_code', {
      api_protocol: 'adaptive',
      base_url: 'https://api.commandcode.ai/provider/v1',
      api_base_urls: { chat_completions: 'https://relay.example/v1' }
    })
    const inactiveOfficialMap = account('command_code', {
      api_protocol: 'responses',
      base_url: 'https://relay.example/v1',
      api_base_urls: { chat_completions: 'https://api.commandcode.ai/provider/v1' }
    })
    const inactiveRelayMap = account('command_code', {
      api_protocol: 'responses',
      base_url: 'https://api.commandcode.ai/provider/v1',
      api_base_urls: { chat_completions: 'https://relay.example/v1' }
    })
    expect(providerUsageBaseUrl('command_code', activeOfficialMap)).toBe('https://api.commandcode.ai/provider/v1')
    expect(cnQuotaCellVisible('command_code', 'payg', activeOfficialMap)).toBe(true)
    expect(cnQuotaCellVisible('command_code', 'payg', activeRelayMap)).toBe(false)
    expect(cnQuotaCellVisible('command_code', 'payg', inactiveOfficialMap)).toBe(false)
    expect(cnQuotaCellVisible('command_code', 'payg', inactiveRelayMap)).toBe(true)
  })

  it('ignores stale api_base_urls for Cline and defaults missing URLs to the backend profile', () => {
    const relay = account('cline', {
      base_url: 'https://relay.example/v1',
      api_base_urls: { chat_completions: 'https://api.cline.bot/api/v1' }
    })
    const official = account('cline', {
      base_url: 'https://api.cline.bot/api/v1',
      api_base_urls: { chat_completions: 'https://relay.example/v1' }
    })
    expect(cnQuotaCellVisible('cline', 'payg', relay)).toBe(false)
    expect(cnQuotaCellVisible('cline', 'payg', official)).toBe(true)
    expect(cnQuotaCellVisible('cline', 'payg', account('cline', {}))).toBe(true)
    expect(cnQuotaCellVisible('cline', 'payg', account('cline', {}, 'oauth'))).toBe(false)
  })
})
