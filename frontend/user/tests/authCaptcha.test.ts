import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { computed, ref, watch } from 'vue'

// Exercise the actual composables, replacing only UI, store and timer boundaries.
function setup(kind: 'Login' | 'Register', provider = 'turnstile', enabled = true) {
  const calls: Array<{ action: string; payload: any }> = []
  let fail = false
  const auth = {
    loading: false,
    async login(payload: any) { calls.push({ action: 'login', payload }); if (fail) throw Error('failed'); return {} },
    async register(payload: any) { calls.push({ action: 'register', payload }); if (fail) throw Error('failed') },
    async sendVerifyCode(payload: any) { calls.push({ action: 'send', payload }); if (fail) throw Error('failed') },
  }
  const app = { config: { captcha: { provider, scenes: { login: enabled, register_send_code: enabled }, turnstile: { site_key: 'test' } }, email_verification_enabled: false }, loadConfig: async () => {} }
  const mocks: Record<string, any> = {
    vue: { ref, computed, watch, onMounted() {}, onUnmounted() {} },
    'vue-router': { useRoute: () => ({ query: {} }), useRouter: () => ({ push() {}, replace() {} }) },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../stores/userAuth': { useUserAuthStore: () => auth },
    '../stores/app': { useAppStore: () => app },
    '../stores/telegramMiniApp': { useTelegramMiniAppStore: () => ({}) },
    '../utils/debounce': { debounceAsync: (fn: any) => fn },
    './useFormValidation': { useFormValidation: () => ({ addRule() {}, requiredRule() {}, emailRule() {}, minLengthRule() {}, validateAll: () => true, touchField() {}, hasError: () => false }), getPasswordStrength: () => 'strong' },
    '../utils/telegramMiniApp': { isTelegramUrlEnvironment: () => false },
    '../utils/googleIdentity': { detectGoogleIdentityUXMode: () => 'popup' },
    '../utils/googleRedirect': { shouldResumeGoogleRedirect2FA: () => false },
  }
  const source = readFileSync(new URL(`../src/composables/use${kind}.ts`, import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports: Record<string, any> = {}
  runInNewContext(compiled, { exports, require: (name: string) => mocks[name] || {}, window: { setInterval: () => 1 }, clearInterval() {} })
  const form = exports[`use${kind}`]()
  form.email.value = 'buyer@example.com'
  form.password.value = 'password123'
  if (kind === 'Register') form.agreed.value = true
  let resets = 0
  form.turnstileRef.value = { reset() { resets++ } }
  return { form, app, auth, calls, reject: () => { fail = true }, resets: () => resets }
}

for (const kind of ['Login', 'Register'] as const) {
  test(`${kind}: blocks missing/expired token and submits verified token`, async () => {
    const { form, calls } = setup(kind)
    const submit = form[`handle${kind}`]
    assert.equal(form.captchaReady.value, false)
    await submit()
    assert.equal(calls.length, 0)
    form.turnstileToken.value = 'verified-token'
    assert.equal(form.captchaReady.value, true)
    form.turnstileToken.value = ''
    await submit()
    assert.equal(calls.length, 0)
    form.turnstileToken.value = 'fresh-token'
    await submit()
    assert.equal(calls.length, 1)
    assert.equal(calls[0]?.payload.captcha_payload.turnstile_token, 'fresh-token')
  })

  test(`${kind}: blocks before config loads and during pending requests`, async () => {
    const { form, app, auth, calls } = setup(kind)
    form.turnstileToken.value = 'valid'
    auth.loading = true
    await form[`handle${kind}`]()
    auth.loading = false
    app.config.captcha = null as any
    await form[`handle${kind}`]()
    assert.equal(calls.length, 0)
  })

  test(`${kind}: clears consumed token after failed request`, async () => {
    const { form, reject, resets } = setup(kind)
    form.turnstileToken.value = 'valid'
    reject()
    await form[`handle${kind}`]()
    assert.equal(form.turnstileToken.value, '')
    assert.equal(resets(), 1)
  })

  test(`${kind}: preserves disabled captcha setting`, async () => {
    const { form, calls } = setup(kind, 'none', false)
    await form[`handle${kind}`]()
    assert.equal(calls.length, 1)
    assert.equal(calls[0]?.payload.captcha_payload, undefined)
  })
}

test('Registration requires a new token after sending email, even with email verification disabled', async () => {
  const { form, calls, resets } = setup('Register')
  await form.handleSendCode()
  assert.equal(calls.length, 0)
  form.turnstileToken.value = 'send-token'
  await form.handleSendCode()
  assert.equal(calls[0]?.action, 'send')
  assert.equal(form.turnstileToken.value, '')
  assert.equal(resets(), 1)
  await form.handleRegister()
  assert.equal(calls.length, 1)
  form.turnstileToken.value = 'registration-token'
  await form.handleRegister()
  assert.equal(calls[1]?.action, 'register')
  assert.equal(calls[1]?.payload.captcha_payload.turnstile_token, 'registration-token')
})
