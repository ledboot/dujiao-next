<template>
  <Card
    class="group relative overflow-hidden flex flex-col h-full rounded-2xl border-border/80 transition-all duration-300 ease-[cubic-bezier(0.16,1,0.3,1)] theme-slide-up"
    :class="isSoldOut(product)
      ? 'cursor-default opacity-80 grayscale-[0.2] saturate-75 border-border'
      : 'cursor-pointer hover:-translate-y-1 hover:border-primary/35 hover:shadow-lg'"
    :style="{ animationDelay: `${index * animationStep}ms` }"
    @click="$emit('click', product.slug)">
    <!-- Image Area -->
    <div class="aspect-[4/3] overflow-hidden bg-muted/70 relative shrink-0 border-b border-border/50">
      <div
        class="pointer-events-none absolute inset-0 z-10 bg-gradient-to-t from-black/25 via-transparent to-transparent opacity-0 transition-opacity duration-300"
        :class="isSoldOut(product) ? '' : 'group-hover:opacity-100'"
      ></div>
      <img v-if="displayImageSrc && !imageErrored" :src="displayImageSrc"
        :alt="getLocalizedText(product.title)" loading="lazy" decoding="async"
        class="w-full h-full object-cover transform transition-transform duration-600 ease-[cubic-bezier(0.16,1,0.3,1)]"
        :class="[
          isSoldOut(product) ? 'grayscale brightness-80' : 'group-hover:scale-[1.05]',
        ]"
        @error="handleImageError" />
      <div v-else class="w-full h-full flex items-center justify-center text-muted-foreground/60" role="img"
        :aria-label="getLocalizedText(product.title)">
        <ImageIcon class="w-8 h-8 md:w-11 md:h-11" :stroke-width="1.5" aria-hidden="true" />
      </div>

      <div v-if="isSoldOut(product)" class="absolute inset-0 z-20 bg-black/40 backdrop-blur-[1px]"></div>
      <Badge
        v-if="isSoldOut(product)"
        variant="destructive"
        size="xs"
        class="absolute left-2.5 top-2.5 md:left-3.5 md:top-3.5 z-30 shadow-sm"
      >
        {{ t('products.stockStatus.outOfStock') }}
      </Badge>

      <!-- Tags -->
      <div v-if="!isSoldOut(product) && product.tags && product.tags.length > 0"
        class="absolute top-2.5 right-2.5 md:top-3.5 md:right-3.5 z-20 flex flex-wrap gap-1.5 justify-end">
        <span v-for="(tag, tagIndex) in product.tags.slice(0, maxTags)" :key="tagIndex"
          class="inline-flex items-center rounded-full border border-white/25 bg-black/55 px-2.5 py-0.5 text-[11px] font-medium text-white backdrop-blur-md">
          {{ tag }}
        </span>
      </div>
    </div>

    <!-- Content Area -->
    <div class="p-3.5 md:p-4 relative z-20 flex flex-col flex-1">
      <div v-if="product.category?.name" class="text-[11px] font-medium text-muted-foreground mb-1 truncate">
        {{ getLocalizedText(product.category.name) }}
      </div>
      <h3 class="text-sm md:text-base font-semibold tracking-tight text-foreground mb-2 transition-colors line-clamp-1 group-hover:text-primary">
        {{ getLocalizedText(product.title) }}
      </h3>

      <!-- Badges -->
      <div class="mb-2.5 md:mb-3 flex flex-wrap items-center gap-1.5">
        <!-- Mobile: show only fulfillment type badge -->
        <Badge
          class="md:hidden"
          size="xs"
          :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'"
        >
          {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
        </Badge>

        <!-- Desktop: show all badges -->
        <Badge
          class="hidden md:inline-flex"
          size="xs"
          :variant="product.purchase_type === 'guest' ? 'warning' : 'success'"
        >
          <UserPlus v-if="product.purchase_type === 'guest'" class="h-3 w-3" />
          <Lock v-else class="h-3 w-3" />
          {{ getPurchaseTypeLabel(product.purchase_type) }}
        </Badge>

        <Badge
          class="hidden md:inline-flex"
          size="xs"
          :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'"
        >
          <Zap v-if="product.fulfillment_type === 'auto'" class="h-3 w-3" />
          <Pencil v-else class="h-3 w-3" />
          {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
        </Badge>

        <Badge class="hidden md:inline-flex" size="xs" :variant="getStockBadgeVariant(product.stock_status)">
          {{ getStockStatusLabel(product) }}
        </Badge>
      </div>

      <p class="hidden md:block text-muted-foreground text-xs leading-relaxed mb-4 line-clamp-2">
        {{ getLocalizedText(product.description) }}
      </p>

      <div class="flex items-end justify-between border-t border-border/65 pt-2.5 md:pt-3.5 mt-auto gap-2">
        <div class="flex flex-col min-w-0">
          <span
            v-if="exclusivePrice !== null"
            class="theme-price-sm text-amber-600 dark:text-amber-300"
          >
            {{ formatPrice(exclusivePrice, siteCurrency) }}
          </span>
          <span
            v-else-if="hasPromotionPrice(product)"
            class="theme-price-sm theme-price-promotion"
            :aria-label="t('products.promotionPriceAria', { price: formatPrice(getPromotionPriceAmount(product), siteCurrency) })"
          >
            {{ formatPrice(getPromotionPriceAmount(product), siteCurrency) }}
          </span>
          <span
            v-else
            class="theme-price-sm"
            :aria-label="t('products.priceAria', { price: formatPrice(product.price_amount, siteCurrency) })"
          >
            {{ formatPrice(product.price_amount, siteCurrency) }}
          </span>
          <div v-if="exclusivePrice !== null" class="mt-0.5 flex flex-wrap items-center gap-1.5">
            <span class="hidden md:inline text-xs text-muted-foreground opacity-75 line-through tabular-nums">{{ formatPrice(product.price_amount, siteCurrency) }}</span>
            <Badge variant="warning" size="xs">
              {{ t('products.memberPriceTag') }}
            </Badge>
          </div>
          <div v-else-if="hasPromotionPrice(product)" class="mt-0.5 flex flex-wrap items-center gap-1.5">
            <span
              class="hidden md:inline text-xs text-muted-foreground opacity-75 line-through tabular-nums"
              :aria-label="t('products.originalPriceAria', { price: formatPrice(product.price_amount, siteCurrency) })"
            >{{ formatPrice(product.price_amount, siteCurrency) }}</span>
            <Badge variant="danger" size="xs">
              {{ t('products.promotionTag') }}
            </Badge>
          </div>
          <div v-else-if="hasWholesalePrices(product)" class="mt-0.5 flex flex-wrap items-center gap-1.5">
            <Badge variant="success" size="xs">
              {{ t('products.wholesaleTag') }}
            </Badge>
          </div>
          <div v-else-if="hasPromotionRules(product)" class="mt-0.5 flex flex-wrap items-center gap-1.5">
            <Badge variant="warning" size="xs">
              {{ t('products.promotionBadge') }}
            </Badge>
          </div>
        </div>

        <div class="flex items-center gap-1.5 shrink-0">
          <!-- Quick buy cart button -->
          <Button
            type="button"
            variant="secondary"
            size="icon"
            class="w-8 h-8 md:w-9 md:h-9 rounded-full transition-all duration-200 group-hover:bg-primary group-hover:text-primary-foreground"
            :aria-label="t('products.quickBuyAria')"
            :disabled="isSoldOut(product)"
            @click.stop="$emit('quickBuy', product)"
          >
            <ShoppingCart class="h-4 w-4" />
          </Button>
          <!-- Desktop: view details -->
          <span
            class="hidden md:flex text-xs font-semibold transition-colors items-center"
            :class="isSoldOut(product)
              ? 'text-muted-foreground/50'
              : 'text-muted-foreground group-hover:text-primary'">
            <ArrowRight class="w-4 h-4 transition-transform duration-200" :class="isSoldOut(product) ? '' : 'group-hover:translate-x-0.5'" />
          </span>
          <!-- Mobile: arrow only -->
          <ChevronRight class="md:hidden w-4 h-4 text-muted-foreground" />
        </div>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, ref, watch } from 'vue'
import { ArrowRight, ChevronRight, Image as ImageIcon, Lock, Pencil, ShoppingCart, UserPlus, Zap } from 'lucide-vue-next'
import { getFirstImageUrl, getImageUrl } from '../utils/image'
import { useLocalized, useProductLabels } from '../composables/useProduct'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'

const props = withDefaults(defineProps<{
  product: any
  index?: number
  maxTags?: number
  animationStep?: number
}>(), {
  index: 0,
  maxTags: 2,
  animationStep: 50,
})

defineEmits<{
  click: [slug: string]
  quickBuy: [product: any]
}>()

const { t } = useI18n()
const { getLocalizedText, siteCurrency, formatPrice } = useLocalized()
const { getPurchaseTypeLabel, getFulfillmentTypeLabel, getStockBadgeVariant, getStockStatusLabel, isSoldOut, hasPromotionPrice, getPromotionPriceAmount, hasPromotionRules, hasWholesalePrices, getCardExclusivePrice } = useProductLabels()
const exclusivePrice = computed(() => getCardExclusivePrice(props.product))

const imageErrored = ref(false)
const attemptIdx = ref(0)

const imageCandidates = computed<string[]>(() => {
  const arr: string[] = []
  const primary = getFirstImageUrl(props.product?.images)
  if (primary) arr.push(primary)
  const categoryIcon = props.product?.category?.icon
  if (categoryIcon) {
    const resolved = getImageUrl(categoryIcon)
    if (resolved && resolved !== primary) arr.push(resolved)
  }
  return arr
})

const displayImageSrc = computed(() => imageCandidates.value[attemptIdx.value] ?? '')

watch(imageCandidates, () => {
  attemptIdx.value = 0
  imageErrored.value = false
}, { deep: true })

const handleImageError = () => {
  if (attemptIdx.value < imageCandidates.value.length - 1) {
    attemptIdx.value++
  } else {
    imageErrored.value = true
  }
}
</script>
