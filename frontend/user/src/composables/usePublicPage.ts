import { ref, watch, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useHead } from '@unhead/vue'
import { serializeJsonLd } from '../utils/seo'

interface PublicPageSnapshot {
  path: string
  locale: string
  title: string
  description: string
  canonical: string
  image: string
  siteName: string
  type?: string
  hidden: boolean
  schema: Record<string, unknown>
}

/** Keep the server metadata until the equivalent storefront mounts. The static
 * body is a no-JavaScript fallback; index.html hides it before first paint so it
 * cannot stack above the verification screen. Visitors and crawlers receive
 * the same HTML and client behavior.
 */
export function usePublicPage() {
  // Also cover deployments using an older cached index without the early hook.
  document.documentElement.classList.add('storefront-js')
  const route = useRoute()
  let initial: PublicPageSnapshot | null = null
  try {
    const raw = document.getElementById('storefront-public-page')?.textContent
    if (raw) initial = JSON.parse(raw)
  } catch { /* Development builds have no server-rendered projection. */ }
  const page = ref(initial)
  // main.ts removes server-owned head nodes before Unhead starts. This
  // composable retains their snapshot until the storefront supplies its head.
  const clear = () => {
    page.value = null
    document.getElementById('storefront-public-content')?.remove()
    document.getElementById('storefront-public-page')?.remove()
  }
  window.addEventListener('storefront-content-ready', clear)
  onUnmounted(() => window.removeEventListener('storefront-content-ready', clear))
  watch(() => route.path, path => {
    if (page.value && path !== page.value.path) clear()
  })
  useHead(() => page.value ? {
    title: page.value.title,
    htmlAttrs: { lang: page.value.locale },
    link: page.value.hidden ? [] : [{ key: 'canonical', rel: 'canonical', href: page.value.canonical }],
    meta: [
      { name: 'description', content: page.value.description },
      { name: 'robots', content: page.value.hidden ? 'noindex, follow' : 'index, follow, max-image-preview:large' },
      { property: 'og:title', content: page.value.title },
      { property: 'og:description', content: page.value.description },
      { property: 'og:site_name', content: page.value.siteName },
      { property: 'og:type', content: page.value.type === 'Article' ? 'article' : 'website' },
      { property: 'og:locale', content: page.value.locale.replace('-', '_') },
      { property: 'og:url', content: page.value.canonical },
      ...(page.value.image ? [{ property: 'og:image', content: page.value.image }] : []),
    ],
    script: [{ key: 'public-page', type: 'application/ld+json', innerHTML: serializeJsonLd(page.value.schema) }],
  } : {})
}
