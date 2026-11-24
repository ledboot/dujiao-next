<template>
  <nav class="lg:hidden fixed bottom-0 left-0 right-0 z-40 bg-card/88 backdrop-blur-2xl border-t border-border/80 theme-safe-bottom">
    <div class="flex items-stretch h-14">
      <router-link
        v-for="item in navItems"
        :key="item.path"
        :to="item.path"
        class="relative flex-1 flex flex-col items-center justify-center gap-0.5 text-[11px] transition-all duration-200 min-h-[44px] active:scale-95"
        :class="isActive(item.path) ? 'text-primary font-semibold' : 'text-muted-foreground hover:text-foreground'"
      >
        <span
          class="absolute top-0 h-0.5 rounded-full bg-primary transition-all duration-300"
          :class="isActive(item.path) ? 'w-5 opacity-100' : 'w-0 opacity-0'"
        />
        <!-- Home -->
        <Home v-if="item.icon === 'home'" class="w-5 h-5 transition-transform duration-200" :class="isActive(item.path) ? '-translate-y-0.5' : ''" />
        <!-- Products -->
        <LayoutGrid v-else-if="item.icon === 'products'" class="w-5 h-5 transition-transform duration-200" :class="isActive(item.path) ? '-translate-y-0.5' : ''" />
        <!-- Cart -->
        <div v-else-if="item.icon === 'cart'" class="relative transition-transform duration-200" :class="isActive(item.path) ? '-translate-y-0.5' : ''">
          <ShoppingCart class="w-5 h-5" />
          <span v-if="cartCount > 0"
            class="absolute -top-1.5 -right-2.5 bg-primary text-primary-foreground font-bold text-[10px] min-w-[16px] h-4 flex items-center justify-center rounded-full px-1 shadow-xs">
            {{ cartCount > 99 ? '99+' : cartCount }}
          </span>
        </div>
        <!-- Me -->
        <User v-else-if="item.icon === 'me'" class="w-5 h-5 transition-transform duration-200" :class="isActive(item.path) ? '-translate-y-0.5' : ''" />
        <span>{{ t(item.label) }}</span>
      </router-link>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Home, LayoutGrid, ShoppingCart, User } from 'lucide-vue-next'
import { useCartStore } from '../stores/cart'
import { useUserAuthStore } from '../stores/userAuth'
import { useAppStore } from '../stores/app'

const route = useRoute()
const { t } = useI18n()
const cartStore = useCartStore()
const userAuthStore = useUserAuthStore()
const appStore = useAppStore()

const cartCount = computed(() => cartStore.totalItems)
const isListMode = computed(() => appStore.config?.template_mode === 'list')

const navItems = computed(() => {
  const items = [
    { path: '/', icon: 'home', label: 'bottomNav.home' },
    ...(!isListMode.value ? [{ path: '/products', icon: 'products', label: 'bottomNav.products' }] : []),
    { path: '/cart', icon: 'cart', label: 'bottomNav.cart' },
    { path: userAuthStore.isAuthenticated ? '/me' : '/auth/login', icon: 'me', label: 'bottomNav.me' },
  ]
  return items
})

const isActive = (path: string) => {
  if (path === '/') return route.path === '/'
  return route.path.startsWith(path)
}
</script>
