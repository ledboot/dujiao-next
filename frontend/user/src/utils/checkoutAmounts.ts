import { amountToCents, calculateFeeCents, rateToBasisPoints } from './money.ts'

// All values are in order-currency cents. Product tax is already included in
// onlineCents; charge tax on the customer fee only once, using combined rounding.
export const checkoutSurcharges = (
  baseCents: number,
  onlineCents: number,
  invoiceRequested: boolean,
  channel?: { fee_policy?: string; fee_rate?: unknown; fixed_fee?: unknown },
) => {
  const productTax = invoiceRequested ? calculateFeeCents(baseCents, 300) || 0 : 0
  let feeBase = onlineCents
  if (invoiceRequested) {
    feeBase = onlineCents === baseCents + productTax ? baseCents : Math.round(onlineCents * 100 / 103)
  }
  const fee = onlineCents > 0 && channel?.fee_policy === 'customer_surcharge'
    ? (calculateFeeCents(feeBase, rateToBasisPoints(channel.fee_rate) || 0) || 0) + (amountToCents(channel.fixed_fee) || 0)
    : 0
  const tax = invoiceRequested ? calculateFeeCents(baseCents + fee, 300) || 0 : 0
  return { fee, tax, total: baseCents + fee + tax }
}
