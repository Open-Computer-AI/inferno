/** One offline withdrawal and the key that makes retries non-duplicating. */
export interface AffiliateWithdrawOperation {
  userId: number
  amount: number
  key: string
  storageKey: string | null
  outcomeUncertain: boolean
}

const SCALE = 1e8
const pendingKeys = new Map<string, string>()

export function canDismissAffiliateWithdraw(submitting: boolean, outcomeUncertain: boolean): boolean {
  return !submitting && !outcomeUncertain
}

function currentAdminId(): number | null {
  try {
    const user = JSON.parse(globalThis.localStorage?.getItem('auth_user') ?? 'null') as { id?: unknown } | null
    return typeof user?.id === 'number' && Number.isSafeInteger(user.id) && user.id > 0 ? user.id : null
  } catch {
    return null
  }
}

function storedKey(key: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(key) ?? null
  } catch {
    return null
  }
}

function saveKey(storageKey: string, key: string | null) {
  try {
    if (key) globalThis.sessionStorage?.setItem(storageKey, key)
    else globalThis.sessionStorage?.removeItem(storageKey)
  } catch {
    // Keep the in-memory key so retries in this page remain safe.
  }
}

export function prepareAffiliateWithdrawOperation(userId: number, amount: number): AffiliateWithdrawOperation {
  const normalizedAmount = Math.round(amount * SCALE) / SCALE
  const adminId = currentAdminId()
  const storageKey = adminId
    ? `sub2api:admin:affiliate-withdraw:${adminId}:${userId}:${normalizedAmount}`
    : null
  const previousKey = storageKey ? pendingKeys.get(storageKey) ?? storedKey(storageKey) : null
  const key = previousKey ?? `affiliate-withdraw-${adminId ?? 'unknown'}-${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`}`
  if (storageKey) {
    pendingKeys.set(storageKey, key)
    saveKey(storageKey, key)
  }
  return { userId, amount: normalizedAmount, key, storageKey, outcomeUncertain: previousKey !== null }
}

export function completeAffiliateWithdrawOperation(operation: AffiliateWithdrawOperation) {
  if (!operation.storageKey) return
  pendingKeys.delete(operation.storageKey)
  saveKey(operation.storageKey, null)
}
