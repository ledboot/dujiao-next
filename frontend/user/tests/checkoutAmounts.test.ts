import test from 'node:test'
import assert from 'node:assert/strict'
import { checkoutSurcharges, invoiceTaxBasisPoints } from '../src/utils/checkoutAmounts.ts'

const channel = { fee_policy: 'customer_surcharge', fee_rate: '5.00', fixed_fee: '0.00' }

test('general invoice adds 3% tax on product plus customer fee', () => {
  assert.deepEqual(checkoutSurcharges(12000, 12360, 'general', channel), { fee: 600, tax: 378, total: 12978 })
  assert.deepEqual(checkoutSurcharges(12000, 12000, '', channel), { fee: 600, tax: 0, total: 12600 })
})

test('special invoice adds 5% tax on product plus customer fee', () => {
  assert.deepEqual(checkoutSurcharges(12000, 12600, 'special', channel), { fee: 600, tax: 630, total: 13230 })
  assert.deepEqual(checkoutSurcharges(12000, 12600, 'special', { ...channel, fee_policy: 'merchant_absorbed' }), { fee: 0, tax: 600, total: 12600 })
})

test('invoice tax basis points follow the selected invoice type', () => {
  assert.equal(invoiceTaxBasisPoints('general'), 300)
  assert.equal(invoiceTaxBasisPoints('special'), 500)
  assert.equal(invoiceTaxBasisPoints(''), 0)
  assert.equal(invoiceTaxBasisPoints(undefined), 0)
})

test('switching channel recalculates fixed fees and tax', () => {
  assert.deepEqual(checkoutSurcharges(12000, 12360, 'general', { ...channel, fee_rate: '0', fixed_fee: '6' }), { fee: 600, tax: 378, total: 12978 })
  assert.deepEqual(checkoutSurcharges(12000, 12360, 'general', { ...channel, fee_policy: 'merchant_absorbed' }), { fee: 0, tax: 360, total: 12360 })
  assert.deepEqual(checkoutSurcharges(12000, 12360, 'general'), { fee: 0, tax: 360, total: 12360 })
})

test('wallet covers goods and tax; only remaining goods incur channel fees', () => {
  assert.deepEqual(checkoutSurcharges(12000, 0, 'general', channel), { fee: 0, tax: 360, total: 12360 })
  assert.deepEqual(checkoutSurcharges(12000, 6180, 'general', channel), { fee: 300, tax: 369, total: 12669 })
})

test('tax rounds combined goods and fees to cents without double rounding', () => {
  assert.deepEqual(checkoutSurcharges(16, 16, 'general', { ...channel, fee_rate: '0', fixed_fee: '0.01' }), { fee: 1, tax: 1, total: 18 })
  assert.deepEqual(checkoutSurcharges(17, 18, 'general', { ...channel, fee_rate: '0', fixed_fee: '0.01' }), { fee: 1, tax: 1, total: 19 })
})
