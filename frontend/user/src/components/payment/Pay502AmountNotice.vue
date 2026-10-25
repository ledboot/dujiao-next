<template>
  <div role="note" class="mt-4 w-full max-w-xl rounded-xl border-2 border-warning/60 bg-warning/10 p-4 text-center text-foreground">
    <div class="flex items-center justify-center gap-2 text-sm font-bold text-warning">
      <TriangleAlert class="h-5 w-5 shrink-0" aria-hidden="true" />
      <p>{{ t('payment.pay502AmountNoticeTitle') }}</p>
    </div>
    <p class="my-2 break-words text-3xl font-bold tabular-nums">{{ amount }}</p>
    <p class="mb-2 text-sm font-semibold">{{ t('payment.pay502ValidityHint') }}</p>
    <p class="text-sm leading-relaxed">{{ t('payment.pay502AmountNoticeHint') }}</p>
  </div>
</template>

<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

defineProps<{ amount: string }>()
const { t } = useI18n()
</script>
