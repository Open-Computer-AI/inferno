import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Needed before importing client/oauth -- mirrors client.spec.ts's own setup.
vi.mock('@/i18n', () => ({
  getLocale: () => 'en'
}))

const PARAMS = {
  response_type: 'code',
  client_id: 'agent:abc123',
  redirect_uri: 'https://agent.example.com/auth/callback',
  scope: 'agent_dashboard:access',
  state: 'csrf-state',
  code_challenge: 'A'.repeat(43),
  code_challenge_method: 'S256'
}

describe('checkAuthorize / decideAuthorize (F7)', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('reports consent_required on a 202', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 202,
      data: '',
      headers: {},
      config: {},
      statusText: 'Accepted'
    })

    const result = await checkAuthorize(PARAMS)
    expect(result).toEqual({ kind: 'consent_required' })
  })

  it('reports the target URL when the body is a genuine absolute https: URL', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')
    const target = 'https://agent.example.com/auth/callback?code=abc&state=csrf-state'
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: target,
      headers: {},
      config: {},
      statusText: 'OK'
    })

    const result = await checkAuthorize(PARAMS)
    expect(result).toEqual({ kind: 'redirect', url: target })
  })

  // The concrete failure mode review found: with no /oauth vite proxy
  // entry, the dev server answers with its own index.html at 200, and
  // that body must never be handed to window.location.replace.
  it('rejects a 200 body that is not a URL at all', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: '<!doctype html><html><head><title>t</title></head></html>',
      headers: {},
      config: {},
      statusText: 'OK'
    })

    await expect(checkAuthorize(PARAMS)).rejects.toThrow()
  })

  it('rejects a 200 body that parses as a URL but is not https:', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: 'javascript:alert(1)',
      headers: {},
      config: {},
      statusText: 'OK'
    })

    await expect(checkAuthorize(PARAMS)).rejects.toThrow()
  })

  it('decideAuthorize also rejects a non-https: body', async () => {
    const { apiClient } = await import('@/api/client')
    const { decideAuthorize } = await import('@/api/oauth')
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: 'not a url',
      headers: {},
      config: {},
      statusText: 'OK'
    })

    await expect(decideAuthorize(PARAMS, 'approve')).rejects.toThrow()
  })

  it('decideAuthorize returns the target URL for a genuine https: body', async () => {
    const { apiClient } = await import('@/api/client')
    const { decideAuthorize } = await import('@/api/oauth')
    const target = 'https://agent.example.com/auth/callback?error=access_denied&state=csrf-state'
    apiClient.defaults.adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: target,
      headers: {},
      config: {},
      statusText: 'OK'
    })

    await expect(decideAuthorize(PARAMS, 'deny')).resolves.toBe(target)
  })
})

describe('rate-limit envelope on a text-response request (M-9)', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('surfaces the 429 envelope message even though responseType is text', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')

    // Exactly what the backend emits on this endpoint's rate-limit arm: the
    // panel {code,message,data} envelope. Because the request sets
    // responseType: 'text', axios never parses it, so it reaches the
    // interceptor as a STRING -- which the old object-only guard discarded,
    // leaving the user with axios's generic "Request failed with status
    // code 429".
    apiClient.defaults.adapter = vi.fn().mockRejectedValue({
      isAxiosError: true,
      config: { url: '/oauth/authorize' },
      response: {
        status: 429,
        data: JSON.stringify({ code: 42901, message: 'Too many authorization requests' }),
        headers: {},
        config: {},
        statusText: 'Too Many Requests'
      }
    })

    await expect(checkAuthorize(PARAMS)).rejects.toMatchObject({
      status: 429,
      code: 42901,
      message: 'Too many authorization requests'
    })
  })

  it('still ignores a non-JSON error body', async () => {
    const { apiClient } = await import('@/api/client')
    const { checkAuthorize } = await import('@/api/oauth')

    apiClient.defaults.adapter = vi.fn().mockRejectedValue({
      isAxiosError: true,
      message: 'Request failed with status code 502',
      config: { url: '/oauth/authorize' },
      response: {
        status: 502,
        data: '<!doctype html><html><body>Bad gateway</body></html>',
        headers: {},
        config: {},
        statusText: 'Bad Gateway'
      }
    })

    await expect(checkAuthorize(PARAMS)).rejects.toMatchObject({
      status: 502,
      message: 'Request failed with status code 502'
    })
  })
})
