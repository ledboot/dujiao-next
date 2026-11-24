<template>
  <nav
    class="fixed top-0 left-0 right-0 z-50 border-b transition-all"
    :class="scrolled
      ? 'bg-background/82 border-border/85 py-2.5 shadow-sm backdrop-blur-xl'
      : 'bg-background/72 border-border/50 py-3.5 backdrop-blur-lg'"
    :style="{ transitionDuration: 'var(--ui-duration-normal)' }">
    <div class="container mx-auto px-4 flex items-center justify-between gap-4">
      <!-- Logo -->
      <router-link to="/" class="theme-wordmark group relative gap-2.5" :title="brandSiteName">
        <img
          v-if="brandLogo"
          :src="brandLogo"
          :alt="brandSiteName"
          class="h-7 max-w-[168px] shrink-0 object-contain transition-transform duration-200 group-hover:scale-[1.02]"
        />
        <span v-else class="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary text-xs font-bold text-primary-foreground shadow-sm">
          {{ brandSiteName.charAt(0).toUpperCase() }}
        </span>
        <span class="theme-wordmark-text">{{ brandSiteName }}</span>
      </router-link>

      <!-- Desktop Menu -->
      <div class="hidden lg:flex items-center gap-1 min-w-0 overflow-x-auto scrollbar-hide rounded-full border border-border/60 bg-secondary/55 p-1">
        <template v-for="item in menuItems" :key="item.key">
          <Button v-if="item.type === 'route'" as-child variant="ghost" size="sm"
            class="h-8 rounded-full px-3.5 gap-1.5 text-muted-foreground hover:text-foreground hover:bg-card/80 whitespace-nowrap shrink-0">
            <router-link :to="item.path" active-class="!text-foreground !bg-card !shadow-sm font-semibold">
              <component :is="item.icon" class="w-3.5 h-3.5 shrink-0 opacity-75" />
              <span>{{ item.label }}</span>
            </router-link>
          </Button>
          <Button v-else as-child variant="ghost" size="sm"
            class="h-8 rounded-full px-3.5 gap-1.5 text-muted-foreground hover:text-foreground hover:bg-card/80 whitespace-nowrap shrink-0">
            <a :href="item.path" :target="item.target" rel="noopener noreferrer">
              <component :is="item.icon" class="w-3.5 h-3.5 shrink-0 opacity-75" />
              <span>{{ item.label }}</span>
            </a>
          </Button>
        </template>
      </div>

      <!-- Right Side Actions -->
      <div class="flex items-center shrink-0 gap-1.5 lg:gap-2">
        <!-- Cart (desktop only, mobile has bottom nav) -->
        <Button as-child variant="ghost" size="sm" class="hidden lg:flex relative rounded-full px-3 gap-1.5 text-muted-foreground hover:text-foreground">
          <router-link to="/cart">
            <ShoppingCart class="w-4 h-4 shrink-0" />
            <span class="text-xs font-medium">{{ t('navbar.cart') }}</span>
            <span v-if="cartCount > 0"
              class="absolute -top-1 -right-1 inline-flex items-center justify-center rounded-full px-1.5 py-0.5 text-[10px] font-bold leading-none min-w-[1.1rem] bg-primary text-primary-foreground shadow-sm"
              :class="{ 'theme-bounce-in': cartBounce }">
              {{ cartCount }}
            </span>
          </router-link>
        </Button>

        <Button v-if="!userAuthStore.isAuthenticated" as-child variant="ghost" size="sm"
          class="hidden lg:inline-flex rounded-full px-3 gap-1.5 text-muted-foreground hover:text-foreground whitespace-nowrap">
          <router-link to="/guest/orders">
            <ClipboardList class="w-4 h-4 shrink-0 opacity-75" />
            {{ t('navbar.guestOrders') }}
          </router-link>
        </Button>
        <Button v-if="!userAuthStore.isAuthenticated" as-child size="sm"
          class="hidden lg:inline-flex rounded-full px-4 gap-1.5 whitespace-nowrap">
          <router-link to="/auth/login">
            <LogIn class="w-3.5 h-3.5 shrink-0" />
            {{ t('navbar.login') }}
          </router-link>
        </Button>
        <Button v-if="userAuthStore.isAuthenticated" as-child variant="secondary" size="sm"
          class="hidden lg:inline-flex rounded-full px-3.5 gap-1.5 whitespace-nowrap">
          <router-link to="/me">
            <User class="w-3.5 h-3.5 shrink-0 opacity-80" />
            {{ t('navbar.personalCenter') }}
          </router-link>
        </Button>
        <Button v-if="userAuthStore.isAuthenticated" variant="ghost" size="icon-sm"
          class="hidden lg:inline-flex rounded-full text-muted-foreground hover:text-destructive hover:bg-destructive/10"
          :title="t('navbar.logout')"
          :aria-label="t('navbar.logout')"
          @click="userAuthStore.logout()">
          <LogOut class="w-4 h-4 shrink-0" />
        </Button>
        <!-- Theme Switcher -->
        <Button variant="ghost" size="icon-sm" class="rounded-full text-muted-foreground hover:text-foreground" @click="toggleTheme">
          <Sun v-if="theme === 'dark'" class="w-4 h-4 transition-transform duration-300 hover:rotate-12" />
          <Moon v-else class="w-4 h-4 transition-transform duration-300 hover:-rotate-12" />
        </Button>

        <!-- Language Switcher (Desktop) -->
        <Popover v-model:open="langOpen">
          <PopoverTrigger as-child>
            <Button variant="ghost" size="sm" class="hidden lg:inline-flex rounded-full px-2.5 gap-1.5 text-muted-foreground hover:text-foreground">
              <Languages class="w-4 h-4" />
              <span class="text-xs font-semibold uppercase tracking-wider">{{ currentLocale }}</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent align="end" class="w-44 p-1.5">
            <div class="px-2.5 pb-1.5 pt-1 mb-1 border-b border-border/70">
              <span class="text-[11px] text-muted-foreground font-medium uppercase tracking-wider">{{ t('navbar.selectLanguage') }}</span>
            </div>
            <button v-for="lang in languages" :key="lang.code" @click="changeLanguage(lang.code)"
              class="w-full text-left px-3 py-2 text-sm rounded-lg transition-colors flex items-center justify-between hover:bg-accent hover:text-accent-foreground"
              :class="{ 'text-primary font-semibold bg-primary/5': appStore.locale === lang.code }">
              {{ lang.name }}
              <span v-if="appStore.locale === lang.code" class="w-1.5 h-1.5 rounded-full bg-primary"></span>
            </button>
          </PopoverContent>
        </Popover>

        <!-- Mobile Menu Button (more menu, not main nav) -->
        <Button variant="ghost" size="icon-sm" class="lg:hidden rounded-full text-muted-foreground [&_svg]:size-5"
          @click="toggleMobileMenu">
          <EllipsisVertical />
        </Button>
      </div>
    </div>

  </nav>

  <!-- Teleport drawer outside nav to avoid backdrop-filter containing block bug -->
  <Teleport to="body">
    <!-- Mobile Drawer Overlay -->
    <Transition
      enter-active-class="transition duration-300 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-200 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0">
      <div v-if="showMobileMenu" class="lg:hidden fixed inset-0 z-[60] bg-black/45 backdrop-blur-xs" @click="showMobileMenu = false" style="overscroll-behavior: none;"></div>
    </Transition>

    <!-- Mobile Drawer (only items NOT in bottom nav) -->
    <Transition
      enter-active-class="transition duration-300 ease-[cubic-bezier(0.16,1,0.3,1)]"
      enter-from-class="translate-x-full"
      enter-to-class="translate-x-0"
      leave-active-class="transition duration-200 ease-in"
      leave-from-class="translate-x-0"
      leave-to-class="translate-x-full">
      <div v-if="showMobileMenu"
        class="lg:hidden fixed right-0 top-0 bottom-0 z-[70] w-72 max-w-[82vw] bg-card/95 backdrop-blur-2xl border-l shadow-2xl overflow-y-auto"
        style="overscroll-behavior: none;">
        <div class="p-5 space-y-1">
          <!-- Header -->
          <div class="flex items-center justify-between mb-4">
            <span class="text-xs font-semibold text-muted-foreground uppercase tracking-wider">{{ t('navbar.more') }}</span>
            <Button variant="secondary" size="icon-sm" class="rounded-full [&_svg]:size-4" @click="showMobileMenu = false">
              <X />
            </Button>
          </div>

          <!-- Navigation items not in bottom nav -->
          <template v-for="item in mobileDrawerItems" :key="item.key">
            <Button v-if="item.type === 'route'" as-child variant="ghost"
              class="w-full justify-start gap-3 h-auto py-3 rounded-xl text-sm text-muted-foreground [&_svg]:size-4.5">
              <router-link :to="item.path" @click="showMobileMenu = false" active-class="!text-primary !bg-primary/10 font-semibold">
                <component :is="item.icon" class="shrink-0 opacity-70" />
                {{ item.label }}
              </router-link>
            </Button>
            <Button v-else as-child variant="ghost"
              class="w-full justify-start gap-3 h-auto py-3 rounded-xl text-sm text-muted-foreground [&_svg]:size-4.5">
              <a :href="item.path" :target="item.target" rel="noopener noreferrer" @click="showMobileMenu = false">
                <component :is="item.icon" class="shrink-0 opacity-70" />
                {{ item.label }}
              </a>
            </Button>
          </template>

          <!-- Guest orders (not in bottom nav) -->
          <Button v-if="!userAuthStore.isAuthenticated" as-child variant="ghost"
            class="w-full justify-start gap-3 h-auto py-3 rounded-xl text-sm text-muted-foreground [&_svg]:size-4.5">
            <router-link to="/guest/orders" @click="showMobileMenu = false" active-class="!text-primary !bg-primary/10 font-semibold">
              <ClipboardList class="shrink-0 opacity-70" />
              {{ t('navbar.guestOrders') }}
            </router-link>
          </Button>

          <!-- Logout (login/me already in bottom nav) -->
          <Button v-if="userAuthStore.isAuthenticated" variant="ghost"
            class="w-full justify-start gap-3 h-auto py-3 rounded-xl text-sm text-destructive hover:text-destructive hover:bg-destructive/10 [&_svg]:size-4.5"
            @click="userAuthStore.logout(); showMobileMenu = false">
            <LogOut class="shrink-0 opacity-70" />
            {{ t('navbar.logout') }}
          </Button>

          <!-- Language Switcher -->
          <div class="mt-4 pt-4 border-t border-border/75">
            <span class="text-xs text-muted-foreground font-semibold uppercase tracking-wider px-3">{{ t('navbar.selectLanguage') }}</span>
            <div class="mt-2 space-y-1">
              <button v-for="lang in languages" :key="lang.code" @click="changeLanguage(lang.code)"
                class="w-full text-left px-3.5 py-2.5 rounded-xl text-sm transition-colors min-h-[42px] flex items-center justify-between"
                :class="appStore.locale === lang.code
                  ? 'text-primary bg-primary/10 font-semibold'
                  : 'text-muted-foreground hover:bg-secondary hover:text-foreground'">
                {{ lang.name }}
                <span v-if="appStore.locale === lang.code"
                  class="w-1.5 h-1.5 rounded-full bg-primary"></span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useCartStore } from '../stores/cart'
import { useUserAuthStore } from '../stores/userAuth'
import { useTheme } from '../utils/theme'
import { getImageUrl } from '../utils/image'
import { useNavConfig } from '../composables/useNavConfig'
import {
  Sun, Moon, ShoppingCart, ClipboardList, LogIn, User, LogOut, Languages,
  EllipsisVertical, X,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const { t } = useI18n()
const appStore = useAppStore()
const cartStore = useCartStore()
const userAuthStore = useUserAuthStore()
const { theme, toggleTheme } = useTheme()
const { primaryNavItems, secondaryNavItems } = useNavConfig()

const showMobileMenu = ref(false)
const langOpen = ref(false)
const scrolled = ref(false)
const cartBounce = ref(false)

const menuItems = primaryNavItems

// Mobile drawer only shows items NOT in the bottom nav (Home, Products, Cart, Me are in bottom nav)
const mobileDrawerItems = secondaryNavItems

const languages = [
  { code: 'zh-CN', name: '简体中文' },
  { code: 'zh-TW', name: '繁體中文' },
  { code: 'en-US', name: 'English' },
]

const currentLocale = computed(() => {
  const lang = languages.find(l => l.code === appStore.locale)
  if (!lang) return 'CN'
  return lang.code === 'en-US' ? 'EN' : (lang.code === 'zh-CN' ? '简' : '繁')
})

const cartCount = computed(() => cartStore.totalItems)

const brandSiteName = computed(() => {
  const text = String(appStore.config?.brand?.site_name || '').trim()
  return text !== '' ? text : 'Dujiao-Next'
})

const brandLogo = computed(() => {
  const raw = String(appStore.config?.brand?.site_logo || '').trim()
  return raw ? getImageUrl(raw) : ''
})

const toggleMobileMenu = () => {
  showMobileMenu.value = !showMobileMenu.value
}

const changeLanguage = (langCode: string) => {
  appStore.setLocale(langCode)
  langOpen.value = false
}

const handleScroll = () => {
  scrolled.value = window.scrollY > 20
}

// Cart badge bounce animation on count change
watch(cartCount, (newVal, oldVal) => {
  if (newVal > oldVal) {
    cartBounce.value = true
    setTimeout(() => { cartBounce.value = false }, 400)
  }
})

onMounted(() => {
  window.addEventListener('scroll', handleScroll, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
})
</script>

<style scoped>
.scrollbar-hide {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.scrollbar-hide::-webkit-scrollbar {
  display: none;
}
</style>
