<template>
  <Teleport to="body">
    <div class="fixed z-[100] pointer-events-none" :class="positionClass">
      <TransitionGroup
        enter-active-class="transition duration-300 ease-[cubic-bezier(0.34,1.56,0.64,1)]"
        enter-from-class="opacity-0 translate-y-2 scale-95"
        enter-to-class="opacity-100 translate-y-0 scale-100"
        leave-active-class="transition duration-200 ease-in"
        leave-from-class="opacity-100 translate-y-0 scale-100"
        leave-to-class="opacity-0 translate-y-2 scale-95"
      >
        <div
          v-for="item in toasts"
          :key="item.id"
          role="alert"
          class="pointer-events-auto mb-2.5 flex items-center gap-2.5 rounded-2xl border bg-card/95 px-4 py-3 text-sm font-medium shadow-xl backdrop-blur-xl"
          :class="typeClass(item.type)"
        >
          <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-full" :class="iconWrapClass(item.type)">
            <Check v-if="item.type === 'success'" class="h-3.5 w-3.5" />
            <X v-else-if="item.type === 'error'" class="h-3.5 w-3.5" />
            <Info v-else class="h-3.5 w-3.5" />
          </span>
          <span class="flex-1 text-foreground">{{ item.message }}</span>
          <button
            v-if="item.action"
            class="ml-2 shrink-0 rounded-lg px-2.5 py-1 text-xs font-semibold text-primary underline underline-offset-2 transition-colors hover:opacity-80"
            @click="handleAction(item)"
          >
            {{ item.action.label }}
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { Check, X, Info } from 'lucide-vue-next'
import { useToast, type ToastItem } from '../composables/useToast'

const { toasts, removeToast } = useToast()

const positionClass = 'bottom-6 left-1/2 -translate-x-1/2 md:bottom-auto md:top-6 flex flex-col items-center'

const typeClass = (type: string) => {
  switch (type) {
    case 'success':
      return 'border-success/30'
    case 'error':
      return 'border-destructive/30'
    default:
      return 'border-border/85'
  }
}

const iconWrapClass = (type: string) => {
  switch (type) {
    case 'success':
      return 'bg-success/15 text-success'
    case 'error':
      return 'bg-destructive/15 text-destructive'
    default:
      return 'bg-primary/15 text-primary'
  }
}

const handleAction = (item: ToastItem) => {
  item.action?.onClick()
  removeToast(item.id)
}
</script>
