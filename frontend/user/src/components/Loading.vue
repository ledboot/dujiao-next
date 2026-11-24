<template>
  <transition name="fade">
    <div v-if="loading"
      class="fixed inset-0 z-50 flex items-center justify-center theme-overlay backdrop-blur-xl transition-all duration-300">
      <div class="relative flex flex-col items-center gap-5">
        <div class="relative w-12 h-12">
          <div class="absolute inset-0 rounded-full border-2 border-border/70"></div>
          <div
            class="absolute inset-0 rounded-full border-2 border-transparent border-t-primary animate-spin">
          </div>
          <div class="absolute inset-0 flex items-center justify-center">
            <div class="w-2 h-2 rounded-full bg-primary/80 animate-pulse"></div>
          </div>
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
defineProps<{
  loading: boolean
}>()
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.25s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
