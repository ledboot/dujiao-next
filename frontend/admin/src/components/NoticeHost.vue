<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-vue-next'
import { useNoticeStore, type NoticeItem } from '@/stores/notice'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

const noticeStore = useNoticeStore()
const { t } = useI18n()

const notices = computed(() => noticeStore.items)

const getAlertVariant = (notice: NoticeItem) => {
  if (notice.type === 'error') {
    return 'destructive'
  }
  return 'default'
}

const getContainerClass = (notice: NoticeItem) => {
  if (notice.type === 'success') {
    return 'border-emerald-500/25 bg-card/95 text-foreground dark:border-emerald-400/25'
  }
  if (notice.type === 'info') {
    return 'border-sky-500/25 bg-card/95 text-foreground dark:border-sky-400/25'
  }
  return 'border-rose-500/30 bg-card/95 text-foreground dark:border-rose-400/30'
}

const getIconColorClass = (notice: NoticeItem) => {
  if (notice.type === 'success') {
    return 'text-emerald-600 dark:text-emerald-400'
  }
  if (notice.type === 'info') {
    return 'text-sky-600 dark:text-sky-400'
  }
  return 'text-rose-600 dark:text-rose-400'
}

const getTitle = (notice: NoticeItem) => {
  if (notice.type === 'success') {
    return t('admin.common.success')
  }
  if (notice.type === 'info') {
    return t('admin.common.notice')
  }
  return t('admin.common.error')
}

const getIcon = (notice: NoticeItem) => {
  if (notice.type === 'success') {
    return CheckCircle2
  }
  if (notice.type === 'info') {
    return Info
  }
  return AlertCircle
}
</script>

<template>
  <div class="pointer-events-none fixed right-4 top-4 z-[120] flex w-[min(420px,calc(100vw-2rem))] flex-col gap-2.5">
    <TransitionGroup name="notice-fade" tag="div" class="flex flex-col gap-2.5">
      <Alert
        v-for="notice in notices"
        :key="notice.id"
        :variant="getAlertVariant(notice)"
        class="pointer-events-auto rounded-2xl shadow-xl backdrop-blur-xl"
        :class="getContainerClass(notice)"
      >
        <component :is="getIcon(notice)" class="h-4 w-4" :class="getIconColorClass(notice)" />
        <div>
          <div class="flex items-start justify-between gap-3">
            <AlertTitle class="text-sm font-semibold">{{ getTitle(notice) }}</AlertTitle>
            <Button
              variant="ghost"
              size="icon-sm"
              class="-mt-1 -mr-1 h-7 w-7 rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
              @click="noticeStore.remove(notice.id)"
            >
              <X class="h-3.5 w-3.5" />
              <span class="sr-only">{{ t('admin.common.close') }}</span>
            </Button>
          </div>
          <AlertDescription class="pr-2 text-xs text-muted-foreground leading-relaxed">
            {{ notice.message }}
          </AlertDescription>
        </div>
      </Alert>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.notice-fade-enter-active {
  transition: all 0.28s cubic-bezier(0.16, 1, 0.3, 1);
}

.notice-fade-leave-active {
  transition: all 0.18s cubic-bezier(0.4, 0, 1, 1);
}

.notice-fade-enter-from,
.notice-fade-leave-to {
  opacity: 0;
  transform: translate3d(12px, -6px, 0) scale(0.96);
}
</style>
