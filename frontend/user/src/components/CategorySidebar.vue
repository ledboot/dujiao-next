<template>
  <!-- Mobile Filter Button -->
  <Button variant="outline" @click="$emit('update:showDrawer', true)"
    class="lg:hidden gap-2 rounded-xl text-sm font-medium min-h-[42px] bg-card">
    <Filter class="w-4 h-4" />
    {{ t('products.filter') }}
    <span v-if="selectedCategory" class="w-2 h-2 rounded-full bg-primary"></span>
  </Button>

  <!-- Mobile Filter Drawer Overlay -->
  <Transition
    enter-active-class="transition duration-300 ease-out"
    enter-from-class="opacity-0"
    enter-to-class="opacity-100"
    leave-active-class="transition duration-200 ease-in"
    leave-from-class="opacity-100"
    leave-to-class="opacity-0">
    <div v-if="showDrawer" class="lg:hidden fixed inset-0 z-40 bg-black/45 backdrop-blur-xs cursor-pointer"
      @click="$emit('update:showDrawer', false)" style="overscroll-behavior: none;"></div>
  </Transition>

  <!-- Mobile Filter Drawer -->
  <Transition
    enter-active-class="transition duration-300 ease-[cubic-bezier(0.16,1,0.3,1)]"
    enter-from-class="-translate-x-full"
    enter-to-class="translate-x-0"
    leave-active-class="transition duration-200 ease-in"
    leave-from-class="translate-x-0"
    leave-to-class="-translate-x-full">
    <div v-if="showDrawer"
      class="lg:hidden fixed left-0 top-0 bottom-0 z-50 w-72 max-w-[82vw] bg-card/95 backdrop-blur-2xl border-r shadow-2xl overflow-y-auto"
      style="overscroll-behavior: none;">
      <div class="p-5">
        <div class="flex items-center justify-between mb-5">
          <span class="text-sm font-bold text-foreground flex items-center gap-2">
            <span class="w-1 h-4 bg-primary rounded-full"></span>
            {{ showSearch ? t('products.filter') : t('products.categories') }}
          </span>
          <Button variant="secondary" size="icon-sm" class="rounded-full [&_svg]:size-4" @click="$emit('update:showDrawer', false)">
            <X />
          </Button>
        </div>

        <!-- Search in drawer (only for Products page mode) -->
        <div v-if="showSearch" class="mb-5">
          <label class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            {{ t('products.searchLabel') }}
          </label>
          <div class="mt-2 flex items-center gap-2">
            <Input :model-value="searchQuery" @update:model-value="$emit('update:searchQuery', String($event))" type="text"
              class="min-w-0 flex-1"
              :placeholder="t('products.searchPlaceholder')" />
            <Button v-if="searchQuery" variant="secondary" size="sm" type="button" class="shrink-0 whitespace-nowrap rounded-xl text-xs" @click="$emit('clearSearch')">
              {{ t('common.cancel') }}
            </Button>
          </div>
        </div>

        <h2 v-if="showSearch" class="text-xs font-semibold uppercase tracking-wider mb-3 text-muted-foreground">{{ t('products.categories') }}</h2>

        <!-- Category List -->
        <ul class="space-y-1">
          <li>
            <button @click="$emit('selectCategory', null, true)"
              class="w-full text-left px-3.5 py-2.5 rounded-xl text-sm transition-all duration-200 min-h-[42px]"
              :class="selectedCategory === null
                ? 'bg-primary/10 text-primary font-semibold'
                : 'text-muted-foreground hover:text-foreground hover:bg-secondary/80'">
              {{ t('products.allCategories') }}
            </button>
          </li>
          <li v-for="group in categories" :key="group.id">
            <div class="space-y-1">
              <div class="flex items-center gap-1">
                <button @click="$emit('selectCategory', group.id, true)"
                  class="flex-1 min-w-0 text-left px-3.5 py-2.5 rounded-xl text-sm transition-all duration-200 flex items-center gap-2.5 min-h-[42px]"
                  :class="selectedCategory === group.id
                    ? 'bg-primary/10 text-primary font-semibold'
                    : 'text-muted-foreground hover:text-foreground hover:bg-secondary/80'">
                  <img v-if="group.icon" :src="getImageUrl(group.icon)"
                    :alt="getLocalizedText(group.name)"
                    loading="lazy" class="h-4.5 w-4.5 rounded object-cover" />
                  <span class="truncate">{{ getLocalizedText(group.name) }}</span>
                </button>
                <button
                  v-if="group.children.length > 0"
                  type="button"
                  class="h-8 w-8 shrink-0 rounded-lg flex items-center justify-center transition-colors text-muted-foreground hover:bg-secondary hover:text-foreground"
                  @click.stop="$emit('toggleParent', group.id)"
                >
                  <ChevronRight class="w-4 h-4 transition-transform duration-200"
                    :class="expandedParentIds.includes(group.id) ? 'rotate-90 text-primary' : ''" />
                </button>
              </div>
              <ul v-if="group.children.length > 0 && expandedParentIds.includes(group.id)" class="space-y-1 ml-4 border-l border-border/70 pl-2.5">
                <li v-for="child in group.children" :key="child.id">
                  <button @click="$emit('selectCategory', child.id, true)"
                    class="w-full text-left px-3 py-2 rounded-xl text-sm transition-all duration-200 flex items-center gap-2 min-h-[38px]"
                    :class="selectedCategory === child.id
                      ? 'bg-primary/10 text-primary font-semibold'
                      : 'text-muted-foreground hover:text-foreground hover:bg-secondary/80'">
                    <img v-if="child.icon" :src="getImageUrl(child.icon)"
                      :alt="getLocalizedText(child.name)"
                      loading="lazy" class="h-4 w-4 rounded object-cover" />
                    <span class="truncate">{{ getLocalizedText(child.name) }}</span>
                  </button>
                </li>
              </ul>
            </div>
          </li>
        </ul>
      </div>
    </div>
  </Transition>

  <!-- Desktop Sidebar -->
  <aside class="hidden lg:block flex-shrink-0" :class="compact ? 'lg:w-60' : 'lg:w-64'">
    <div class="bg-card/90 backdrop-blur-xl border border-border/80 rounded-2xl shadow-sm sticky top-24" :class="compact ? 'p-4' : 'p-5'">
      <!-- Search (desktop, only for Products page) -->
      <div v-if="showSearch" :class="compact ? 'mb-4' : 'mb-5'">
        <label class="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {{ t('products.searchLabel') }}
        </label>
        <div class="mt-2 flex items-center gap-2">
          <Input :model-value="searchQuery" @update:model-value="$emit('update:searchQuery', String($event))" type="text"
            class="min-w-0 flex-1"
            :placeholder="t('products.searchPlaceholder')" />
          <Button v-if="searchQuery" variant="secondary" size="sm" type="button" class="shrink-0 whitespace-nowrap rounded-lg text-xs" @click="$emit('clearSearch')">
            {{ t('common.cancel') }}
          </Button>
        </div>
      </div>

      <h2 :class="compact ? 'text-xs font-semibold uppercase tracking-wider mb-3' : 'text-xs font-semibold uppercase tracking-wider mb-3.5'"
        class="text-muted-foreground flex items-center gap-2 px-1">
        <span class="w-1 h-3.5 bg-primary rounded-full"></span>
        {{ t('products.categories') }}
      </h2>

      <ul class="space-y-1">
        <li>
          <button @click="$emit('selectCategory', null)"
            class="w-full text-left rounded-xl transition-all duration-200 px-3 py-2 text-sm flex items-center justify-between"
            :class="selectedCategory === null
              ? 'bg-primary/10 text-primary font-semibold'
              : 'text-muted-foreground hover:text-foreground hover:bg-secondary/75'">
            <span>{{ t('products.allCategories') }}</span>
            <span v-if="selectedCategory === null" class="h-1.5 w-1.5 rounded-full bg-primary" />
          </button>
        </li>
        <li v-for="group in categories" :key="group.id">
          <div class="space-y-1">
            <div class="flex items-center gap-1">
              <button @click="$emit('selectCategory', group.id)"
                class="flex-1 min-w-0 text-left rounded-xl transition-all duration-200 px-3 py-2 text-sm flex items-center gap-2.5"
                :class="selectedCategory === group.id
                  ? 'bg-primary/10 text-primary font-semibold'
                  : 'text-muted-foreground hover:text-foreground hover:bg-secondary/75'">
                <img v-if="group.icon" :src="getImageUrl(group.icon)"
                  :alt="getLocalizedText(group.name)"
                  loading="lazy" class="h-4 w-4 rounded object-cover shrink-0" />
                <span class="truncate">{{ getLocalizedText(group.name) }}</span>
              </button>
              <button
                v-if="group.children.length > 0"
                type="button"
                class="h-8 w-8 shrink-0 rounded-lg flex items-center justify-center transition-colors text-muted-foreground hover:bg-secondary hover:text-foreground"
                @click.stop="$emit('toggleParent', group.id)"
              >
                <ChevronRight
                  :class="expandedParentIds.includes(group.id) ? 'rotate-90 text-primary' : ''"
                  class="w-3.5 h-3.5 transition-transform duration-200" />
              </button>
            </div>
            <ul v-if="group.children.length > 0 && expandedParentIds.includes(group.id)"
              class="space-y-0.5 ml-3.5 border-l border-border/70 pl-2.5">
              <li v-for="child in group.children" :key="child.id">
                <button @click="$emit('selectCategory', child.id)"
                  class="w-full text-left rounded-lg transition-all duration-200 px-2.5 py-1.5 text-[13px] flex items-center gap-2"
                  :class="selectedCategory === child.id
                    ? 'bg-primary/10 text-primary font-semibold'
                    : 'text-muted-foreground hover:text-foreground hover:bg-secondary/75'">
                  <img v-if="child.icon" :src="getImageUrl(child.icon)"
                    :alt="getLocalizedText(child.name)"
                    loading="lazy" class="h-3.5 w-3.5 rounded object-cover shrink-0" />
                  <span class="truncate">{{ getLocalizedText(child.name) }}</span>
                </button>
              </li>
            </ul>
          </div>
        </li>
      </ul>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Filter, X, ChevronRight } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getImageUrl } from '../utils/image'
import { useLocalized } from '../composables/useProduct'
import type { CategoryGroup } from '../utils/category'

const { t } = useI18n()
const { getLocalizedText } = useLocalized()

defineProps<{
  categories: CategoryGroup[]
  selectedCategory: number | null
  expandedParentIds: number[]
  showDrawer: boolean
  compact?: boolean
  showSearch?: boolean
  searchQuery?: string
}>()

defineEmits<{
  'selectCategory': [categoryId: number | null, closeDrawer?: boolean]
  'toggleParent': [categoryId: number]
  'update:showDrawer': [value: boolean]
  'update:searchQuery': [value: string]
  'clearSearch': []
}>()
</script>
