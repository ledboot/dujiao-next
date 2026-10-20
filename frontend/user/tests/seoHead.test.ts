import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'
import { useHead } from '@unhead/vue'
import { createHead } from '@unhead/vue/client'
import { getLocalizedText } from '../src/utils/resellerSiteConfig.ts'
import * as seo from '../src/utils/seo.ts'

function loadComposable(file: string, mocks: Record<string, any>) {
  const source = readFileSync(new URL(`../src/composables/${file}.ts`, import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const exports: Record<string, any> = {}
  // Unhead recognizes plain objects by constructor, so execute in the same realm.
  const execute = new Function('exports', 'require', 'window', compiled)
  execute(exports, (name: string) => {
    assert.ok(name in mocks, `unexpected dependency: ${name}`)
    return mocks[name]
  }, { location: { origin: 'https://tenant.example.com', pathname: '/products/item' } })
  return exports
}

function setup() {
  const head = createHead()
  // Exercise client reactivity while keeping the test independent of a DOM.
  head.ssr = false
  const scope = effectScope()
  const app = reactive({
    locale: 'en-US',
    config: {
      brand: { site_url: 'https://tenant.example.com/', site_name: 'Shop' },
      seo: { title: { 'zh-TW': '商店' }, description: { 'zh-TW': '預設介紹' }, default_og_image: '/uploads/default.png' },
    },
  })
  const mocks = {
    vue: { computed },
    '@unhead/vue': { useHead: (input: any) => useHead(input, { head }) },
    '../stores/app': { useAppStore: () => app },
    '../utils/image': { getImageUrl: (image: string) => image.startsWith('/') ? image : '/' + image },
    '../utils/resellerSiteConfig': { getLocalizedText },
    '../utils/seo': seo,
  }
  const { usePageSeo } = loadComposable('usePageSeo', mocks)
  return { head, scope, app, mocks, usePageSeo }
}

test('page head combines localized fallback, absolute sharing images and one canonical', async (t) => {
  const s = setup()
  t.after(() => s.scope.stop())
  const title = ref('Product')
  const hidden = ref(false)
  s.scope.run(() => s.usePageSeo({ title: () => title.value, canonicalPath: () => '/products/item?ref=test', noindex: () => hidden.value }))
  const tags = await s.head.resolveTags()
  const meta = (name: string) => tags.find(tag => tag.props.name === name || tag.props.property === name)?.props.content
  assert.equal(tags.find(tag => tag.tag === 'title')?.textContent, 'Product - 商店')
  assert.equal(meta('description'), '預設介紹')
  assert.equal(meta('og:image'), 'https://tenant.example.com/uploads/default.png')
  assert.equal(meta('og:locale'), 'en_US')
  assert.equal(tags.filter(tag => tag.props.rel === 'canonical').length, 1)
  assert.equal(tags.find(tag => tag.props.rel === 'canonical')?.props.href, 'https://tenant.example.com/products/item')

  title.value = 'Other product'
  hidden.value = true
  await nextTick()
  const updated = await s.head.resolveTags()
  assert.equal(updated.find(tag => tag.tag === 'title')?.textContent, 'Other product - 商店')
  assert.equal(updated.find(tag => tag.props.name === 'robots')?.props.content, 'noindex, nofollow')
  assert.equal(updated.some(tag => tag.props.rel === 'canonical'), false)

  hidden.value = false
  await nextTick()
  const restored = await s.head.resolveTags()
  assert.equal(restored.some(tag => tag.props.name === 'robots'), false)
  assert.equal(restored.filter(tag => tag.props.rel === 'canonical').length, 1)
})

test('article navigation ignores old responses and clears metadata on missing content', async (t) => {
  const s = setup()
  t.after(() => s.scope.stop())
  const route = reactive({ name: 'blog-detail', params: { slug: 'old' } })
  const pending: Array<(value: any) => void> = []
  let mount = () => {}
  let unmount = () => {}
  const mocks = {
    ...s.mocks,
    vue: { ref, computed, watch, onMounted: (fn: () => void) => { mount = fn }, onUnmounted: (fn: () => void) => { unmount = fn } },
    'vue-router': { useRoute: () => route },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../api': { postAPI: { detail: () => new Promise(resolve => pending.push(resolve)) } },
    '../utils/debounce': { debounceAsync: (fn: any) => Object.assign(fn, { cancel() {} }) },
    './useProduct': { useLocalized: () => ({ formatPrice() {} }) },
    './usePageSeo': { usePageSeo: s.usePageSeo },
  }
  const { useBlogDetail } = loadComposable('useBlogDetail', mocks)
  const detail = s.scope.run(() => useBlogDetail())
  t.after(() => unmount())
  mount()
  route.params.slug = 'new'
  await nextTick()
  assert.equal(pending.length, 2)
  pending[1]!({ data: { data: { slug: 'new', title: { 'en-US': 'New article' }, summary: {}, published_at: '2026-10-20T01:00:00Z' } } })
  await nextTick()
  pending[0]!({ data: { data: { slug: 'old', title: { 'en-US': 'Old article' } } } })
  await nextTick()
  assert.equal(detail.post.value.slug, 'new')
  let tags = await s.head.resolveTags()
  assert.equal(tags.find(tag => tag.props.rel === 'canonical')?.props.href, 'https://tenant.example.com/blog/new')
  assert.equal(tags.find(tag => tag.tag === 'title')?.textContent, 'New article - 商店')
  assert.equal(tags.filter(tag => tag.props.type === 'application/ld+json').length, 1)

  route.params.slug = 'missing'
  await nextTick()
  pending[2]!({ data: { data: null } })
  await nextTick()
  tags = await s.head.resolveTags()
  assert.equal(tags.find(tag => tag.props.name === 'robots')?.props.content, 'noindex, nofollow')
  assert.equal(tags.some(tag => tag.props.type === 'application/ld+json'), false)
  assert.equal(tags.some(tag => tag.props.rel === 'canonical'), false)
})

test('product head falls back to product description and site image, and removes stale schema', async (t) => {
  const s = setup()
  t.after(() => s.scope.stop())
  const pending: Array<(value: any) => void> = []
  const route = reactive({ name: 'product-detail', params: { slug: 'item' } })
  const mocks = {
    ...s.mocks,
    vue: { ref, computed, watch, nextTick, onMounted() {}, onUnmounted() {} },
    'vue-router': { useRoute: () => route, useRouter: () => ({}) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../api': { productAPI: { detail: () => new Promise(resolve => pending.push(resolve)) } },
    '../stores/cart': { useCartStore: () => ({ items: [] }) },
    '../stores/buyNow': { useBuyNowStore: () => ({}) },
    '../stores/userAuth': { useUserAuthStore: () => ({ user: null }) },
    '../stores/userProfile': { useUserProfileStore: () => ({ memberLevels: [] }) },
    '../utils/debounce': { debounceAsync: (fn: any) => Object.assign(fn, { cancel() {} }) },
    '../utils/sku': {}, '../utils/publicStock': {},
    './useProduct': { useLocalized: () => ({ getLocalizedText, siteCurrency: ref('CNY') }), useProductLabels: () => ({}) },
    './useToast': { toast: {} }, './usePageSeo': { usePageSeo: s.usePageSeo },
    '../utils/resellerSiteConfig': { getLocalizedText: getLocalizedText, getSeoText: getLocalizedText },
  }
  const { useProductDetail } = loadComposable('useProductDetail', mocks)
  const detail = s.scope.run(() => useProductDetail())
  const first = detail.loadProduct()
  const latest = detail.loadProduct()
  pending[1]!({ data: { data: {
    slug: 'item', title: { 'en-US': 'Product' }, description: { 'en-US': 'Product description' },
    price_amount: '12.30', stock_status: 'out_of_stock', images: [],
  } } })
  await latest
  pending[0]!({ data: { data: { slug: 'old', title: { 'en-US': 'Old product' } } } })
  await first
  const tags = await s.head.resolveTags()
  assert.equal(tags.find(tag => tag.props.name === 'description')?.props.content, 'Product description')
  assert.equal(tags.find(tag => tag.props.property === 'og:image')?.props.content, 'https://tenant.example.com/uploads/default.png')
  assert.equal(tags.find(tag => tag.tag === 'title')?.textContent, 'Product - 商店')
  const schema = JSON.parse(tags.find(tag => tag.props.type === 'application/ld+json')!.innerHTML!)
  assert.equal(schema.description, 'Product description')
  assert.equal(schema.offers.price, '12.30')

  detail.product.value.seo_meta = { description: { 'en-US': 'Custom SEO description' } }
  await nextTick()
  const customized = await s.head.resolveTags()
  assert.equal(customized.find(tag => tag.props.name === 'description')?.props.content, 'Custom SEO description')
  const missing = detail.loadProduct()
  pending[2]!({ data: { data: null } })
  await missing
  const cleared = await s.head.resolveTags()
  assert.equal(cleared.find(tag => tag.props.name === 'robots')?.props.content, 'noindex, nofollow')
  assert.equal(cleared.some(tag => tag.props.type === 'application/ld+json'), false)
})
