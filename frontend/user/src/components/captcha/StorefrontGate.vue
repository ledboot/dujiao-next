<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { captchaAPI } from '../../api/auth'
import { STOREFRONT_VERIFICATION_REQUIRED } from '../../api/client'
import TurnstileCaptcha from './TurnstileCaptcha.vue'

const { t } = useI18n()
const allowed = ref(false)
const entered = ref(false)
const checking = ref(false)
const verifying = ref(false)
const failed = ref(false)
const challenge = ref<{ siteKey: string; action: string } | null>(null)

const check = async (expectClearance = false) => {
  if (checking.value) return
  allowed.value = false
  checking.value = true
  failed.value = false
  challenge.value = null
  try {
    const { data } = await captchaAPI.storefrontStatus()
    const status = data.data
    if (status?.required === false) {
      entered.value = true
      allowed.value = true
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
  if (allowed.value) void check()
}

onMounted(() => {
  window.addEventListener(STOREFRONT_VERIFICATION_REQUIRED, onClearanceRequired)
  void check()
})
onBeforeUnmount(() => window.removeEventListener(STOREFRONT_VERIFICATION_REQUIRED, onClearanceRequired))
</script>

<template>
  <div v-if="entered" v-show="allowed" class="contents">
    <slot />
  </div>
  <main v-if="!allowed" class="flex min-h-screen items-center justify-center px-4 py-8">
    <section class="w-full max-w-sm space-y-4 rounded-xl border border-border bg-card p-5 text-center shadow-sm">
      <h1 class="text-xl font-semibold">{{ t('storefrontGate.title') }}</h1>
      <p v-if="checking" role="status" class="text-sm text-muted-foreground">{{ t('storefrontGate.checking') }}</p>
      <template v-if="challenge && !failed">
        <p class="text-sm text-muted-foreground">{{ t('storefrontGate.prompt') }}</p>
        <div class="flex justify-center">
          <TurnstileCaptcha
            :site-key="challenge.siteKey"
            :action="challenge.action"
            size="compact"
            @update:model-value="verify"
            @challenge-error="onChallengeError"
          />
        </div>
        <p v-if="verifying" role="status" class="text-sm text-muted-foreground">{{ t('storefrontGate.verifying') }}</p>
      </template>
      <template v-if="failed">
        <p role="alert" class="text-sm text-muted-foreground">{{ t('storefrontGate.failed') }}</p>
        <button type="button" class="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground" @click="check()">
          {{ t('storefrontGate.retry') }}
        </button>
      </template>
    </section>
  </main>
</template>
