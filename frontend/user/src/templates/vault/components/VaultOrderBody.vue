<template>
  <div class="grid gap-[18px]">
    <!-- 子订单（含卡密） -->
    <section v-if="order.children && order.children.length > 0" class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.childOrdersTitle') }}</h2>
      <div class="grid gap-4">
        <div v-for="child in order.children" :key="child.id" class="rounded-lg border p-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <div class="text-[13px] text-muted-foreground">{{ t('orderDetail.childOrderNo') }}：{{ child.order_no }}</div>
              <div class="text-[13px] text-muted-foreground">{{ t('orderDetail.childOrderAmount') }}：{{ formatMoney(orderDisplayAmount(child), child.currency || order.currency) }}</div>
            </div>
            <Badge :variant="statusVariant(resolvedChildStatus(child))" size="sm" class="rounded-full">{{ statusLabel(resolvedChildStatus(child)) }}</Badge>
          </div>

              <OrderAmountDetails :order="child" :fallback-currency="order.currency" class="mt-4" />
          <h3 class="my-2.5 mt-4 text-sm font-bold">{{ t('orderDetail.childItemsTitle') }}</h3>
          <div v-if="child.items && child.items.length" class="grid">
            <VaultOrderItem v-for="(item, cidx) in child.items" :key="cidx" :item="item" :currency="child.currency || order.currency" />
          </div>
          <div v-else class="text-[13px] text-muted-foreground">{{ t('orderDetail.noItems') }}</div>

          <div class="mt-3.5 border-t pt-3.5">
            <VaultOrderFulfillment
              v-if="child.fulfillment"
              :title="t('orderDetail.childFulfillmentTitle')"
              :fulfillment="child.fulfillment"
              :items="child.items"
              :order-no="child.order_no || order.order_no"
              :downloading="fulfillmentDownloading"
              @download="emit('download', $event)"
            />
            <div v-else class="text-[13px] text-muted-foreground">{{ t('orderDetail.childFulfillmentEmpty') }}</div>
          </div>
        </div>
      </div>
    </section>

    <!-- 主订单发货（含卡密） -->
    <section v-if="order.fulfillment" class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.fulfillmentTitle') }}</h2>
      <VaultOrderFulfillment
        :fulfillment="order.fulfillment"
        :items="order.items"
        :order-no="order.order_no"
        :downloading="fulfillmentDownloading"
        @download="emit('download', $event)"
      />
    </section>

    <!-- 商品 -->
    <section class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.itemsTitle') }}</h2>
      <div v-if="order.items && order.items.length > 0" class="grid">
        <VaultOrderItem v-for="(item, idx) in order.items" :key="idx" :item="item" :currency="order.currency" />
      </div>
      <div v-else class="text-[13px] text-muted-foreground">{{ t('orderDetail.noItems') }}</div>
    </section>

    <!-- 金额明细 -->
    <section class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.amountTitle') }}</h2>
      <OrderAmountDetails :order="order" />
    </section>

    <!-- 退款记录 -->
    <section v-if="showRefundRecordsCard" class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.refundRecordsTitle') }}</h2>
      <div v-if="refundRecords.length > 0" class="overflow-hidden rounded-md border">
        <div class="grid grid-cols-[1.2fr_1fr_1.6fr] gap-3 bg-secondary px-3.5 py-2.5 text-xs font-bold text-muted-foreground max-[640px]:hidden">
          <span>{{ t('orderDetail.refundRecordTime') }}</span>
          <span>{{ t('orderDetail.refundRecordAmount') }}</span>
          <span>{{ t('orderDetail.refundRecordReason') }}</span>
        </div>
        <div v-for="(record, idx) in refundRecords" :key="`refund-${idx}`" class="grid grid-cols-[1.2fr_1fr_1.6fr] gap-3 border-t px-3.5 py-2.5 text-[13px] max-[640px]:grid-cols-1 max-[640px]:gap-1">
          <span class="text-muted-foreground">{{ formatDate(record.created_at) }}</span>
          <span class="tabular-nums">{{ formatMoney(record.amount, record.currency || order.currency) }}</span>
          <span class="whitespace-pre-wrap break-words text-muted-foreground">{{ refundReasonText(record.remark) }}</span>
        </div>
      </div>
      <div v-else class="text-[13px] text-muted-foreground">{{ t('orderDetail.refundRecordsEmpty') }}</div>
    </section>

    <!-- 时间信息 -->
    <section v-if="showTimeCard" class="rounded-xl border bg-card p-[22px]">
      <h2 class="mb-4 text-lg font-bold">{{ t('orderDetail.timeTitle') }}</h2>
      <div class="grid gap-3 grid-cols-[repeat(auto-fill,minmax(180px,1fr))]">
        <div class="rounded-md border bg-secondary px-3.5 py-3">
          <div class="text-xs text-muted-foreground">{{ t('orderDetail.createdAtLabel') }}</div>
          <div class="mt-1.5 font-bold">{{ formatDate(order.created_at) }}</div>
        </div>
        <div v-if="order.paid_at" class="rounded-md border bg-secondary px-3.5 py-3">
          <div class="text-xs text-muted-foreground">{{ t('orderDetail.paidAtLabel') }}</div>
          <div class="mt-1.5 font-bold">{{ formatDate(order.paid_at) }}</div>
        </div>
        <div v-if="order.expires_at" class="rounded-md border bg-secondary px-3.5 py-3">
          <div class="text-xs text-muted-foreground">{{ t('orderDetail.expiresAtLabel') }}</div>
          <div class="mt-1.5 font-bold">{{ formatDate(order.expires_at) }}</div>
        </div>
        <div v-if="order.canceled_at" class="rounded-md border bg-secondary px-3.5 py-3">
          <div class="text-xs text-muted-foreground">{{ t('orderDetail.canceledAtLabel') }}</div>
          <div class="mt-1.5 font-bold">{{ formatDate(order.canceled_at) }}</div>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import OrderAmountDetails from '../../../components/order/OrderAmountDetails.vue'
import { orderDisplayAmount } from '../../../utils/orderAmounts'
import { toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import VaultOrderItem from './VaultOrderItem.vue'
import VaultOrderFulfillment from './VaultOrderFulfillment.vue'
import { useOrderDisplayHelpers } from '../../../composables/useOrderDisplayHelpers'

const props = defineProps<{
  order: any
  variant: 'user' | 'guest'
  fulfillmentDownloading: boolean
}>()

const emit = defineEmits<{
  (e: 'download', orderNo: string): void
}>()

const { t } = useI18n()

const {
  showTimeCard, showRefundRecordsCard, refundRecords, resolvedChildStatus,
  statusLabel, statusVariant, formatDate, refundReasonText,
  formatMoney,
} = useOrderDisplayHelpers(toRef(props, 'order'))
</script>
