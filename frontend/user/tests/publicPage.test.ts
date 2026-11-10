import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { ref, watch, reactive, effectScope, nextTick } from 'vue'
import { createHead } from '@unhead/vue/client'
import { useHead } from '@unhead/vue'
import { serializeJsonLd } from '../src/utils/seo.ts'

test('verification hides the static fallback while keeping SEO until storefront mount', async (t) => {
  const source = readFileSync(new URL('../src/composables/usePublicPage.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const head = createHead()
  head.ssr = false
  const scope = effectScope()
  t.after(() => scope.stop())
  const route = reactive({ path: '/blog/guide' })
  const removed: string[] = []
  const listeners = new Map<string, () => void>()
  const snapshot = { path: route.path, locale: 'zh-CN', title: 'Article - Brand', description: 'Useful guide', canonical: 'https://example.com/blog/guide', image: '', siteName: 'Brand', hidden: false, schema: { '@type': 'Article' } }
  const activeClasses = new Set<string>()
  const document = {
    documentElement: { classList: { add: (name: string) => activeClasses.add(name) } },
    getElementById: (id: string) => ({ textContent: id === 'storefront-public-page' ? JSON.stringify(snapshot) : '', remove: () => removed.push(id) }),
    querySelectorAll: () => [{ remove: () => removed.push('server-head') }],
  }
  const window = { addEventListener: (event: string, fn: () => void) => listeners.set(event, fn), removeEventListener: (event: string) => listeners.delete(event) }
  const mocks: Record<string, any> = {
    vue: { ref, watch, onUnmounted() {} },
    'vue-router': { useRoute: () => route },
    '@unhead/vue': { useHead: (input: any) => useHead(input, { head }) },
    '../utils/seo': { serializeJsonLd },
  }
  const exports: Record<string, any> = {}
  new Function('exports', 'require', 'document', 'window', compiled)(exports, (name: string) => mocks[name], document, window)
  scope.run(() => exports.usePublicPage())
  let tags = await head.resolveTags()
  assert.equal(tags.find(tag => tag.tag === 'title')?.textContent, snapshot.title)
  assert.equal(tags.filter(tag => tag.props.rel === 'canonical').length, 1)
  assert.deepEqual(removed, [])
  assert.ok(activeClasses.has('storefront-js'))
  assert.equal(removed.includes('storefront-public-content'), false)
  listeners.get('storefront-content-ready')!()
  await nextTick()
  tags = await head.resolveTags()
  assert.equal(tags.some(tag => tag.props.rel === 'canonical'), false)
  assert.ok(removed.includes('storefront-public-content'))

  // A client navigation must not retain the previous public article or metadata.
  scope.run(() => exports.usePublicPage())
  route.path = '/auth/login'
  await nextTick()
  tags = await head.resolveTags()
  assert.equal(tags.some(tag => tag.props.rel === 'canonical'), false)
})

test('bootstrap removes server head nodes before Unhead can hydrate them', async () => {
  const source = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const events: string[] = []
  let attached = true
  const app = { use() {}, mount() { events.push('mount') } }
  const mocks: Record<string, any> = {
    vue: { createApp: () => app },
    pinia: { createPinia: () => ({}) },
    '@unhead/vue/client': { createHead: () => { assert.equal(attached, false); events.push('head'); return {} } },
    './style.css': {}, './App.vue': {},
    './router': { default: { isReady: () => Promise.resolve() }, warmupCommonRoutes() {} },
    './i18n': { default: {}, detectLocale: () => 'en-US', setI18nLocale: () => Promise.resolve(), warmupLocaleMessages() {} },
    './stores/telegramMiniApp': { useTelegramMiniAppStore: () => ({ init: () => Promise.resolve() }) },
    './templates/registry': { initTemplateOverride() {} },
  }
  const document = { querySelectorAll: () => [{ remove: () => { attached = false; events.push('remove') } }] }
  const execute = new Function('require', 'exports', 'document', 'globalThis', compiled)
  execute((name: string) => { assert.ok(name in mocks, name); return mocks[name] }, {}, document, { console: {} })
  await Promise.resolve()
  await Promise.resolve()
  assert.deepEqual(events.slice(0, 2), ['remove', 'head'])
  assert.ok(events.includes('mount'))
})
