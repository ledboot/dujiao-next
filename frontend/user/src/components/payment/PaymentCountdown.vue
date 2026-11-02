<template>
  <div
    role="timer"
    class="flex items-center justify-between gap-3 rounded-2xl border-2 p-4 sm:p-5"
    :class="expired ? 'border-destructive/50 bg-destructive/10' : 'border-success/50 bg-success/10'"
  >
    <div class="flex min-w-0 items-center gap-2 sm:gap-3">
      <span
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full sm:h-11 sm:w-11"
        :class="expired ? 'bg-destructive/20 text-destructive' : 'bg-success/20 text-success'"
      >
        <Clock class="h-5 w-5 sm:h-6 sm:w-6" aria-hidden="true" />
      </span>
      <span
        class="whitespace-nowrap text-sm font-bold sm:text-base"
        :class="expired ? 'text-destructive' : 'text-success'"
      >
        {{ t('payment.countdownLabel') }}
      </span>
    </div>
    <div
      class="shrink-0 whitespace-nowrap font-mono text-3xl font-black tabular-nums sm:text-5xl"
      :class="expired ? 'text-destructive' : 'text-success'"
    >
      {{ text }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { Clock } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'

defineProps<{
  text: string
  expired: boolean
}>()

const { t } = useI18n()
</script>
