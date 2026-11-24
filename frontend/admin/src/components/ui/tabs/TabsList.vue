<script setup lang="ts">
import type { TabsListProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { TabsList } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<TabsListProps & { class?: HTMLAttributes["class"] }>()

const delegatedProps = reactiveOmit(props, "class")
</script>

<template>
  <TabsList
    data-slot="tabs-list"
    v-bind="delegatedProps"
    :class="cn(
      'bg-muted/75 text-muted-foreground no-scrollbar inline-flex h-10 max-w-full items-center justify-start overflow-x-auto overscroll-x-contain rounded-xl p-1 sm:h-9.5 sm:w-fit sm:justify-center',
      props.class,
    )"
  >
    <slot />
  </TabsList>
</template>
