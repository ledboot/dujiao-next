import { amountToCents, centsToAmount } from './money.ts'

export const orderPayableAmount = (order: any): string => order?.payable_amount
  ?? centsToAmount((amountToCents(order?.total_amount) || 0) + (amountToCents(order?.fee_amount) || 0))

export const orderPaidAmount = (order: any): string => order?.paid_amount
  ?? (order?.paid_at ? orderPayableAmount(order) : '0.00')

export const orderDisplayAmount = (order: any): string => order?.paid_at ? orderPaidAmount(order) : orderPayableAmount(order)

export const orderSubtotalAmount = (order: any): string => centsToAmount(
  (amountToCents(order?.total_amount) || 0) - (amountToCents(order?.tax_amount) || 0),
)
