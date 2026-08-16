export interface RazorpayCheckoutResult {
  razorpay_order_id?: string
  razorpay_subscription_id?: string
  razorpay_payment_id: string
  razorpay_signature: string
}

export interface RazorpayFailureResult {
  error?: {
    description?: string
    reason?: string
    code?: string
  }
}

export interface RazorpayCheckoutOptions {
  key: string
  amount?: number
  currency: string
  name: string
  description: string
  order_id?: string
  subscription_id?: string
  prefill?: { name?: string; email?: string }
  theme?: { color?: string }
  retry?: { enabled?: boolean; max_count?: number }
  handler: (response: RazorpayCheckoutResult) => void | Promise<void>
  modal?: { ondismiss?: () => void }
}

export interface RazorpayCheckoutInstance {
  open(): void
  on(event: 'payment.failed', handler: (response: RazorpayFailureResult) => void): void
}

interface RazorpayConstructor {
  new (options: RazorpayCheckoutOptions): RazorpayCheckoutInstance
}

declare global {
  interface Window {
    Razorpay?: RazorpayConstructor
  }
}

const RAZORPAY_CHECKOUT_SCRIPT = 'https://checkout.razorpay.com/v1/checkout.js'
let checkoutScriptPromise: Promise<void> | undefined

function loadCheckoutScript(): Promise<void> {
  if (typeof window === 'undefined') {
    return Promise.reject(new Error('Razorpay Checkout is only available in a browser'))
  }
  if (window.Razorpay) return Promise.resolve()
  if (checkoutScriptPromise) return checkoutScriptPromise

  checkoutScriptPromise = new Promise<void>((resolve, reject) => {
    const existing = document.querySelector<HTMLScriptElement>(`script[src="${RAZORPAY_CHECKOUT_SCRIPT}"]`)
    const script = existing || document.createElement('script')
    const cleanup = () => {
      script.removeEventListener('load', handleLoad)
      script.removeEventListener('error', handleError)
    }
    const handleLoad = () => {
      cleanup()
      if (window.Razorpay) resolve()
      else reject(new Error('Razorpay Checkout loaded without its API'))
    }
    const handleError = () => {
      cleanup()
      reject(new Error('Razorpay Checkout script failed to load'))
    }

    script.addEventListener('load', handleLoad, { once: true })
    script.addEventListener('error', handleError, { once: true })
    if (!existing) {
      script.src = RAZORPAY_CHECKOUT_SCRIPT
      script.async = true
      document.head.appendChild(script)
    }
  }).catch((error) => {
    checkoutScriptPromise = undefined
    throw error
  })

  return checkoutScriptPromise
}

export async function openRazorpayCheckout(
  options: RazorpayCheckoutOptions,
  onFailure?: (response: RazorpayFailureResult) => void,
): Promise<void> {
  await loadCheckoutScript()
  if (!window.Razorpay) throw new Error('Razorpay Checkout is unavailable')
  const checkout = new window.Razorpay(options)
  if (onFailure) checkout.on('payment.failed', onFailure)
  checkout.open()
}
