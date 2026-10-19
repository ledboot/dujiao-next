import test from 'node:test'
import assert from 'node:assert/strict'
import { handoffPaymentTab, openPaymentTab, showPaymentTabError, takeCheckoutPayment, takePaymentTab } from '../src/utils/paymentTab.ts'

const fakeTab = () => {
  const state = { closed: false, close() { this.closed = true } }
  return state as unknown as Window
}

test('checkout tab is consumed only once by its matching order', () => {
  const tab = fakeTab()
  handoffPaymentTab('ORDER-A', tab)
  assert.equal(takePaymentTab('ORDER-B'), null)
  assert.equal(takePaymentTab('ORDER-A'), tab)
  assert.equal(takePaymentTab('ORDER-A'), null)
  assert.equal(tab.closed, false)
})

test('replaced and manually closed blank tabs cannot be reused', () => {
  const previous = fakeTab()
  const next = fakeTab()
  handoffPaymentTab('ORDER-A', previous)
  handoffPaymentTab('ORDER-B', next)
  assert.equal(previous.closed, true)
  next.close()
  assert.equal(takePaymentTab('ORDER-B'), null)
})


test('checkout result is handed off only to its order and only once', () => {
  const tab = fakeTab()
  const checkout = { order: { order_no: 'ORDER-A', status: 'pending_payment' }, payment: { channel_id: 4, pay_url: 'https://pay.example.test/' } }
  handoffPaymentTab('ORDER-A', tab, checkout)
  assert.equal(takeCheckoutPayment('ORDER-B'), null)
  assert.equal(takeCheckoutPayment('ORDER-A'), checkout)
  assert.equal(takeCheckoutPayment('ORDER-A'), null)
  assert.equal(takePaymentTab('ORDER-A'), tab)
})

test('closed tabs and mismatched order snapshots cannot provide a checkout result', () => {
  for (const closed of [false, true]) {
    const tab = fakeTab()
    const checkout = { order: { order_no: closed ? 'ORDER-A' : 'OTHER', status: 'pending_payment' }, payment: { channel_id: 4, pay_url: 'https://pay.example.test/' } }
    handoffPaymentTab('ORDER-A', tab, checkout)
    if (closed) tab.close()
    assert.equal(takeCheckoutPayment('ORDER-A'), null)
    takePaymentTab('ORDER-A')
  }
})

function mockDocument() {
  const element = () => ({
    textContent: '', className: '', style: { cssText: '' }, children: [] as any[],
    attributes: {} as Record<string, string>, listeners: {} as Record<string, () => void>,
    setAttribute(key: string, value: string) { this.attributes[key] = value },
    append(...items: any[]) { this.children.push(...items) },
    replaceChildren(...items: any[]) { this.children = items },
    addEventListener(event: string, fn: () => void) { this.listeners[event] = fn },
  })
  return { title: '', documentElement: { lang: 'zh-CN' }, head: element(), body: element(), createElement: element }
}

test('reserved tab shows loading immediately and renders errors as text with a return action', () => {
  const previousWindow = Object.getOwnPropertyDescriptor(globalThis, 'window')
  const previousDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
  const doc = mockDocument()
  let focused = false
  const tab = { ...fakeTab(), document: doc, closed: false, close() { this.closed = true } }
  Object.defineProperty(globalThis, 'window', { configurable: true, value: { open: () => tab, focus: () => { focused = true } } })
  Object.defineProperty(globalThis, 'document', { configurable: true, value: doc })
  try {
    assert.equal(openPaymentTab('Opening checkout…'), tab)
    assert.equal(doc.title, 'Opening checkout…')
    assert.equal(doc.body.children[0].children[1].textContent, 'Opening checkout…')
    showPaymentTabError(tab as unknown as Window, '<img src=x onerror=alert(1)>', 'Return to payment')
    const main = doc.body.children[0]
    assert.equal(main.attributes.role, 'alert')
    assert.equal(main.children[0].textContent, '<img src=x onerror=alert(1)>')
    main.children[1].listeners.click()
    assert.equal(focused, true)
    assert.equal(tab.closed, true)
  } finally {
    if (previousWindow) Object.defineProperty(globalThis, 'window', previousWindow)
    else Reflect.deleteProperty(globalThis, 'window')
    if (previousDocument) Object.defineProperty(globalThis, 'document', previousDocument)
    else Reflect.deleteProperty(globalThis, 'document')
  }
})
