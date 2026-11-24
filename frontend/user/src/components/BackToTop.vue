<template>
  <Transition
    enter-active-class="transition duration-300 ease-[cubic-bezier(0.16,1,0.3,1)]"
    enter-from-class="opacity-0 scale-90 translate-y-2"
    enter-to-class="opacity-100 scale-100 translate-y-0"
    leave-active-class="transition duration-200 ease-in"
    leave-from-class="opacity-100 scale-100 translate-y-0"
    leave-to-class="opacity-0 scale-90 translate-y-2"
  >
    <Button
      v-if="visible"
      variant="outline"
      size="icon"
      @click="scrollToTop"
      class="back-to-top fixed right-4 md:right-6 z-30 lg:z-40 h-10 w-10 rounded-full bg-card/88 backdrop-blur-xl border-border/85 shadow-lg transition-all duration-200 hover:border-primary/40 hover:text-primary hover:shadow-xl hover:-translate-y-0.5 [&_svg]:size-4.5"
      :aria-label="t('common.backToTop')"
    >
      <ChevronUp />
    </Button>
  </Transition>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronUp } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const { t } = useI18n()
const visible = ref(false)

const onScroll = () => {
  visible.value = window.scrollY > 400
}

const scrollToTop = () => {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(() => window.addEventListener('scroll', onScroll, { passive: true }))
onUnmounted(() => window.removeEventListener('scroll', onScroll))
</script>

<style scoped>
/* Mobile: sit above bottom nav (h-14 = 3.5rem) + safe area, with breathing room */
.back-to-top {
  bottom: calc(3.5rem + env(safe-area-inset-bottom, 0px) + 1rem);
}
@media (min-width: 1024px) {
  .back-to-top {
    bottom: 1.5rem;
  }
}
</style>
