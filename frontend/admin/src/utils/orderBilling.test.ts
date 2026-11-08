import { describe, expect, it } from 'vitest'
import type { AdminOrder } from '@/api/types'
import { orderPaidAmount } from './orderBilling'

const order = (overrides: Partial<AdminOrder> = {}): AdminOrder => ({
  total_amount: 123.60,
  tax_amount: '3.60',
  fee_amount: '6.00',
  payment_tax_amount: '0.18',
  paid_at: '2026-11-08T00:00:00Z',
  ...overrides,
} as AdminOrder)

describe('user order paid amount', () => {
  it.each(['pending_payment', 'canceled'])('shows zero for unpaid %s orders despite a payment quote', status => {
    expect(orderPaidAmount(order({ status, paid_at: undefined, online_paid_amount: 123.60 }))).toBe('0.00')
  })

  it('includes customer fees and fee tax without counting product tax twice', () => {
    expect(orderPaidAmount(order())).toBe('129.78')
  })

  it('counts mixed wallet and online payments once', () => {
    expect(orderPaidAmount(order({ wallet_paid_amount: 20, online_paid_amount: 103.60 }))).toBe('129.78')
  })

  it('supports wallet-only payments without surcharges', () => {
    expect(orderPaidAmount(order({ wallet_paid_amount: 123.60, fee_amount: undefined, payment_tax_amount: undefined }))).toBe('123.60')
  })

  it('preserves the original paid amount after refunds', () => {
    expect(orderPaidAmount(order({ status: 'refunded', refunded_amount: 123.60 }))).toBe('129.78')
  })

  it('adds decimal amounts in cents', () => {
    expect(orderPaidAmount(order({ total_amount: 0.1, fee_amount: '0.20', payment_tax_amount: '0.01' }))).toBe('0.31')
  })
})
