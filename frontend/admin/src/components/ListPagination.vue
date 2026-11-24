<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const props = defineProps<{
  page: number
  totalPage: number
  total: number
  pageSize?: number
  pageSizeOptions?: number[]
}>()

const emit = defineEmits<{
  changePage: [page: number]
  changePageSize: [pageSize: number]
}>()

const { t } = useI18n()
const jumpPage = ref('')
const normalizedPageSizeOptions = computed(() => {
  const source = Array.isArray(props.pageSizeOptions) ? props.pageSizeOptions : []
  return source
    .map((item) => Number(item))
    .filter((item, index, array) => Number.isFinite(item) && item > 0 && array.indexOf(item) === index)
    .sort((a, b) => a - b)
})
const pageSizeValue = computed(() => {
  const raw = Number(props.pageSize || 0)
  if (!Number.isFinite(raw) || raw <= 0) return ''
  return String(Math.floor(raw))
})
const shouldShow = computed(() => {
  if (props.total <= 0) return false
  return props.totalPage > 1 || normalizedPageSizeOptions.value.length > 0
})

const jumpToPage = () => {
  if (!jumpPage.value) return
  const raw = Number(jumpPage.value)
  if (Number.isNaN(raw)) return
  const target = Math.min(Math.max(Math.floor(raw), 1), props.totalPage)
  if (target === props.page) return
  emit('changePage', target)
}

const changePageSize = (value: unknown) => {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return
  emit('changePageSize', Math.floor(parsed))
}
</script>

<template>
  <div
    v-if="shouldShow"
    class="flex flex-col gap-2.5 border-t border-border/70 bg-muted/15 px-3.5 py-3 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:px-5 sm:py-3.5"
  >
    <div class="flex flex-wrap items-center justify-between gap-2 sm:justify-start sm:gap-3">
      <span class="text-xs tabular-nums text-muted-foreground">
        {{ t('admin.common.pageInfo', { total, page, totalPage }) }}
      </span>
      <slot name="actions" />
    </div>
    <div class="flex flex-wrap items-center justify-between gap-2 sm:justify-end">
      <div class="flex items-center gap-1.5 sm:gap-2">
        <div v-if="normalizedPageSizeOptions.length > 0" class="flex items-center gap-1.5">
          <span class="hidden text-xs text-muted-foreground sm:inline">{{ t('admin.common.pageSize') }}</span>
          <Select :model-value="pageSizeValue" @update:modelValue="changePageSize">
            <SelectTrigger class="h-8 w-[72px] px-2.5 text-xs sm:w-[88px] sm:px-3">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="size in normalizedPageSizeOptions" :key="size" :value="String(size)">
                {{ size }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div v-if="totalPage > 1" class="flex items-center gap-1.5">
          <Input
            v-model="jumpPage"
            type="number"
            inputmode="numeric"
            min="1"
            :max="totalPage"
            class="h-8 w-14 px-2 text-center text-base md:text-xs tabular-nums sm:w-18"
            :placeholder="t('admin.common.jumpPlaceholder')"
            @keydown.enter="jumpToPage"
          />
          <Button variant="outline" size="sm" class="h-8 px-2.5 sm:px-3" @click="jumpToPage">
            {{ t('admin.common.jumpTo') }}
          </Button>
        </div>
      </div>
      <div class="flex items-center gap-1.5 ml-auto sm:ml-0">
        <Button
          variant="outline"
          size="sm"
          class="h-8 px-2.5 sm:px-3"
          :disabled="page <= 1 || totalPage <= 1"
          @click="emit('changePage', page - 1)"
        >
          {{ t('admin.common.prevPage') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          class="h-8 px-2.5 sm:px-3"
          :disabled="page >= totalPage || totalPage <= 1"
          @click="emit('changePage', page + 1)"
        >
          {{ t('admin.common.nextPage') }}
        </Button>
      </div>
    </div>
  </div>
</template>
