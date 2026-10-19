export interface CheckoutPaymentHandoff {
  order: { order_no: string; status: string; expires_at?: string }
  payment: { channel_id: number | null; pay_url: string; [key: string]: unknown }
}

// Keep the click-created tab and payment result across SPA navigation, in memory only.
// Only the matching order may consume either, once.
let reserved: { orderNo: string; tab: Window; checkout?: CheckoutPaymentHandoff } | null = null

export const openPaymentTab = (message: string): Window | null => {
  const tab = window.open('', '_blank')
  if (!tab) return null
  tab.opener = null
  try {
    const doc = tab.document
    doc.title = message
    doc.documentElement.lang = document.documentElement.lang
    const style = doc.createElement('style')
    style.textContent = `
      body { margin: 0; min-height: 100vh; display: grid; place-items: center;
        background: #f8fafc; color: #334155; font: 16px system-ui, sans-serif; }
      main { padding: 24px; text-align: center; }
      .spinner { width: 32px; height: 32px; margin: 0 auto 20px; border: 3px solid #e2e8f0;
        border-top-color: #2563eb; border-radius: 50%; animation: spin 1s linear infinite; }
      @keyframes spin { to { transform: rotate(360deg); } }
      @media (prefers-reduced-motion: reduce) { .spinner { animation: none; } }
    `
    const main = doc.createElement('main')
    main.setAttribute('role', 'status')
    const spinner = doc.createElement('div')
    spinner.className = 'spinner'
    spinner.setAttribute('aria-hidden', 'true')
    const text = doc.createElement('p')
    text.textContent = message
    main.append(spinner, text)
    doc.head.append(style)
    doc.body.replaceChildren(main)
  } catch { /* Rendering must not prevent checkout when browser document access fails. */ }
  return tab
}

// Return to the original payment page for retry; never submit a second order here.
export const showPaymentTabError = (tab: Window | null, message: string, returnLabel: string) => {
  if (!tab || tab.closed) return
  try {
    const doc = tab.document
    doc.title = message
    const main = doc.createElement('main')
    main.setAttribute('role', 'alert')
    const text = doc.createElement('p')
    text.textContent = message
    const back = doc.createElement('button')
    back.textContent = returnLabel
    back.style.cssText = 'padding:10px 18px;border:0;border-radius:8px;background:#2563eb;color:white;cursor:pointer'
    back.addEventListener('click', () => { window.focus(); tab.close() })
    main.append(text, back)
    doc.body.replaceChildren(main)
  } catch { tab.close() }
}

export const handoffPaymentTab = (orderNo: string, tab: Window, checkout?: CheckoutPaymentHandoff) => {
  reserved?.tab.close()
  reserved = { orderNo, tab, checkout }
}

export const takeCheckoutPayment = (orderNo: string): CheckoutPaymentHandoff | null => {
  if (!reserved || reserved.orderNo !== orderNo) return null
  const checkout = reserved.checkout
  reserved.checkout = undefined
  if (!checkout || reserved.tab.closed || checkout.order.order_no !== orderNo || !checkout.payment.pay_url) return null
  return checkout
}

export const takePaymentTab = (orderNo: string): Window | null => {
  if (!reserved || reserved.orderNo !== orderNo) return null
  const tab = reserved.tab
  reserved = null
  return tab.closed ? null : tab
}
