<template>
  <Card
    class="group relative overflow-hidden flex flex-row items-center rounded-2xl border-border/80 transition-all duration-250 cursor-pointer theme-slide-up"
    :class="isSoldOut(product)
      ? 'opacity-80 grayscale-[0.2] saturate-75 border-border'
      : 'hover:-translate-y-0.5 hover:border-primary/35 hover:shadow-md'"
    :style="{ animationDelay: `${index * animationStep}ms` }"
    @click="$emit('click', product.slug)">

    <!-- Thumbnail -->
    <div class="w-12 h-12 sm:w-16 sm:h-16 flex-shrink-0 overflow-hidden relative rounded-xl border border-border/50 m-2 sm:m-2.5 bg-muted/60">
      <img v-if="product.images && getFirstImageUrl(product.images)" :src="getFirstImageUrl(product.images)"
        :alt="getLocalizedText(product.title)" loading="lazy"
        class="w-full h-full object-cover transition-transform duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]"
        :class="isSoldOut(product) ? 'grayscale brightness-80' : 'group-hover:scale-108'" />
      <img v-else-if="product.category?.icon" :src="getImageUrl(product.category.icon)"
        :alt="getLocalizedText(product.category?.name)" loading="lazy"
        class="w-full h-full object-cover transition-transform duration-500 ease-[cubic-bezier(0.16,1,0.3,1)]"
        :class="isSoldOut(product) ? 'grayscale brightness-80' : 'group-hover:scale-108'" />
      <div v-else class="w-full h-full flex items-center justify-center text-muted-foreground/60">
        <ImageIcon class="w-5 h-5" :stroke-width="1.5" />
      </div>
      <!-- Sold out overlay -->
      <div v-if="isSoldOut(product)" class="absolute inset-0 bg-black/50 flex items-center justify-center">
        <span class="text-white text-[10px] font-bold">{{ t('products.stockStatus.outOfStock') }}</span>
      </div>
    </div>

    <!-- Info (flex-1) -->
    <div class="flex-1 min-w-0 py-2 pr-1 flex flex-col justify-center gap-1">
      <!-- Row 1: Category · Title · Tags -->
      <div class="flex items-center gap-1.5 min-w-0">
        <span v-if="product.category?.name" class="hidden sm:inline text-[11px] font-medium text-muted-foreground truncate max-w-[92px] flex-shrink-0">
          {{ getLocalizedText(product.category.name) }}
        </span>
        <span v-if="product.category?.name" class="hidden sm:inline text-[11px] text-muted-foreground opacity-35 flex-shrink-0">·</span>
        <h3 class="text-xs sm:text-sm font-semibold tracking-tight text-foreground truncate transition-colors group-hover:text-primary">
          {{ getLocalizedText(product.title) }}
        </h3>
        <span v-for="(tag, tagIndex) in (product.tags || []).slice(0, 1)" :key="tagIndex"
          class="hidden md:inline-flex items-center rounded-full border border-border bg-secondary px-2 py-0.5 text-[10px] font-medium text-muted-foreground flex-shrink-0">
          {{ tag }}
        </span>
      </div>

      <!-- Row 2: Badges -->
      <div class="flex flex-wrap items-center gap-1">
        <!-- Mobile: fulfillment + stock warning -->
        <Badge class="sm:hidden" size="xs" :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'">
          {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
        </Badge>
        <Badge v-if="product.stock_status === 'out_of_stock' || product.stock_status === 'low_stock'"
          class="sm:hidden" size="xs" :variant="getStockBadgeVariant(product.stock_status)">
          {{ getStockStatusLabel(product) }}
        </Badge>

        <!-- Desktop: all badges -->
        <Badge class="hidden sm:inline-flex" size="xs" :variant="product.purchase_type === 'guest' ? 'warning' : 'success'">
          {{ getPurchaseTypeLabel(product.purchase_type) }}
        </Badge>
        <Badge class="hidden sm:inline-flex" size="xs" :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'">
          {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
        </Badge>
        <Badge class="hidden sm:inline-flex" size="xs" :variant="getStockBadgeVariant(product.stock_status)">
          {{ getStockStatusLabel(product) }}
        </Badge>
      </div>
    </div>

    <!-- Price + Actions (right) -->
    <div class="flex-shrink-0 flex items-center gap-1.5 sm:gap-3 pr-2 sm:pr-4">
      <!-- Price -->
      <div class="text-right tabular-nums">
        <div v-if="exclusivePrice !== null" class="flex flex-col items-end">
          <span class="text-xs sm:text-sm font-bold text-amber-600 dark:text-amber-300 whitespace-nowrap">
            {{ formatPrice(exclusivePrice, siteCurrency) }}
          </span>
          <div class="flex items-center gap-1">
            <span class="text-[10px] text-muted-foreground line-through">{{ formatPrice(product.price_amount, siteCurrency) }}</span>
            <Badge variant="warning" size="xs" class="px-1.5 py-0 text-[9px] leading-tight">{{ t('products.memberPriceTag') }}</Badge>
          </div>
        </div>
        <div v-else-if="hasPromotionPrice(product)" class="flex flex-col items-end">
          <span class="text-xs sm:text-sm font-bold text-destructive whitespace-nowrap">
            {{ formatPrice(getPromotionPriceAmount(product), siteCurrency) }}
          </span>
          <div class="flex items-center gap-1">
            <span class="text-[10px] text-muted-foreground line-through">{{ formatPrice(product.price_amount, siteCurrency) }}</span>
            <Badge variant="danger" size="xs" class="px-1.5 py-0 text-[9px] leading-tight">{{ t('products.promotionTag') }}</Badge>
          </div>
        </div>
        <div v-else>
          <span class="text-xs sm:text-sm font-bold text-foreground whitespace-nowrap">
            {{ formatPrice(product.price_amount, siteCurrency) }}
          </span>
          <div v-if="hasWholesalePrices(product)">
            <Badge variant="success" size="xs" class="px-1.5 py-0 text-[9px] leading-tight">{{ t('products.wholesaleTag') }}</Badge>
          </div>
          <div v-else-if="hasPromotionRules(product)">
            <Badge variant="warning" size="xs" class="px-1.5 py-0 text-[9px] leading-tight">{{ t('products.promotionBadge') }}</Badge>
          </div>
        </div>
      </div>

      <!-- Quick buy cart icon -->
      <Button
        type="button"
        variant="secondary"
        size="icon"
        class="w-8 h-8 rounded-full flex-shrink-0 transition-all duration-200 group-hover:bg-primary group-hover:text-primary-foreground"
        :disabled="isSoldOut(product)"
        @click.stop="$emit('quickBuy', product)"
      >
        <ShoppingCart class="h-3.5 w-3.5" />
      </Button>

      <!-- Arrow -->
      <ChevronRight class="hidden sm:block w-4 h-4 flex-shrink-0 transition-transform duration-200 text-muted-foreground"
        :class="isSoldOut(product) ? '' : 'group-hover:translate-x-0.5 group-hover:text-primary'" />
    </div>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronRight, Image as ImageIcon, ShoppingCart } from 'lucide-vue-next'
import { getFirstImageUrl, getImageUrl } from '../utils/image'
import { useLocalized, useProductLabels } from '../composables/useProduct'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

const props = withDefaults(defineProps<{
  product: any
  index?: number
  animationStep?: number
}>(), {
  index: 0,
  animationStep: 30,
})

defineEmits<{
  click: [slug: string]
  quickBuy: [product: any]
}>()

const { t } = useI18n()
const { getLocalizedText, siteCurrency, formatPrice } = useLocalized()
const { getPurchaseTypeLabel, getFulfillmentTypeLabel, getStockBadgeVariant, getStockStatusLabel, isSoldOut, hasPromotionPrice, getPromotionPriceAmount, hasPromotionRules, hasWholesalePrices, getCardExclusivePrice } = useProductLabels()
const exclusivePrice = computed(() => getCardExclusivePrice(props.product))
</script>
