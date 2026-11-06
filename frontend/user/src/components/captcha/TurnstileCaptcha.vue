<script lang="ts">
interface TurnstileRenderOptions {
  sitekey: string
  action?: string
  size?: 'normal' | 'compact'
  appearance?: 'interaction-only'
  'before-interactive-callback'?: () => void
  'timeout-callback'?: () => void
  theme?: 'auto' | 'light' | 'dark'
  callback?: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
}

interface TurnstileAPI {
  render: (container: HTMLElement, options: TurnstileRenderOptions) => string
  reset: (widgetID?: string) => void
  remove: (widgetID: string) => void
}

declare global {
  interface Window {
    turnstile?: TurnstileAPI
  }
}

let turnstileScriptPromise: Promise<void> | null = null

const loadScript = async () => {
  if (window.turnstile) return
  if (!turnstileScriptPromise) {
    turnstileScriptPromise = new Promise<void>((resolve, reject) => {
      const script = document.createElement('script')
      const fail = () => {
        window.clearTimeout(timeout)
        script.remove()
        reject(new Error('Failed to load Turnstile'))
      }
      const timeout = window.setTimeout(fail, 15000)
      script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
      script.setAttribute('data-cfasync', 'false')
      script.async = true
      script.onload = () => {
        window.clearTimeout(timeout)
        resolve()
      }
      script.onerror = fail
      document.head.appendChild(script)
    }).catch((error: unknown) => {
      turnstileScriptPromise = null
      throw error
    })
  }
  await turnstileScriptPromise
}
</script>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps<{
  modelValue?: string
  siteKey: string
  action?: string
  size?: 'normal' | 'compact'
  appearance?: 'interaction-only'
}>()

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'challenge-error'): void
  (event: 'interactive'): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const widgetID = ref<string>('')

const onError = () => {
  emit('update:modelValue', '')
  emit('challenge-error')
}

const renderWidget = async () => {
  if (!props.siteKey || !containerRef.value) {
    emit('update:modelValue', '')
    return
  }

  await loadScript()
  if (!containerRef.value) return
  if (!window.turnstile) {
    onError()
    return
  }

  if (widgetID.value) {
    window.turnstile.remove(widgetID.value)
    widgetID.value = ''
  }

  containerRef.value.innerHTML = ''
  widgetID.value = window.turnstile.render(containerRef.value, {
    sitekey: props.siteKey,
    ...(props.action ? { action: props.action } : {}),
    ...(props.size ? { size: props.size } : {}),
    ...(props.appearance ? {
      appearance: props.appearance,
      'before-interactive-callback': () => emit('interactive'),
    } : {}),
    theme: 'auto',
    callback: (token: string) => emit('update:modelValue', token || ''),
    'expired-callback': () => emit('update:modelValue', ''),
    'error-callback': onError,
    ...(props.action ? { 'timeout-callback': onError } : {}),
  })
}

const reset = () => {
  if (window.turnstile && widgetID.value) {
    window.turnstile.reset(widgetID.value)
  }
  emit('update:modelValue', '')
}

watch(() => props.siteKey, () => void renderWidget().catch(onError))
onMounted(() => void renderWidget().catch(onError))

onBeforeUnmount(() => {
  if (window.turnstile && widgetID.value) {
    window.turnstile.remove(widgetID.value)
  }
})

defineExpose({
  reset,
})
</script>

<template>
  <div ref="containerRef" class="min-h-[65px]"></div>
</template>
