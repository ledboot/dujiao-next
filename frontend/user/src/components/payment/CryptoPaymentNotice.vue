<template>
  <div role="note" class="mt-4 w-full overflow-hidden rounded-xl border-2 border-warning/60 bg-warning/10 text-left text-sm leading-relaxed text-foreground">
    <div class="flex items-start gap-3 border-b border-warning/30 bg-warning/15 p-4">
      <TriangleAlert class="mt-0.5 h-6 w-6 shrink-0 text-warning" aria-hidden="true" />
      <div class="min-w-0">
        <p class="text-base font-bold text-warning">{{ t('payment.cryptoNoticeTitle') }}</p>
        <p class="mt-1 font-semibold">{{ t('payment.cryptoArrivalNotice') }}</p>
      </div>
    </div>
    <div class="space-y-3 p-4">
      <p class="font-medium">{{ t('payment.cryptoGasNotice') }}</p>
      <p class="rounded-lg border border-warning/30 bg-background/70 p-3 text-xs leading-relaxed sm:text-sm">{{ t('payment.cryptoGasExample') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { TriangleAlert } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
</script>
