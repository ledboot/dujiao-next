import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { computed, effectScope, nextTick, reactive, ref, watch } from 'vue'
import { isIndexableStorefrontRoute } from '../src/utils/seo.ts'

function setup(name: string | undefined = 'blog', fullPath = '/blog', delayRouter = false) {
  const route = reactive({ name, fullPath })
  const scope = effectScope()
  const listeners = new Map<string, () => void>()
  const emitted: string[] = []
  const calls = { status: 0, verify: 0 }
  let mount = () => {}
  let unmount = () => {}
  let required = true
  let fail = false
  let ready = () => {}
  const routerReady = delayRouter ? new Promise<void>(resolve => { ready = resolve }) : Promise.resolve()
  const source = readFileSync(new URL('../src/components/captcha/StorefrontGate.vue', import.meta.url), 'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)![1]
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const mocks: Record<string, any> = {
    vue: { computed, ref, watch, nextTick, onMounted: (fn: () => void) => { mount = fn }, onBeforeUnmount: (fn: () => void) => { unmount = fn } },
    'vue-router': { useRoute: () => route, useRouter: () => ({ isReady: () => routerReady }) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../../utils/seo': { isIndexableStorefrontRoute },
    '../../api/client': { STOREFRONT_VERIFICATION_REQUIRED: 'storefront-verification-required' },
    '../../api/auth': { captchaAPI: {
      storefrontStatus: async () => {
        calls.status++
        if (fail) throw Error('status unavailable')
        return { data: { data: { required, site_key: 'test-key', action: 'storefront' } } }
      },
      verifyStorefront: async (token: string) => { calls.verify++; assert.equal(token, 'unit-token'); required = false },
    } },
    './TurnstileCaptcha.vue': {},
  }
  const window = {
    location: { hostname: 'shop.example' },
    addEventListener: (type: string, fn: () => void) => listeners.set(type, fn),
    removeEventListener: (type: string) => listeners.delete(type),
    dispatchEvent: (event: Event) => { emitted.push(event.type); listeners.get(event.type)?.(); return true },
  }
  const execute = new Function('require', 'exports', 'window', 'Event', compiled + '\nreturn { allowed, entered, showContent, challenge, failed, verify }')
  const gate = scope.run(() => execute((name: string) => { assert.ok(name in mocks, name); return mocks[name] }, {}, window, Event))
  mount()
  return { gate, route, calls, emitted, listeners, ready, fail: () => { fail = true }, flush: async () => { for (let i = 0; i < 4; i++) await nextTick() }, stop: () => { unmount(); scope.stop() } }
}

test('all public reading routes render without requesting or solving a challenge', async (t) => {
  for (const name of ['home', 'blog', 'blog-detail', 'about', 'products', 'product-detail', 'category-products', 'notice', 'terms', 'privacy']) {
    const s = setup(name)
    t.after(s.stop)
    s.fail() // Even an unavailable verification service cannot block public reading.
    await s.flush()
    assert.equal(s.calls.status, 0, name)
    assert.equal(s.calls.verify, 0, name)
    assert.equal(s.gate.entered.value, true, name)
    assert.equal(s.gate.showContent.value, true, name)
    assert.ok(s.emitted.includes('storefront-content-ready'), name)
  }
})

test('login, checkout, private and new routes keep verification on navigation', async (t) => {
  const s = setup()
  t.after(s.stop)
  await s.flush()
  for (const name of ['user-login', 'user-register', 'checkout', 'payment', 'personal-center-orders', 'new-route']) {
    s.route.name = name
    s.route.fullPath = '/' + name
    await s.flush()
    assert.equal(s.gate.allowed.value, false, name)
    assert.equal(s.gate.showContent.value, false, name)
    assert.equal(s.gate.challenge.value?.siteKey, 'test-key', name)
    s.route.name = 'blog'
    s.route.fullPath = '/blog'
    await s.flush()
    assert.equal(s.gate.showContent.value, true)
  }
  assert.equal(s.calls.status, 6)
})

test('a protected action from a public page still requires successful verification', async (t) => {
  const s = setup()
  t.after(s.stop)
  await s.flush()
  s.listeners.get('storefront-verification-required')!()
  await s.flush()
  assert.equal(s.gate.showContent.value, false)
  assert.equal(s.calls.status, 1)
  await s.gate.verify('unit-token')
  await s.flush()
  assert.equal(s.calls.verify, 1)
  assert.equal(s.gate.allowed.value, true)
  assert.equal(s.gate.showContent.value, true)
})

test('initial route resolution does not trigger a challenge or leave home blank', async (t) => {
  const s = setup('unresolved', '/', true)
  s.route.name = undefined
  t.after(s.stop)
  await s.flush()
  assert.equal(s.calls.status, 0)
  s.route.name = 'home'
  s.ready()
  await s.flush()
  assert.equal(s.calls.status, 0)
  assert.equal(s.gate.entered.value, true)
  assert.equal(s.gate.showContent.value, true)
})


test('verification service errors keep protected pages closed', async (t) => {
  const s = setup('checkout', '/checkout')
  t.after(s.stop)
  s.fail()
  await s.flush()
  assert.equal(s.gate.allowed.value, false)
  assert.equal(s.gate.showContent.value, false)
  assert.equal(s.gate.failed.value, true)
})
