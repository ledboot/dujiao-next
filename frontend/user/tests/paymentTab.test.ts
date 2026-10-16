import test from 'node:test'
import assert from 'node:assert/strict'
import { handoffPaymentTab, takePaymentTab } from '../src/utils/paymentTab.ts'

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
