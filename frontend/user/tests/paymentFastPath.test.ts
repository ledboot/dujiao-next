import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { computed, ref } from 'vue'
import * as policy from '../src/utils/paymentResumePolicy.ts'
import * as invoiceChannels from '../src/utils/invoiceChannels.ts'
import * as money from '../src/utils/money.ts'

function setup({ guest = false, status = 'pending_payment', expired = false, offline = false } = {}) {
  let mount = () => {}
  let serverTime = Date.parse('2026-10-19T00:00:00Z')
  const clockTicks: Array<() => void> = []
  const calls: string[] = []
  const snapshot = { order_no: 'TEST', status: 'pending_payment', total_amount: '1', expires_at: '2099-01-01T00:00:00Z' }
  const payment = { payment_id: 12, channel_id: 4, interaction_mode: 'redirect', pay_url: 'https://cashier.example.test/', provider_type: 'mola' }
  const tab = {
    closed: false, close() { this.closed = true },
    location: { href: 'about:blank', replace() { calls.push('cashier') } },
    sessionStorage: { setItem() {} },
  }
  const detail = async () => {
    calls.push('validate')
    if (offline) throw Error('offline')
    return { data: { data: { ...snapshot, status, expires_at: expired ? '2000-01-01T00:00:00Z' : snapshot.expires_at } } }
  }
  const api = {
    detail,
    latest: async () => { calls.push('latest'); return { data: { data: payment } } },
    latestPayment: async () => { calls.push('latest'); return { data: { data: payment } } },
    create: async () => { calls.push('create'); return { data: { data: payment } } },
    createPayment: async () => { calls.push('create'); return { data: { data: payment } } },
  }
  let handedOff = true
  const mocks: Record<string, any> = {
    vue: { ref, computed, watch() {}, onMounted(fn: () => void) { mount = fn }, onUnmounted() {} },
    'vue-router': { useRoute: () => ({ query: { order_no: 'TEST', ...(guest ? { guest: '1' } : {}) } }), useRouter: () => ({}) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../stores/app': { useAppStore: () => ({ getServerTime: () => serverTime, config: { payment_channels: [{ id: 4, interaction_mode: 'redirect' }] } }) },
    '../stores/telegramMiniApp': { useTelegramMiniAppStore: () => ({}) },
    './useConfirmDialog': { useConfirmDialog: () => ({ confirm: () => { calls.push('dialog'); return new Promise(() => {}) } }) },
    '../utils/paymentResumePolicy': policy,
    '../utils/invoiceChannels': invoiceChannels,
    '../utils/money': money,
    '../utils/paymentTab': {
      takeCheckoutPayment: () => handedOff ? { order: snapshot, payment } : null,
      takePaymentTab: () => tab,
      openPaymentTab: () => tab,
      showPaymentTabError: () => { calls.push('error-page') },
    },
    '../utils/guestOrderAuth': { loadGuestOrderAuth: () => ({ email: 'guest@example.test', order_password: 'test' }) },
    '../api': { userOrderAPI: api, guestOrderAPI: api, paymentAPI: api, walletAPI: { account: () => { calls.push('wallet'); return new Promise(() => {}) } } },
    '../utils/debounce': { debounceAsync: (fn: any) => Object.assign(fn, { cancel() {} }) },
  }
  const source = readFileSync(new URL('../src/composables/usePayment.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports: Record<string, any> = {}
  runInNewContext(compiled, {
    exports, require: (name: string) => mocks[name] || {},
    window: { sessionStorage: { length: 0 }, scrollTo() {}, setInterval: (fn: () => void, delay: number) => { if (delay === 1000) clockTicks.push(fn); return 1 }, clearInterval() {} },
  })
  const form = exports.usePayment()
  return { form, calls, tab, advanceClock: (ms: number) => { serverTime += ms; clockTicks.forEach(fn => fn()) }, mount: () => mount(), withoutHandoff: () => { handedOff = false } }
}

const settle = async () => { for (let i = 0; i < 12; i++) await Promise.resolve() }

for (const guest of [false, true]) {
  test(`checkout fast path validates once and opens cashier without another latest-payment query (${guest ? 'guest' : 'user'})`, async () => {
    const s = setup({ guest })
    s.mount()
    await settle()
    assert.equal(s.calls.filter(call => call === 'validate').length, 1)
    assert.equal(s.calls.includes('latest'), false)
    assert.equal(s.calls.includes('create'), false)
    assert.equal(s.calls.includes('cashier'), true)
    assert.equal(s.calls.includes('dialog'), true)
    assert.equal(s.form.walletLoading.value, !guest) // wallet may still be pending
  })
}

for (const invalid of [{ status: 'canceled' }, { expired: true }, { offline: true }]) {
  test(`checkout fast path still blocks invalid orders: ${JSON.stringify(invalid)}`, async () => {
    const s = setup(invalid)
    s.mount()
    await settle()
    assert.equal(s.calls.includes('cashier'), false)
    assert.equal(s.calls.includes('dialog'), false)
    assert.equal(s.calls.includes('error-page'), true)
  })
}

test('creating payment on payment page opens cashier while wallet refresh is pending', async () => {
  const s = setup()
  s.withoutHandoff()
  s.form.order.value = { order_no: 'TEST', status: 'pending_payment', total_amount: '1', expires_at: '2099-01-01T00:00:00Z' }
  s.form.selectedChannelId.value = 4
  await s.form.handlePayment()
  await settle()
  assert.equal(s.calls.includes('create'), true)
  assert.equal(s.calls.includes('cashier'), true)
  assert.equal(s.calls.includes('dialog'), true)
  assert.equal(s.form.walletLoading.value, true)
})

test('502 amount notice uses the actual payable amount including fees', () => {
  const { form } = setup()
  form.order.value = { total_amount: '1.00', currency: 'CNY' }
  form.paymentResult.value = { provider_type: '502', interaction_mode: 'qr', qr_code: 'code', payable_amount: '1.02', currency: 'CNY', fee_amount: '0.02' }
  assert.equal(form.showPay502AmountNotice.value, true)
  assert.equal(form.payableAmountDisplay.value, '1.02 CNY')
  form.paymentResult.value.provider_type = 'epay'
  assert.equal(form.showPay502AmountNotice.value, false)
})

for (const provider of ['502', 'epay']) {
  test(`${provider} payment follows its order expiry without offering a payment restart`, () => {
    const s = setup()
    s.form.order.value = { order_no: 'TEST', status: 'pending_payment', total_amount: '1.00', expires_at: '2026-10-19T00:15:00Z' }
    s.form.cachedPayment.value = {
      provider_type: provider, channel_id: 4, interaction_mode: 'qr', qr_code: 'code',
      expires_at: provider === '502' ? '2026-10-19T00:05:00Z' : undefined,
    }
    s.form.restoreCachedPayment()
    assert.equal(s.form.showResultView.value, true)
    assert.equal(s.form.countdownText.value, provider === '502' ? '05:00' : '15:00')
    s.advanceClock(5 * 60 * 1000)
    assert.equal(s.form.orderExpired.value, provider === '502')
    assert.equal(s.form.showResultView.value, provider !== '502')
    assert.equal(s.form.canSubmitPayment.value, provider !== '502')
    assert.equal(s.form.payLinkDisabled.value, provider === '502')
    assert.ok(!s.calls.includes('create'), 'expiry must not generate a new payment')
  })
}

test('shorter order validity still takes precedence over the 502 five-minute limit', () => {
  const s = setup()
  s.form.order.value = { order_no: 'TEST', status: 'pending_payment', total_amount: '1.00', expires_at: '2026-10-19T00:02:00Z' }
  s.form.cachedPayment.value = { provider_type: '502', channel_id: 4, interaction_mode: 'qr', qr_code: 'code', expires_at: '2026-10-19T00:05:00Z' }
  s.form.restoreCachedPayment()
  assert.equal(s.form.countdownText.value, '02:00')
  s.advanceClock(2 * 60 * 1000)
  assert.equal(s.form.orderExpired.value, true)
  assert.equal(s.form.showResultView.value, false)
})
