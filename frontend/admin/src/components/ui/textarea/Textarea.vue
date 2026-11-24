<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { useVModel } from "@vueuse/core"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
  defaultValue?: string | number
  modelValue?: string | number
}>()

const emits = defineEmits<{
  (e: "update:modelValue", payload: string | number): void
}>()

const modelValue = useVModel(props, "modelValue", emits, {
  passive: true,
  defaultValue: props.defaultValue,
})
</script>

<template>
  <textarea v-model="modelValue" :class="cn('block min-h-[68px] w-full min-w-0 max-w-full rounded-lg border border-input bg-background/60 px-3 py-2 text-base md:text-sm transition-all duration-150 placeholder:text-muted-foreground/80 hover:border-foreground/20 focus-visible:outline-none focus-visible:border-primary/60 focus-visible:bg-card focus-visible:ring-[3px] focus-visible:ring-ring/15 disabled:cursor-not-allowed disabled:opacity-50', props.class)" />
</template>
