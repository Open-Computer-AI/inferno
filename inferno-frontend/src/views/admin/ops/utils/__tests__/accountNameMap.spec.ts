import { beforeEach, describe, expect, it, vi } from 'vitest'
import { accountsAPI } from '@/api/admin/accounts'
import { getAccountNameMap, resetAccountNameCacheForTests } from '../accountNameMap'

vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { list: vi.fn() } }))
const list = vi.mocked(accountsAPI.list)

beforeEach(() => {
  vi.clearAllMocks()
  resetAccountNameCacheForTests()
})

describe('accountNameMap', () => {
  it('loads all lite pages and caches the completed map', async () => {
    list.mockResolvedValueOnce({ items: [{ id: 1, name: 'one' }, { id: 2, name: 'two' }], total: 3, page: 1, page_size: 2, pages: 2 } as never)
    list.mockResolvedValueOnce({ items: [{ id: 3, name: 'three' }], total: 3, page: 2, page_size: 2, pages: 2 } as never)
    const first = await getAccountNameMap()
    const second = await getAccountNameMap()
    expect([...first.entries()]).toEqual([[1, 'one'], [2, 'two'], [3, 'three']])
    expect(second).toBe(first)
    expect(list).toHaveBeenCalledTimes(2)
    expect(list).toHaveBeenNthCalledWith(1, 1, 1000, { lite: '1', include_scheduler_score: '0' })
    expect(list).toHaveBeenNthCalledWith(2, 2, 1000, { lite: '1', include_scheduler_score: '0' })
  })

  it('deduplicates concurrent requests', async () => {
    let resolve!: (value: never) => void
    list.mockReturnValueOnce(new Promise(resolveRequest => { resolve = resolveRequest as typeof resolve }))
    const a = getAccountNameMap()
    const b = getAccountNameMap(true)
    resolve({ items: [{ id: 7, name: 'shared' }], total: 1, page: 1, page_size: 1000, pages: 1 } as never)
    expect(await a).toBe(await b)
    expect(list).toHaveBeenCalledTimes(1)
  })

  it('does not cache failures and can retry', async () => {
    list.mockRejectedValueOnce(new Error('offline'))
    await expect(getAccountNameMap()).rejects.toThrow('offline')
    list.mockResolvedValueOnce({ items: [{ id: 9, name: 'retry' }], total: 1, page: 1, page_size: 1000, pages: 1 } as never)
    expect((await getAccountNameMap()).get(9)).toBe('retry')
    expect(list).toHaveBeenCalledTimes(2)
  })
})
