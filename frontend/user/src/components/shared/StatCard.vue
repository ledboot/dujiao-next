<template>
  <Card class="group relative overflow-hidden border-border/80 transition-all duration-250 hover:-translate-y-0.5 hover:border-primary/25 hover:shadow-md">
    <div class="flex items-start justify-between gap-3 p-4 sm:p-5">
      <div class="min-w-0">
        <p class="truncate text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">{{ label }}</p>
        <div class="mt-2 truncate text-xl font-bold tracking-tight leading-tight text-foreground tabular-nums sm:text-2xl" :class="{ 'font-mono': mono }">
          <slot name="value">{{ value }}</slot>
        </div>
        <p v-if="hint" class="mt-1 truncate text-xs text-muted-foreground">{{ hint }}</p>
      </div>
      <span
        v-if="icon"
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl transition-transform duration-250 group-hover:scale-105"
        :class="toneClass"
      >
        <component :is="icon" class="h-5 w-5" />
      </span>
    </div>
  </Card>
</template>

<script setup lang="ts">
import { computed, type Component } from 'vue'
import { Card } from '@/components/ui/card'

const props = withDefaults(
  defineProps<{
    label: string
    value?: string | number
    hint?: string
    icon?: Component
    tone?: 'accent' | 'success' | 'warning' | 'info' | 'neutral'
    mono?: boolean
  }>(),
  { tone: 'accent', mono: false },
)

const toneClass = computed(
  () =>
    ({
      success: 'bg-success/10 text-success',
      warning: 'bg-warning/10 text-warning',
      info: 'bg-info/10 text-info',
      neutral: 'bg-muted text-muted-foreground',
      accent: 'bg-primary/10 text-primary',
    })[props.tone],
)
</script>
