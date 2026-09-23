import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AffiliateOfflineWithdrawDialog from '../AffiliateOfflineWithdrawDialog.vue'

const { lookupUsers, getUserOverview, withdrawUserQuota, showError, showSuccess } = vi.hoisted(() => ({
  lookupUsers: vi.fn(),
  getUserOverview: vi.fn(),
  withdrawUserQuota: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin/affiliates', () => ({
  affiliatesAPI: { lookupUsers, getUserOverview, withdrawUserQuota },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key} ${JSON.stringify(params)}` : key,
    }),
  }
})

const BaseDialogStub = {
  props: ['show', 'title'],
  emits: ['close'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
}

function mountDialog() {
  return mount(AffiliateOfflineWithdrawDialog, {
    props: { show: true },
    global: { stubs: { BaseDialog: BaseDialogStub } },
  })
}

function overviewWithQuota(availableQuota: number) {
  return {
    user_id: 42,
    email: 'inviter@example.com',
    username: 'inviter',
    aff_code: 'INVITER',
    rebate_rate_percent: 20,
    invited_count: 3,
    rebated_invitee_count: 2,
    available_quota: availableQuota,
    history_quota: 50,
  }
}

function withdrawResponse(amount: number, availableAfter: number, replayed = false) {
  return {
    result: {
      ledger_id: 9,
      user_id: 42,
      amount,
      available_quota_after: availableAfter,
      frozen_quota_after: 0,
      history_quota_after: 50,
    },
    replayed,
  }
}

function buttonWithText(wrapper: VueWrapper, ...labels: string[]) {
  return wrapper.findAll('button').find((button) => labels.includes(button.text().trim()))
}

function amountInput(wrapper: VueWrapper) {
  return wrapper.get('input[type="number"]')
}

function submitButton(wrapper: VueWrapper) {
  return buttonWithText(
    wrapper,
    'admin.affiliates.withdraw.submit',
    'admin.affiliates.withdraw.retry',
  )
}

async function pickUser(wrapper: VueWrapper, availableQuota: number) {
  lookupUsers.mockResolvedValue([{ id: 42, email: 'inviter@example.com', username: 'inviter' }])
  getUserOverview.mockResolvedValue(overviewWithQuota(availableQuota))
  await wrapper.get('input[type="text"]').setValue('inviter')
  vi.advanceTimersByTime(300)
  await flushPromises()

  const option = wrapper.findAll('button').find((button) => button.text().includes('inviter@example.com'))
  expect(option).toBeDefined()
  await option!.trigger('click')
  await flushPromises()
}

describe('AffiliateOfflineWithdrawDialog', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.clearAllMocks()
    localStorage.clear()
    sessionStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 17 }))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('records the exact available quota as the withdrawal amount', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 12.34567891)

    expect(lookupUsers).toHaveBeenCalledWith('inviter')
    expect(getUserOverview).toHaveBeenCalledWith(42)
    expect(wrapper.get('span.font-semibold').text()).toBe('$12.34567891')

    await buttonWithText(wrapper, 'admin.affiliates.withdraw.fillAll')!.trigger('click')
    expect((amountInput(wrapper).element as HTMLInputElement).value).toBe('12.34567891')

    withdrawUserQuota.mockResolvedValue(withdrawResponse(12.34567891, 0))
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(withdrawUserQuota).toHaveBeenCalledWith(42, { amount: 12.34567891 }, expect.stringMatching(/^affiliate-withdraw-/))
    expect(showSuccess).toHaveBeenCalledWith(
      'admin.affiliates.withdraw.success {"amount":"$12.34567891","remaining":"$0.00"}',
    )
    expect(wrapper.emitted('success')?.[0]?.[0]).toMatchObject({ ledger_id: 9 })
  })

  it('blocks amounts above the available quota', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)

    await amountInput(wrapper).setValue('10.00000001')

    expect(wrapper.get('p.input-error-text').text()).toBe('admin.affiliates.withdraw.amountExceeds')
    expect(submitButton(wrapper)!.attributes('disabled')).toBeDefined()
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()
    expect(withdrawUserQuota).not.toHaveBeenCalled()
  })

  it('treats a user without an affiliate profile as having no available quota', async () => {
    lookupUsers.mockResolvedValue([{ id: 7, email: 'fresh@example.com', username: 'fresh' }])
    getUserOverview.mockRejectedValue({ reason: 'USER_NOT_FOUND', message: 'user not found' })
    const wrapper = mountDialog()

    await wrapper.get('input[type="text"]').setValue('fresh')
    vi.advanceTimersByTime(300)
    await flushPromises()
    const option = wrapper.findAll('button').find((button) => button.text().includes('fresh@example.com'))
    expect(option).toBeDefined()
    await option!.trigger('click')
    await flushPromises()

    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('fresh@example.com')
    expect(wrapper.get('span.font-semibold').text()).toBe('$0.00')
    expect(buttonWithText(wrapper, 'admin.affiliates.withdraw.fillAll')!.attributes('disabled')).toBeDefined()

    await amountInput(wrapper).setValue('1')
    expect(wrapper.get('p.input-error-text').text()).toBe('admin.affiliates.withdraw.amountExceeds')
    expect(submitButton(wrapper)!.attributes('disabled')).toBeDefined()
  })

  it('shows the API error and reloads the available quota when recording fails', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)

    withdrawUserQuota.mockRejectedValue({ status: 400, reason: 'AFFILIATE_QUOTA_INSUFFICIENT', message: 'insufficient' })
    await amountInput(wrapper).setValue('5')
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.affiliates.errors.AFFILIATE_QUOTA_INSUFFICIENT {}')
    expect(getUserOverview).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('success')).toBeUndefined()
  })

  it.each([
    ['a network error', { status: 0, message: 'Network error. Please check your connection.' }],
    ['a gateway error', { status: 502, message: 'Bad Gateway' }],
    ['a request timeout', { status: 408, message: 'Request Timeout' }],
  ])('retries the same registration with the same key after %s', async (_label, failure) => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)
    await buttonWithText(wrapper, 'admin.affiliates.withdraw.fillAll')!.trigger('click')

    withdrawUserQuota.mockRejectedValueOnce(failure)
    getUserOverview.mockResolvedValue(overviewWithQuota(0))
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledTimes(1)
    expect(wrapper.get('span.font-semibold').text()).toBe('$0.00')
    expect(wrapper.find('div.border-blue-200').text()).toBe('admin.affiliates.withdraw.uncertainHint')
    expect(amountInput(wrapper).attributes('disabled')).toBeDefined()
    expect(buttonWithText(wrapper, 'admin.affiliates.withdraw.fillAll')!.attributes('disabled')).toBeDefined()
    const clearUserButton = wrapper.findAll('button').find((button) => button.text().trim() === '×')
    expect(clearUserButton?.attributes('disabled')).toBeDefined()
    expect(wrapper.find('p.input-error-text').exists()).toBe(false)
    expect(submitButton(wrapper)!.attributes('disabled')).toBeUndefined()

    withdrawUserQuota.mockResolvedValueOnce(withdrawResponse(10, 0, true))
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(withdrawUserQuota).toHaveBeenCalledTimes(2)
    const [firstCall, retryCall] = withdrawUserQuota.mock.calls
    expect(firstCall).toEqual([42, { amount: 10 }, expect.stringMatching(/^affiliate-withdraw-/)])
    expect(retryCall).toEqual(firstCall)
    expect(showSuccess).toHaveBeenCalledWith(
      'admin.affiliates.withdraw.replayed {"amount":"$10.00","remaining":"$0.00"}',
    )
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('starts a new registration with a fresh key after a definite rejection', async () => {
    const wrapper = mountDialog()
    await pickUser(wrapper, 10)
    await amountInput(wrapper).setValue('6')

    withdrawUserQuota.mockRejectedValueOnce({ status: 400, reason: 'AFFILIATE_QUOTA_INSUFFICIENT', message: 'insufficient' })
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(wrapper.find('div.border-blue-200').exists()).toBe(false)
    expect(amountInput(wrapper).attributes('disabled')).toBeUndefined()

    await amountInput(wrapper).setValue('5')
    withdrawUserQuota.mockResolvedValueOnce(withdrawResponse(5, 5))
    await submitButton(wrapper)!.trigger('click')
    await flushPromises()

    expect(withdrawUserQuota).toHaveBeenCalledTimes(2)
    const [rejectedCall, nextCall] = withdrawUserQuota.mock.calls
    expect(nextCall[1]).toEqual({ amount: 5 })
    expect(nextCall[2]).not.toBe(rejectedCall[2])
    expect(showSuccess).toHaveBeenCalledWith('admin.affiliates.withdraw.success {"amount":"$5.00","remaining":"$5.00"}')
  })
})
