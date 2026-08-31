/**
 * Upstream's spec, restored.
 *
 * The client dropped its browser-timezone argument when the backend moved to a
 * server-side daily rollup (cb7b03795). Sending one again would make the two
 * sides disagree about which day a cost belongs to -- and it would still
 * type-check and still render, because the shape does not change.
 *
 * Our port took the client change and left the spec behind; the field was
 * arriving in the browser and being discarded. Found by
 * scripts/behaviour-parity.mjs.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { getUsageSummary } from '@/api/admin/groups'

describe('admin group usage summary API', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: [] })
  })

  it('does not send browser timezone parameters', async () => {
    const summary = [{ group_id: 1, today_cost: 1.25, yesterday_cost: 2.5, total_cost: 9.75 }]
    get.mockResolvedValue({ data: summary })

    await expect(getUsageSummary()).resolves.toEqual(summary)
    expect(get).toHaveBeenCalledWith('/admin/groups/usage-summary')
  })

  it('returns yesterday_cost through to the caller', async () => {
    // Ours: the field the backend rollup added was reaching the browser and
    // being dropped by a local type before it could render.
    get.mockResolvedValue({ data: [{ group_id: 2, today_cost: 0, yesterday_cost: 4.5, total_cost: 4.5 }] })
    const [row] = await getUsageSummary()
    expect(row.yesterday_cost).toBe(4.5)
  })
})
