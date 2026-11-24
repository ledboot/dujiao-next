<template>
  <div class="p-5 md:p-8 bg-secondary/45 border-b lg:border-b-0 lg:border-r border-border/75">
    <div class="mb-4 md:mb-5 relative group overflow-hidden rounded-2xl border border-border/70 bg-muted/60 shadow-sm"
      @touchstart="onImageTouchStart"
      @touchend="onImageTouchEnd">
      <Transition name="gallery-fade" mode="out-in">
        <img v-if="currentImage && !imageErrored" :key="currentImage" :src="currentImage" :alt="productTitle"
          class="w-full aspect-[4/3] object-cover relative z-10"
          @error="imageErrored = true" />
        <div v-else
          class="w-full aspect-[4/3] flex items-center justify-center relative z-10 text-muted-foreground/55">
          <ImageIcon class="w-16 h-16 md:w-20 md:h-20" :stroke-width="1.5" />
        </div>
      </Transition>
    </div>

    <!-- Thumbnail Gallery: horizontal scroll on mobile, grid on desktop -->
    <div v-if="images.length > 1" class="flex md:grid md:grid-cols-5 gap-2.5 overflow-x-auto md:overflow-visible pb-2 md:pb-0 -mx-1 px-1 snap-x snap-mandatory">
      <button v-for="(image, index) in images" :key="index" type="button" @click="$emit('update:currentImage', image)"
        class="cursor-pointer rounded-xl overflow-hidden border-2 transition-all duration-200 shrink-0 w-16 h-16 md:w-auto md:h-auto md:aspect-square snap-start"
        :class="currentImage === image ? 'border-primary ring-2 ring-primary/25 opacity-100 scale-[1.02]' : 'border-transparent opacity-65 hover:opacity-100 hover:scale-[1.02]'">
        <img :src="image" :alt="`Image ${index + 1}`" loading="lazy" class="w-full h-full object-cover" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Image as ImageIcon } from 'lucide-vue-next'

const props = defineProps<{
  images: string[]
  currentImage: string
  productTitle: string
}>()

const emit = defineEmits<{
  'update:currentImage': [value: string]
}>()

const imageErrored = ref(false)
watch(() => props.currentImage, () => {
  imageErrored.value = false
})

// Image touch swipe
let touchStartX = 0
const onImageTouchStart = (e: TouchEvent) => {
  touchStartX = e.touches[0]?.clientX ?? 0
}
const onImageTouchEnd = (e: TouchEvent) => {
  const touchEndX = e.changedTouches[0]?.clientX ?? 0
  const diff = touchStartX - touchEndX
  if (Math.abs(diff) < 50) return
  if (props.images.length <= 1) return
  const currentIdx = props.images.indexOf(props.currentImage)
  if (currentIdx === -1) return
  if (diff > 0) {
    // Swipe left -> next
    emit('update:currentImage', props.images[(currentIdx + 1) % props.images.length] ?? '')
  } else {
    // Swipe right -> prev
    emit('update:currentImage', props.images[(currentIdx - 1 + props.images.length) % props.images.length] ?? '')
  }
}
</script>

<style scoped>
.gallery-fade-enter-active,
.gallery-fade-leave-active {
  transition: opacity 220ms cubic-bezier(0.16, 1, 0.3, 1);
}
.gallery-fade-enter-from,
.gallery-fade-leave-to {
  opacity: 0;
}
</style>
