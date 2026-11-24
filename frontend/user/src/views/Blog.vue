<template>
  <div
    class="blog-page min-h-screen bg-background text-foreground pt-20 pb-16 relative overflow-hidden">
    <div class="container mx-auto px-4 relative z-10">
      <!-- Page Header -->
      <div class="mb-10 mt-8 text-center">
        <h1 class="text-3xl md:text-4xl font-bold mb-3 tracking-tight text-foreground">{{ t('nav.blog') }}</h1>
        <p class="text-muted-foreground max-w-2xl mx-auto text-sm md:text-base leading-relaxed">
          {{ t('blog.subtitle') }}
        </p>
      </div>

      <!-- Search Box -->
      <div class="mb-10 max-w-xl mx-auto">
        <div
          class="relative bg-card/90 backdrop-blur-md border border-border/85 rounded-2xl flex items-center px-4 py-2.5 shadow-sm transition-all duration-200 focus-within:border-primary/60 focus-within:ring-[3px] focus-within:ring-primary/15">
          <Search class="w-4.5 h-4.5 text-muted-foreground shrink-0" />
          <input v-model="searchKeyword" type="text" :placeholder="t('blog.searchPlaceholder')"
            class="flex-1 bg-transparent border-none outline-none px-3 text-sm text-foreground placeholder:text-muted-foreground" />
          <Button v-if="searchKeyword" type="button" variant="ghost" size="sm" @click="searchKeyword = ''">
            {{ t('blog.searchClear') }}
          </Button>
        </div>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <div v-for="i in 6" :key="i"
          class="rounded-2xl h-[300px] theme-skeleton border border-border/60">
        </div>
      </div>

      <!-- Posts Grid -->
      <div v-else-if="posts.length > 0">
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <article v-for="(post, idx) in posts" :key="post.id"
            class="group bg-card border border-border/80 rounded-2xl overflow-hidden shadow-sm transition-all duration-300 hover:-translate-y-1 hover:border-primary/35 hover:shadow-lg cursor-pointer flex flex-col theme-slide-up"
            :style="{ animationDelay: `${idx * 45}ms` }"
            @click="goToPost(post.slug)">
            <!-- Thumbnail -->
            <div v-if="post.thumbnail" class="h-48 overflow-hidden relative border-b border-border/50 bg-muted/60">
              <img :src="getImageUrl(post.thumbnail)" :alt="getLocalizedText(post.title)"
                loading="lazy" class="w-full h-full object-cover transition-transform duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:scale-105">
            </div>

            <div class="p-6 flex flex-col flex-1">
              <div class="flex items-center justify-between mb-4">
                <Badge :variant="post.type === 'blog' ? 'accent' : 'info'" size="sm">
                  {{ post.type === 'blog' ? t('nav.blog') : t('nav.notice') }}
                </Badge>
                <time class="text-xs text-muted-foreground font-mono tabular-nums">
                  {{ formatDate(post.published_at) }}
                </time>
              </div>

              <h2
                class="text-lg font-bold tracking-tight mb-3 text-foreground transition-colors line-clamp-2 leading-snug group-hover:text-primary">
                {{ getLocalizedText(post.title) }}
              </h2>

              <p class="text-muted-foreground text-sm mb-6 line-clamp-3 leading-relaxed flex-1">
                {{ getLocalizedText(post.summary) }}
              </p>

              <div
                class="flex items-center text-sm font-semibold text-primary mt-auto pt-4 border-t border-border/65">
                {{ t('blog.readMore') }}
                <ArrowRight class="w-4 h-4 ml-1.5 transition-transform duration-200 group-hover:translate-x-1" />
              </div>
            </div>
          </article>
        </div>

        <!-- Pagination -->
        <PaginationNav
          :current-page="currentPage"
          :total-pages="totalPages"
          @change-page="changePage"
        />
      </div>

      <!-- Empty State -->
      <EmptyState v-else variant="soft" size="lg" :title="searchKeyword.trim() ? t('blog.noResults') : t('blog.empty')">
        <template #icon>
          <BookOpen class="w-20 h-20 text-muted-foreground opacity-70" :stroke-width="1.5" />
        </template>
      </EmptyState>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowRight, BookOpen, Search } from 'lucide-vue-next'
import { getImageUrl } from '../utils/image'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import EmptyState from '../components/EmptyState.vue'
import PaginationNav from '../components/PaginationNav.vue'
import { usePostList } from '../composables/usePostList'

const { t } = useI18n()

const {
  loading, posts, currentPage, totalPages, searchKeyword,
  getLocalizedText, formatDate, goToPost, changePage,
} = usePostList('blog', { title: () => t('nav.blog'), canonicalPath: '/blog' })
</script>

<style scoped>
.line-clamp-2 {
  overflow: hidden;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  line-clamp: 2;
}

.line-clamp-3 {
  overflow: hidden;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
  line-clamp: 3;
}
</style>
