import test from 'node:test'
import assert from 'node:assert/strict'
import { orderDisplayAmount, orderPaidAmount, orderPayableAmount, orderSubtotalAmount } from '../src/utils/orderAmounts.ts'

const invoiceOrder = { total_amount: '123.60', tax_amount: '3.60', fee_amount: '6.00', invoice_tax_amount: '3.78', payable_amount: '129.78', paid_amount: '129.78', paid_at: '2026-10-24T00:00:00Z' }

test('detail displays actual payment including fee and all tax', () => {
  assert.equal(orderDisplayAmount(invoiceOrder), '129.78')
  assert.equal(orderSubtotalAmount(invoiceOrder), '120.00')
  assert.equal(orderPayableAmount(invoiceOrder), '129.78')
})

test('pending order distinguishes amount due and amount paid', () => {
  const pending = { ...invoiceOrder, paid_at: null, paid_amount: '0.00' }
  assert.equal(orderDisplayAmount(pending), '129.78')
  assert.equal(orderPaidAmount(pending), '0.00')
})

test('historical data fallback preserves included tax without charging it again', () => {
  assert.equal(orderPayableAmount({ total_amount: '123.60', tax_amount: '3.60', fee_amount: '6.00' }), '129.60')
  assert.equal(orderDisplayAmount({ total_amount: '120.00', paid_at: '2026-10-24' }), '120.00')
  assert.equal(orderSubtotalAmount({ total_amount: '120.00' }), '120.00')
})
