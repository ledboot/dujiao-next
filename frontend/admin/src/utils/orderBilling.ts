import type { AdminOrder } from '@/api/types'

// Match the order detail's paid total; total_amount already includes product tax.
export const orderPaidAmount = (order: Pick<AdminOrder, 'paid_at' | 'total_amount' | 'fee_amount' | 'payment_tax_amount'>): string => {
  if (!order.paid_at) return '0.00'
  const cents = [order.total_amount, order.fee_amount, order.payment_tax_amount]
    .reduce<number>((sum, value) => sum + Math.round(Number(value || 0) * 100), 0)
  return (cents / 100).toFixed(2)
}
