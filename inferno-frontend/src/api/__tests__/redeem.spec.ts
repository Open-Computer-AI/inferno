import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getHistory } from '../redeem'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { get } }))

describe('redemption history pagination', () => {
  beforeEach(() => vi.clearAllMocks())

  it.each([
    [undefined, undefined, 1, 20],
    [3, 50, 3, 50]
  ])('requests page %s with page size %s', async (page, pageSize, expectedPage, expectedSize) => {
    const response = {
      items: [],
      total: 105,
      page: expectedPage,
      page_size: expectedSize,
      pages: 6
    }
    get.mockResolvedValue({ data: response })

    expect(await getHistory(page, pageSize)).toEqual(response)
    expect(get).toHaveBeenCalledWith('/redeem/history', {
      params: { page: expectedPage, page_size: expectedSize }
    })
  })
})
