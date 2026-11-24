<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <div
    :class="
      cn(
        'rounded-2xl border border-border/80 bg-card text-card-foreground shadow-[0_1px_2px_rgba(15,23,42,0.03),0_8px_24px_-8px_rgba(15,23,42,0.05)] dark:shadow-[0_1px_2px_rgba(0,0,0,0.3),0_12px_28px_-10px_rgba(0,0,0,0.45)] transition-[border-color,box-shadow,transform] duration-200',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
