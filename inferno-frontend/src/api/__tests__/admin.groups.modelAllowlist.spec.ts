import { describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { getModelAllowlistCandidates } from '../admin/groups'

describe('admin groups model allowlist API', () => {
  it('loads platform candidates for a create request', async () => {
    get.mockResolvedValueOnce({ data: { models: ['gpt-5.5', 'gpt-5.4'] } })

    await expect(getModelAllowlistCandidates(0, 'openai')).resolves.toEqual([
      'gpt-5.5',
      'gpt-5.4',
    ])
    expect(get).toHaveBeenCalledWith('/admin/groups/0/model-allowlist-candidates', {
      params: { platform: 'openai' },
    })
  })

  it('returns an empty list when the response omits models', async () => {
    get.mockResolvedValueOnce({ data: {} })

    await expect(getModelAllowlistCandidates(42)).resolves.toEqual([])
    expect(get).toHaveBeenCalledWith('/admin/groups/42/model-allowlist-candidates', {
      params: undefined,
    })
  })
})
