<script setup lang="ts">
import { Copy } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { copyText } from '@/utils/clipboard'
import { notifyError } from '@/utils/notify'

const props = defineProps<{
  value: number | string
}>()

const { t } = useI18n()

const handleCopy = async () => {
  try {
    await copyText(String(props.value))
  } catch (error) {
    notifyError(t('admin.common.copyFailed'))
  }
}
</script>

<template>
  <div class="group inline-flex items-center gap-1.5 text-xs text-muted-foreground">
    <span class="font-mono tabular-nums text-foreground/90">#{{ value }}</span>
    <button
      type="button"
      class="inline-flex h-5 w-5 items-center justify-center rounded-md text-muted-foreground/70 transition-all hover:bg-muted hover:text-foreground active:scale-95"
      :title="t('admin.common.copy')"
      @click="handleCopy"
    >
      <Copy class="h-3 w-3" />
    </button>
  </div>
</template>
