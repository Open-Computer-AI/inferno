import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

enableAutoUnmount(afterEach)
afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

const routeState = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}))
const routerPush = vi.hoisted(() => vi.fn())
const getOrder = vi.hoisted(() => vi.fn())
const paymentStore = vi.hoisted(() => ({
  config: { stripe_publishable_key: 'pk_test' } as { stripe_publishable_key?: string },
  fetchConfig: vi.fn(),
  pollOrderStatus: vi.fn(),
}))
const loadStripe = vi.hoisted(() => vi.fn())
const stripeElements = vi.hoisted(() => ({
  create: vi.fn(),
}))
const stripePaymentElement = vi.hoisted(() => ({
  mount: vi.fn(),
  on: vi.fn(),
}))
const stripeInstance = vi.hoisted(() => ({
  elements: vi.fn(),
  confirmPayment: vi.fn(),
  confirmAlipayPayment: vi.fn(),
  confirmWechatPayPayment: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
    useRouter: () => ({ push: routerPush }),
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => paymentStore,
}))

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getOrder,
  },
}))

vi.mock('@stripe/stripe-js/pure', () => ({
  loadStripe,
}))

import StripePaymentView from '../StripePaymentView.vue'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { PaymentOrder } from '@/types/payment'

function orderFactory(overrides: Partial<PaymentOrder> = {}): PaymentOrder {
  return {
    id: 42,
    user_id: 7,
    amount: 100,
    pay_amount: 103,
    currency: 'CNY',
    fee_rate: 0.03,
    payment_type: 'stripe',
    out_trade_no: 'sub2_stripe_42',
    status: 'PENDING',
    order_type: 'balance',
    created_at: '2026-04-20T12:00:00Z',
    expires_at: '2026-04-20T12:30:00Z',
    refund_amount: 0,
    ...overrides,
  }
}

function mountView() {
  return shallowMount(StripePaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: true,
      },
    },
  })
}

function viewState(wrapper: ReturnType<typeof mountView>) {
  return wrapper.vm as unknown as {
    redirecting: boolean
    stripeError: string
    stripeSuccess: boolean
    stripeSubmitting: boolean
    stripeReady: boolean
    wechatQrUrl: string
  }
}

describe('StripePaymentView', () => {
  beforeEach(() => {
    routeState.query = {
      order_id: '42',
      client_secret: 'pi_secret_42',
    }
    routerPush.mockReset()
    getOrder.mockReset().mockResolvedValue({ data: orderFactory() })
    paymentStore.config = { stripe_publishable_key: 'pk_test' }
    paymentStore.fetchConfig.mockReset().mockResolvedValue(undefined)
    paymentStore.pollOrderStatus.mockReset()
    loadStripe.mockReset().mockResolvedValue(stripeInstance)
    stripeElements.create.mockReset().mockReturnValue(stripePaymentElement)
    stripePaymentElement.mount.mockReset()
    stripePaymentElement.on.mockReset().mockImplementation((event: string, callback: () => void) => {
      if (event === 'ready') callback()
    })
    stripeInstance.elements.mockReset().mockReturnValue(stripeElements)
    stripeInstance.confirmPayment.mockReset()
    stripeInstance.confirmAlipayPayment.mockReset()
    stripeInstance.confirmWechatPayPayment.mockReset()
    window.localStorage.clear()
  })

  it.each(['alipay', 'wechat_pay', ''])('does not start %s payment after leaving during SDK loading', async (method) => {
    routeState.query.method = method
    let resolveSDK!: (value: typeof stripeInstance) => void
    loadStripe.mockReturnValueOnce(new Promise((resolve) => { resolveSDK = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    expect(loadStripe).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    resolveSDK(stripeInstance)
    await flushPromises()
    await flushPromises()
    expect(stripeInstance.confirmAlipayPayment).not.toHaveBeenCalled()
    expect(stripeInstance.confirmWechatPayPayment).not.toHaveBeenCalled()
    expect(stripeInstance.elements).not.toHaveBeenCalled()
  })

  it('ignores a late Alipay result after leaving', async () => {
    routeState.query.method = 'alipay'
    let resolvePayment!: (value: unknown) => void
    stripeInstance.confirmAlipayPayment.mockReturnValueOnce(new Promise((resolve) => { resolvePayment = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    expect(stripeInstance.confirmAlipayPayment).toHaveBeenCalledTimes(1)
    const state = viewState(wrapper)
    wrapper.unmount()
    resolvePayment({ error: { message: 'late alipay error' } })
    await flushPromises()
    expect(state.redirecting).toBe(true)
    expect(state.stripeError).toBe('')
    expect(routerPush).not.toHaveBeenCalled()
  })

  it('ignores a late WeChat result without rendering QR or restarting polling', async () => {
    vi.useFakeTimers()
    routeState.query.method = 'wechat_pay'
    let resolvePayment!: (value: unknown) => void
    stripeInstance.confirmWechatPayPayment.mockReturnValueOnce(new Promise((resolve) => { resolvePayment = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    expect(stripeInstance.confirmWechatPayPayment).toHaveBeenCalledTimes(1)
    const state = viewState(wrapper)
    wrapper.unmount()
    resolvePayment({ paymentIntent: { status: 'requires_action', next_action: {
      wechat_pay_display_qr_code: { image_data_url: 'data:image/png;base64,late' }
    } } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    expect(state.wechatQrUrl).toBe('')
    expect(state.stripeSuccess).toBe(false)
    expect(paymentStore.pollOrderStatus).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([false, true])('ignores a late generic confirmation after leaving (reject=%s)', async (reject) => {
    let resolvePayment!: (value: unknown) => void
    let rejectPayment!: (reason?: unknown) => void
    stripeInstance.confirmPayment.mockReturnValueOnce(new Promise((resolve, rejectPromise) => {
      resolvePayment = resolve
      rejectPayment = rejectPromise
    }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    await wrapper.find('button.btn-stripe').trigger('click')
    expect(stripeInstance.confirmPayment).toHaveBeenCalledTimes(1)
    const state = viewState(wrapper)
    wrapper.unmount()
    if (reject) rejectPayment(new Error('late generic error'))
    else resolvePayment({ error: { message: 'late generic error' } })
    await flushPromises()
    expect(state.stripeError).toBe('')
    expect(state.stripeSuccess).toBe(false)
    expect(state.stripeSubmitting).toBe(true)
    expect(routerPush).not.toHaveBeenCalled()
  })

  it('ignores a late Payment Element ready callback after leaving', async () => {
    let readyCallback!: () => void
    stripePaymentElement.on.mockImplementation((_event: string, callback: () => void) => {
      readyCallback = callback
    })
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    expect(stripePaymentElement.mount).toHaveBeenCalledTimes(1)
    const state = viewState(wrapper)
    wrapper.unmount()
    readyCallback()
    expect(state.stripeReady).toBe(false)
  })

  it('ignores a delayed close callback after leaving', async () => {
    routeState.query.method = 'wechat_pay'
    let resolvePayment!: (value: unknown) => void
    stripeInstance.confirmWechatPayPayment.mockReturnValueOnce(new Promise((resolve) => { resolvePayment = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    let delayedClose!: () => void
    const setTimeoutSpy = vi.spyOn(globalThis, 'setTimeout').mockImplementation(((handler) => {
      delayedClose = handler as () => void
      return 1 as unknown as ReturnType<typeof setTimeout>
    }) as typeof setTimeout)
    resolvePayment({ paymentIntent: { status: 'succeeded' } })
    await flushPromises()
    expect(delayedClose).toBeTypeOf('function')
    setTimeoutSpy.mockRestore()
    wrapper.unmount()
    delayedClose()
    expect(routerPush).not.toHaveBeenCalled()
  })

  it('does not restart polling when a WeChat QR result arrives after leaving', async () => {
    vi.useFakeTimers()
    routeState.query.method = 'wechat_pay'
    let resolvePayment!: (value: unknown) => void
    stripeInstance.confirmWechatPayPayment.mockReturnValueOnce(new Promise((resolve) => { resolvePayment = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    expect(stripeInstance.confirmWechatPayPayment).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    resolvePayment({ paymentIntent: { status: 'requires_action', next_action: {
      wechat_pay_display_qr_code: { image_data_url: 'data:image/png;base64,test' }
    } } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    expect(paymentStore.pollOrderStatus).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each([false, true])('handles a completed poll with page unmounted=%s', async (unmounted) => {
    vi.useFakeTimers()
    routeState.query.method = 'wechat_pay'
    stripeInstance.confirmWechatPayPayment.mockResolvedValueOnce({ paymentIntent: {
      status: 'requires_action', next_action: { wechat_pay_display_qr_code: { image_data_url: 'data:image/png;base64,test' } }
    } })
    let resolvePoll!: (value: PaymentOrder) => void
    paymentStore.pollOrderStatus.mockReturnValueOnce(new Promise((resolve) => { resolvePoll = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(3000)
    expect(paymentStore.pollOrderStatus).toHaveBeenCalledTimes(1)
    const state = viewState(wrapper)
    if (unmounted) wrapper.unmount()
    resolvePoll(orderFactory({ status: 'PAID' }))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    if (unmounted) {
      expect(state.stripeSuccess).toBe(false)
      expect(state.wechatQrUrl).toBe('data:image/png;base64,test')
    }
    expect(routerPush).toHaveBeenCalledTimes(unmounted ? 0 : 1)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('本地恢复快照缺失时使用订单接口返回的 Stripe 币种展示金额', async () => {
    getOrder.mockResolvedValue({
      data: orderFactory({ currency: 'HKD', pay_amount: 103 }),
    })

    const wrapper = mountView()
    await flushPromises()
    await flushPromises()

    expect(getOrder).toHaveBeenCalledWith(42)
    expect(loadStripe).toHaveBeenCalledWith('pk_test')
    expect(wrapper.text()).toContain(formatPaymentAmount(103, 'HKD', 'zh-CN'))
  })
})
