<template>
  <div class="grid gap-3 grid-cols-[repeat(auto-fit,minmax(150px,1fr))]">
    <div v-for="row in rows" :key="row.key" class="rounded-lg border bg-secondary/40 p-3">
      <div class="text-xs text-muted-foreground">{{ row.label }}</div>
      <div class="mt-1.5 font-semibold tabular-nums" :class="row.discount ? 'text-destructive' : 'text-foreground'">
        {{ row.discount ? formatDiscountMoney(row.amount, currency) : formatMoney(row.amount, currency) }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useOrderDisplayHelpers } from '../../composables/useOrderDisplayHelpers'
import { orderPaidAmount, orderPayableAmount, orderSubtotalAmount } from '../../utils/orderAmounts'

const props = defineProps<{ order: any; fallbackCurrency?: string }>()
const { t } = useI18n()
const { formatMoney, formatDiscountMoney, hasAmount } = useOrderDisplayHelpers(toRef(props, 'order'))
const currency = computed(() => props.order.currency || props.fallbackCurrency)
const rows = computed(() => {
  const order = props.order
  const result = [{ key: 'original', label: t('orderDetail.amountOriginal'), amount: order.original_amount || '0', discount: false }]
  for (const [key, label] of [
    ['discount_amount', 'orderDetail.amountDiscount'],
    ['promotion_discount_amount', 'orderDetail.promotionDiscountLabel'],
    ['wholesale_discount_amount', 'orderDetail.amountWholesaleDiscount'],
    ['member_discount_amount', 'orderDetail.amountMemberDiscount'],
  ]) {
    if (hasAmount(order[key!])) result.push({ key: key!, label: t(label!), amount: order[key!], discount: true })
  }
  result.push(
    { key: 'subtotal', label: t('orderDetail.amountSubtotal'), amount: orderSubtotalAmount(order), discount: false },
    { key: 'fee', label: t('payment.feeAmountLabel'), amount: order.fee_amount || '0', discount: false },
    { key: 'tax', label: t('checkout.taxAmount'), amount: order.invoice_tax_amount ?? order.tax_amount ?? '0', discount: false },
    { key: 'payable', label: t('orderDetail.amountPayable'), amount: orderPayableAmount(order), discount: false },
    { key: 'paid', label: t('orderDetail.amountTotal'), amount: orderPaidAmount(order), discount: false },
  )
  for (const [key, label, amount] of [
    ['wallet', 'orderDetail.amountWalletPaid', order.wallet_paid_amount],
    ['online', 'orderDetail.amountOnlinePaid', order.online_payable_amount ?? order.online_paid_amount],
    ['refunded', 'orderDetail.amountRefunded', order.refunded_amount],
  ]) {
    if (hasAmount(amount)) result.push({ key, label: t(label), amount, discount: false })
  }
  return result
})
</script>
