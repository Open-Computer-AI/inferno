import { accountsAPI } from '@/api/admin/accounts'

const ACCOUNT_NAME_PAGE_SIZE = 1000

let accountNameCache: Map<number, string> | null = null
let accountNameRequest: Promise<Map<number, string>> | null = null

export async function getAccountNameMap(force = false): Promise<Map<number, string>> {
  if (!force && accountNameCache) return accountNameCache
  if (accountNameRequest) return accountNameRequest

  const request = (async () => {
    const map = new Map<number, string>()
    let page = 1
    let total = Number.POSITIVE_INFINITY
    while (map.size < total) {
      const res = await accountsAPI.list(page, ACCOUNT_NAME_PAGE_SIZE, {
        lite: '1',
        include_scheduler_score: '0'
      })
      total = res.total || 0
      const items = res.items || []
      if (items.length === 0) break
      for (const item of items) map.set(item.id, item.name)
      page++
    }

    accountNameCache = map
    return map
  })()
  accountNameRequest = request
  try {
    return await request
  } finally {
    if (accountNameRequest === request) accountNameRequest = null
  }
}

export function resetAccountNameCacheForTests() {
  accountNameCache = null
  accountNameRequest = null
}
