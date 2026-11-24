<script setup lang="ts">
import type { SelectTriggerProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { ChevronDown } from "lucide-vue-next"
import { SelectIcon, SelectTrigger, useForwardProps } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<SelectTriggerProps & { class?: HTMLAttributes["class"] }>()

const delegatedProps = reactiveOmit(props, "class")

const forwardedProps = useForwardProps(delegatedProps)
</script>

<template>
  <SelectTrigger
    v-bind="forwardedProps"
    :class="cn(
      'flex h-9.5 sm:h-9 w-full min-w-0 max-w-full items-center justify-between whitespace-nowrap rounded-lg border border-input bg-background/60 px-3 py-2 text-sm transition-all duration-150 ring-offset-background data-[placeholder]:text-muted-foreground hover:border-foreground/20 focus:outline-none focus:border-primary/60 focus:bg-card focus:ring-[3px] focus:ring-ring/15 disabled:cursor-not-allowed disabled:opacity-50 [&>span]:min-w-0 [&>span]:truncate text-start',
      props.class,
    )"
  >
    <slot />
    <SelectIcon as-child>
      <ChevronDown class="w-4 h-4 opacity-50 shrink-0 transition-transform duration-200" />
    </SelectIcon>
  </SelectTrigger>
</template>
