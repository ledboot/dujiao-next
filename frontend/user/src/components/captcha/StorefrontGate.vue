<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { isIndexableStorefrontRoute } from '../../utils/seo'
import { captchaAPI } from '../../api/auth'
import { STOREFRONT_VERIFICATION_REQUIRED } from '../../api/client'
import TurnstileCaptcha from './TurnstileCaptcha.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const routeReady = ref(false)
const hostname = window.location.hostname
const allowed = ref(false)
const entered = ref(false)
const checking = ref(false)
const verifying = ref(false)
const failed = ref(false)
const needsInteraction = ref(false)
const challenge = ref<{ siteKey: string; action: string } | null>(null)
const verificationRequested = ref(false)
const publicReading = computed(() => isIndexableStorefrontRoute(route.name))
const showContent = computed(() => allowed.value || (publicReading.value && !verificationRequested.value))

const contentReady = async () => {
  entered.value = true
  await nextTick()
  window.dispatchEvent(new Event('storefront-content-ready'))
}

const check = async (expectClearance = false) => {
  if (checking.value) return
  allowed.value = false
  checking.value = true
  failed.value = false
  needsInteraction.value = false
  challenge.value = null
  try {
    const { data } = await captchaAPI.storefrontStatus()
    const status = data.data
    if (status?.required === false) {
      allowed.value = true
      await contentReady()
    } else if (!expectClearance && status?.required === true && status.site_key && status.action) {
      challenge.value = { siteKey: status.site_key, action: status.action }
    } else {
      failed.value = true
    }
  } catch {
    failed.value = true
  } finally {
    checking.value = false
  }
}

const verify = async (token: string) => {
  if (!token || verifying.value || failed.value) return
  verifying.value = true
  try {
    await captchaAPI.verifyStorefront(token)
    await check(true)
  } catch {
    failed.value = true
  } finally {
    verifying.value = false
  }
}

const onChallengeError = () => {
  if (!verifying.value) failed.value = true
}

const onClearanceRequired = () => {
  verificationRequested.value = true
  void check()
}

watch(() => [routeReady.value, route.name, route.fullPath], () => {
  if (!routeReady.value) return
  verificationRequested.value = false
  if (publicReading.value) void contentReady()
  // The server checks the clearance cookie on every protected API request.
  // Reuse its confirmed result across SPA tabs; a 428 requests a fresh check.
  else if (!allowed.value) void check()
}, { immediate: true })

onMounted(() => {
  window.addEventListener(STOREFRONT_VERIFICATION_REQUIRED, onClearanceRequired)
  void router.isReady().then(() => { routeReady.value = true })
})
onBeforeUnmount(() => window.removeEventListener(STOREFRONT_VERIFICATION_REQUIRED, onClearanceRequired))
</script>

<template>
  <div v-if="entered" v-show="showContent" class="contents">
    <slot />
  </div>
  <div v-if="routeReady && !showContent" class="flex min-h-dvh flex-col bg-white px-2 text-[#313131] [color-scheme:light] min-[348px]:px-6">
    <main class="mx-auto w-full max-w-[56rem] flex-1 pb-12 pt-20 sm:pt-32">
      <h1 class="break-words text-3xl font-semibold leading-tight sm:text-[2.375rem]">{{ hostname }}</h1>
      <h2 class="mt-2 text-xl font-semibold leading-tight sm:text-2xl">{{ t('storefrontGate.title') }}</h2>
      <p class="mt-2 text-sm leading-relaxed sm:text-base">{{ t('storefrontGate.prompt') }}</p>

      <div v-if="!failed" class="mt-7">
        <div v-if="!needsInteraction" role="status">
          <span
            aria-hidden="true"
            class="block size-7 rounded-full border-[4px] border-transparent border-t-current motion-safe:animate-spin"
          ></span>
          <span class="sr-only">{{ t(verifying ? 'storefrontGate.verifying' : 'storefrontGate.checking') }}</span>
        </div>
        <TurnstileCaptcha
          v-if="challenge"
          :site-key="challenge.siteKey"
          :action="challenge.action"
          size="normal"
          appearance="interaction-only"
          @interactive="needsInteraction = true"
          @update:model-value="verify"
          @challenge-error="onChallengeError"
        />
      </div>

      <div v-else class="mt-7">
        <p role="alert" class="text-sm">{{ t('storefrontGate.failed') }}</p>
        <button type="button" class="mt-3 rounded border border-[#767676] px-4 py-2 text-sm hover:bg-[#f4f4f4]" @click="check()">
          {{ t('storefrontGate.retry') }}
        </button>
      </div>
    </main>

    <footer class="mx-auto flex w-full max-w-[56rem] flex-wrap items-center justify-center gap-x-2 gap-y-1 border-t border-[#dedede] py-4 text-center text-xs">
      <i18n-t keypath="storefrontGate.poweredBy" tag="span">
        <template #provider>
          <a class="text-[#1746a2] underline" href="https://www.cloudflare.com/products/turnstile/" target="_blank" rel="noopener noreferrer">Cloudflare Turnstile</a>
        </template>
      </i18n-t>
      <span class="inline-flex items-center gap-x-2">
        <span aria-hidden="true" class="text-[#767676]">|</span>
        <a class="text-[#1746a2] underline" href="https://www.cloudflare.com/privacypolicy/" target="_blank" rel="noopener noreferrer">
          {{ t('storefrontGate.privacy') }}
        </a>
      </span>
    </footer>
  </div>
</template>
