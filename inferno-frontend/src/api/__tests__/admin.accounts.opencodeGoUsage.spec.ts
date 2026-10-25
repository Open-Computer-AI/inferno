import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getOpenCodeGoUsage,
  getOpenCodeGoUsageSettings,
  refreshOpenCodeGoUsage,
  setOpenCodeGoUsageAutoRefresh,
  updateOpenCodeGoUsageSettings
} from '@/api/admin/accounts'

const state = {
  account_id: 7,
  eligible: true,
  auto_refresh_enabled: false
}

describe('admin OpenCode Go usage API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('uses the dedicated global settings contract', async () => {
    const settings = { enabled: false, interval_minutes: 60, debounce_minutes: 1 }
    get.mockResolvedValueOnce({ data: settings })
    put.mockResolvedValueOnce({ data: settings })

    await expect(getOpenCodeGoUsageSettings()).resolves.toEqual(settings)
    await expect(updateOpenCodeGoUsageSettings(settings)).resolves.toEqual(settings)

    expect(get).toHaveBeenCalledWith('/admin/accounts/opencode-go-usage/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/opencode-go-usage/settings', settings)
  })

  it('reads account state, toggles auto-refresh and triggers a manual refresh', async () => {
    get.mockResolvedValueOnce({ data: state })
    put.mockResolvedValueOnce({ data: { ...state, auto_refresh_enabled: true } })
    post.mockResolvedValueOnce({ data: state })

    await expect(getOpenCodeGoUsage(7)).resolves.toEqual(state)
    await expect(setOpenCodeGoUsageAutoRefresh(7, true)).resolves.toMatchObject({ auto_refresh_enabled: true })
    await expect(refreshOpenCodeGoUsage(7)).resolves.toEqual(state)

    expect(get).toHaveBeenCalledWith('/admin/accounts/7/opencode-go-usage')
    expect(put).toHaveBeenCalledWith('/admin/accounts/7/opencode-go-usage/auto-refresh', { enabled: true })
    expect(post).toHaveBeenCalledWith('/admin/accounts/7/opencode-go-usage/refresh')
  })

  it('propagates refresh and toggle failures for the admin consumer to handle', async () => {
    const error = new Error('upstream unavailable')
    put.mockRejectedValueOnce(error)
    post.mockRejectedValueOnce(error)

    await expect(setOpenCodeGoUsageAutoRefresh(7, false)).rejects.toBe(error)
    await expect(refreshOpenCodeGoUsage(7)).rejects.toBe(error)
  })
})
