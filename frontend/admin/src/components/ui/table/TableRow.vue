<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <tr :class="cn('border-b border-border/70 transition-colors duration-150 hover:bg-muted/45 data-[state=selected]:bg-primary/5', props.class)">
    <slot />
  </tr>
</template>
