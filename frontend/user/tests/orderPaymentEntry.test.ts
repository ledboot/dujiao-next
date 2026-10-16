import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { ref } from 'vue'
import { paymentLinkBlockReason } from '../src/utils/paymentResumePolicy.ts'

function setup(guest = false) {
  let now = 1000
  let unmount = () => {}
  const order = ref<any>({ order_no: 'test', status: 'pending_payment', expires_at: new Date(2000).toISOString() })
  const routes: any[] = []
  const errors: string[] = []
  let fetches = 0
  let fetchOrder = async () => ({ data: { data: { ...order.value } } })
  const mocks: Record<string, any> = {
    vue: { ref, onUnmounted: (fn: () => void) => { unmount = fn } },
    'vue-router': { useRouter: () => ({ push: async (route: any) => { routes.push(route) } }) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../stores/app': { useAppStore: () => ({ getServerTime: () => now }) },
    '../utils/paymentResumePolicy': { paymentLinkBlockReason },
    './useToast': { toast: { error: (key: string) => errors.push(key) } },
  }
  const source = readFileSync(new URL('../src/composables/useOrderPaymentEntry.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports: Record<string, any> = {}
  runInNewContext(compiled, { exports, require: (name: string) => mocks[name] })
  const entry = exports.useOrderPaymentEntry(order, () => { fetches++; return fetchOrder() }, guest)
  return { entry, order, routes, errors, fetches: () => fetches, setTime: (value: number) => { now = value }, setFetch: (fn: typeof fetchOrder) => { fetchOrder = fn }, unmount: () => unmount() }
}

test('detail payment entry blocks an expired page before navigation', async () => {
  const s = setup()
  s.setTime(2000)
  await s.entry.goToPayment()
  assert.equal(s.fetches(), 0)
  assert.equal(s.routes.length, 0)
  assert.deepEqual(s.errors, ['payment.orderExpired'])
})

test('detail payment entry checks fresh server status and expiry after request', async () => {
  for (const status of ['canceled', 'paid', 'pending_payment']) {
    const s = setup()
    s.setFetch(async () => {
      s.setTime(2000)
      return { data: { data: { ...s.order.value, status } } }
    })
    await s.entry.goToPayment()
    assert.equal(s.routes.length, 0)
    assert.equal(s.order.value.status, status)
    assert.equal(s.errors.length, 1)
  }
})

test('detail payment entry fails closed when validation cannot finish', async () => {
  const s = setup()
  s.setFetch(async () => { throw Error('offline') })
  await s.entry.goToPayment()
  assert.equal(s.routes.length, 0)
  assert.deepEqual(s.errors, ['payment.verifyOrderFailed'])
  assert.equal(s.entry.checkingPayment.value, false)
})

test('detail payment entry preserves user and guest routes and ignores duplicate clicks', async () => {
  for (const guest of [false, true]) {
    const s = setup(guest)
    await Promise.all([s.entry.goToPayment(), s.entry.goToPayment()])
    assert.equal(s.fetches(), 1)
    assert.equal(s.routes.length, 1)
    assert.equal(s.routes[0].path, '/pay')
    assert.equal(s.routes[0].query.order_no, 'test')
    assert.equal(s.routes[0].query.guest, guest ? '1' : undefined)
  }
})

test('detail payment entry does not navigate after leaving the detail page', async () => {
  const s = setup()
  s.setFetch(async () => { s.unmount(); return { data: { data: s.order.value } } })
  await s.entry.goToPayment()
  assert.equal(s.routes.length, 0)
})
