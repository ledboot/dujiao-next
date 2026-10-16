import { onUnmounted, ref, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { paymentLinkBlockReason } from '../utils/paymentResumePolicy'
import { toast } from './useToast'

// Revalidate stale detail pages before entering the payment flow.
export function useOrderPaymentEntry(order: Ref<any>, fetchOrder: () => Promise<any>, guest = false) {
  const router = useRouter()
  const { t } = useI18n()
  const appStore = useAppStore()
  const checkingPayment = ref(false)
  let disposed = false
  onUnmounted(() => { disposed = true })

  const goToPayment = async () => {
    if (checkingPayment.value || !order.value) return
    const blocked = paymentLinkBlockReason(order.value, appStore.getServerTime())
    if (blocked) {
      toast.error(t(blocked))
      return
    }
    checkingPayment.value = true
    try {
      const response = await fetchOrder()
      if (disposed) return
      order.value = response.data.data
      const reason = paymentLinkBlockReason(order.value, appStore.getServerTime())
      if (reason) {
        toast.error(t(reason))
        return
      }
      await router.push({ path: '/pay', query: {
        order_no: order.value.order_no,
        ...(guest ? { guest: '1' } : {}),
      } })
    } catch {
      if (!disposed) toast.error(t('payment.verifyOrderFailed'))
    } finally {
      checkingPayment.value = false
    }
  }

  return { goToPayment, checkingPayment }
}
