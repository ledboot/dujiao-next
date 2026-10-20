import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { postAPI } from '../api'
import { debounceAsync } from '../utils/debounce'
import { useLocalized } from './useProduct'
import { usePageSeo } from './usePageSeo'
import { useHead } from '@unhead/vue'
import { getLocalizedText as resolveLocalizedText } from '../utils/resellerSiteConfig'
import { buildArticleStructuredData, serializeJsonLd } from '../utils/seo'

/**
 * 文章/公告详情共享逻辑（classic + vault 双模板共用）。
 * 完整保留原 views/BlogDetail.vue 的行为，仅抽离为 composable。
 */
export function useBlogDetail() {
  const route = useRoute()
  const { t } = useI18n()
  const appStore = useAppStore()
  const { formatPrice } = useLocalized()

  const loading = ref(true)
  const post = ref<any>(null)
  const relatedProducts = computed<any[]>(() => post.value?.related_products || [])

  const getLocalizedText = (jsonData: any) => {
    if (!jsonData) return ''
    return resolveLocalizedText(jsonData, appStore.locale)
  }

  const formatDate = (dateString: string) => {
    if (!dateString) return ''
    const date = new Date(dateString)
    return date.toLocaleDateString(appStore.locale, {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
    })
  }

  const { canonicalUrl, imageUrl, description, siteName } = usePageSeo({
    title: () => post.value ? getLocalizedText(post.value.title) : '',
    description: () => post.value ? getLocalizedText(post.value.summary) : '',
    image: () => post.value?.thumbnail || '',
    canonicalPath: () => `/blog/${encodeURIComponent(post.value?.slug || String(route.params.slug || ''))}`,
    type: () => 'article',
    noindex: () => !loading.value && !post.value,
  })

  useHead({
    meta: () => post.value?.published_at ? [
      { property: 'article:published_time', content: post.value.published_at },
    ] : [],
    script: () => {
      if (!post.value || !canonicalUrl.value) return []
      return [{
        key: 'article',
        type: 'application/ld+json',
        innerHTML: serializeJsonLd(buildArticleStructuredData({
          title: getLocalizedText(post.value.title),
          description: description.value,
          url: canonicalUrl.value,
          image: imageUrl.value,
          locale: appStore.locale,
          publishedAt: post.value.published_at,
          publisher: siteName.value,
        })),
      }]
    },
  })

  const backLink = computed(() => {
    if (!post.value) return '/blog'
    return post.value.type === 'notice' ? '/notice' : '/blog'
  })

  const backText = computed(() => {
    if (!post.value) return t('blogDetail.backToBlog')
    return post.value.type === 'notice' ? t('blogDetail.backToNotice') : t('blogDetail.backToBlog')
  })

  let postRequest = 0
  const loadPost = async () => {
    const request = ++postRequest
    loading.value = true
    post.value = null
    try {
      const slug = route.params.slug as string
      const response = await postAPI.detail(slug)
      if (request !== postRequest) return
      post.value = response.data.data || null
    } catch (error) {
      if (request !== postRequest) return
      console.error('Failed to load post:', error)
      post.value = null
    } finally {
      if (request === postRequest) loading.value = false
    }
  }

  const debouncedLoadPost = debounceAsync(loadPost, 300)

  onMounted(() => {
    loadPost()
  })

  watch(() => route.params.slug, () => {
    if (route.name === 'blog-detail') void loadPost()
  })

  onUnmounted(() => {
    postRequest++
    debouncedLoadPost.cancel()
  })

  return {
    loading,
    post,
    relatedProducts,
    getLocalizedText,
    formatDate,
    formatPrice,
    backLink,
    backText,
  }
}
