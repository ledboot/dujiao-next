import { amountToCents, calculateFeeCents, rateToBasisPoints } from './money.ts'

// 开票类型对应的手续费比例（基点）：普票 3%，专票 5%，空值不开票。
export const invoiceTaxBasisPoints = (invoiceType?: string) =>
  invoiceType === 'special' ? 500 : invoiceType === 'general' ? 300 : 0

// All values are in order-currency cents. Product tax is already included in
// onlineCents; charge tax on the customer fee only once, using combined rounding.
export const checkoutSurcharges = (
  baseCents: number,
  onlineCents: number,
  invoiceType: string,
  channel?: { fee_policy?: string; fee_rate?: unknown; fixed_fee?: unknown },
) => {
  const taxBasisPoints = invoiceTaxBasisPoints(invoiceType)
  const productTax = taxBasisPoints > 0 ? calculateFeeCents(baseCents, taxBasisPoints) || 0 : 0
  let feeBase = onlineCents
  if (taxBasisPoints > 0) {
    feeBase = onlineCents === baseCents + productTax ? baseCents : Math.round(onlineCents * 10000 / (10000 + taxBasisPoints))
  }
  const fee = onlineCents > 0 && channel?.fee_policy === 'customer_surcharge'
    ? (calculateFeeCents(feeBase, rateToBasisPoints(channel.fee_rate) || 0) || 0) + (amountToCents(channel.fixed_fee) || 0)
    : 0
  const tax = taxBasisPoints > 0 ? calculateFeeCents(baseCents + fee, taxBasisPoints) || 0 : 0
  return { fee, tax, total: baseCents + fee + tax }
}
