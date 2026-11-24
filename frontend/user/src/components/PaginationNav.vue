<template>
  <div v-if="totalPages > 1" class="flex justify-center" :class="compact ? 'mt-8' : 'mt-12'">
    <nav
      class="flex items-center gap-1.5 rounded-full border border-border/80 bg-card/85 px-2 py-1.5 shadow-sm backdrop-blur-xl"
      :aria-label="t('pagination.label')"
    >
      <Button
        type="button"
        variant="ghost"
        size="icon"
        class="rounded-full"
        :class="compact ? 'h-9 w-9' : 'h-10 w-10'"
        :aria-label="t('pagination.previous')"
        :disabled="currentPage === 1 || loading"
        @click="handlePage(currentPage - 1)"
      >
        <ChevronLeft />
      </Button>

      <span
        class="flex items-center tabular-nums text-muted-foreground"
        :class="compact ? 'px-3 py-1 text-xs' : 'px-4 py-1.5 text-sm'"
        aria-live="polite"
      >
        <Loader2
          v-if="loading"
          class="mr-2 animate-spin text-primary"
          :class="compact ? 'h-3.5 w-3.5' : 'h-4 w-4'"
          aria-hidden="true"
        />
        <span class="font-bold text-foreground">{{ currentPage }}</span>
        <span :class="compact ? 'mx-1.5' : 'mx-2'" class="opacity-40">/</span>
        <span>{{ totalPages }}</span>
      </span>

      <Button
        type="button"
        variant="ghost"
        size="icon"
        class="rounded-full"
        :class="compact ? 'h-9 w-9' : 'h-10 w-10'"
        :aria-label="t('pagination.next')"
        :disabled="currentPage === totalPages || loading"
        @click="handlePage(currentPage + 1)"
      >
        <ChevronRight />
      </Button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, Loader2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  currentPage: number
  totalPages: number
  compact?: boolean
  loading?: boolean
  scrollTop?: boolean
}>()

const emit = defineEmits<{
  'changePage': [page: number]
}>()

const { t } = useI18n()

const handlePage = (page: number) => {
  if (page < 1 || page > props.totalPages || page === props.currentPage) return
  emit('changePage', page)
  if (props.scrollTop !== false) {
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }
}
</script>
