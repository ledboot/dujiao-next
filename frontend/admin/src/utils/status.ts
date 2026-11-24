export type TranslateFn = (...args: any[]) => string

export const orderStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    pending_payment: t('order.status.pending_payment'),
    paid: t('order.status.paid'),
    fulfilling: t('order.status.fulfilling'),
    partially_delivered: t('order.status.partially_delivered'),
    partially_refunded: t('order.status.partially_refunded'),
    delivered: t('order.status.delivered'),
    completed: t('order.status.completed'),
    canceled: t('order.status.canceled'),
    refunded: t('order.status.refunded'),
  }
  return map[status] || status
}

export const orderStatusClass = (status?: string) => {
  switch (status) {
    case 'pending_payment':
      return 'text-amber-700 border-amber-500/25 bg-amber-500/10 dark:text-amber-300 dark:border-amber-400/25 dark:bg-amber-400/10'
    case 'paid':
    case 'fulfilling':
      return 'text-emerald-700 border-emerald-500/25 bg-emerald-500/10 dark:text-emerald-300 dark:border-emerald-400/25 dark:bg-emerald-400/10'
    case 'partially_delivered':
    case 'partially_refunded':
      return 'text-orange-700 border-orange-500/25 bg-orange-500/10 dark:text-orange-300 dark:border-orange-400/25 dark:bg-orange-400/10'
    case 'delivered':
    case 'completed':
      return 'text-indigo-700 border-indigo-500/25 bg-indigo-500/10 dark:text-indigo-300 dark:border-indigo-400/25 dark:bg-indigo-400/10'
    case 'canceled':
      return 'text-muted-foreground border-border bg-muted/60'
    case 'refunded':
      return 'text-sky-700 border-sky-500/25 bg-sky-500/10 dark:text-sky-300 dark:border-sky-400/25 dark:bg-sky-400/10'
    default:
      return 'text-muted-foreground border-border bg-muted/60'
  }
}

export const paymentStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    initiated: t('payment.status.initiated'),
    pending: t('payment.status.pending'),
    success: t('payment.status.success'),
    failed: t('payment.status.failed'),
    expired: t('payment.status.expired'),
  }
  return map[status] || status
}

// 支付异常码未配置翻译时原样显示，方便排查新增的异常码
export const paymentExceptionLabel = (t: TranslateFn, code?: string) => {
  if (!code) return '-'
  const key = `admin.payments.exceptions.${code}`
  const translated = t(key)
  return translated === key ? code : translated
}

export const paymentStatusClass = (status?: string) => {
  switch (status) {
    case 'success':
      return 'text-emerald-700 border-emerald-500/25 bg-emerald-500/10 dark:text-emerald-300 dark:border-emerald-400/25 dark:bg-emerald-400/10'
    case 'pending':
      return 'text-amber-700 border-amber-500/25 bg-amber-500/10 dark:text-amber-300 dark:border-amber-400/25 dark:bg-amber-400/10'
    case 'failed':
      return 'text-rose-700 border-rose-500/25 bg-rose-500/10 dark:text-rose-300 dark:border-rose-400/25 dark:bg-rose-400/10'
    case 'expired':
      return 'text-muted-foreground border-border bg-muted/60'
    default:
      return 'text-muted-foreground border-border bg-muted/60'
  }
}

export const userStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    active: t('admin.users.status.active'),
    disabled: t('admin.users.status.disabled'),
  }
  return map[status] || status
}

export const userStatusClass = (status?: string) => {
  if (status === 'active') return 'text-emerald-700 border-emerald-500/25 bg-emerald-500/10 dark:text-emerald-300 dark:border-emerald-400/25 dark:bg-emerald-400/10'
  if (status === 'disabled') return 'text-rose-700 border-rose-500/25 bg-rose-500/10 dark:text-rose-300 dark:border-rose-400/25 dark:bg-rose-400/10'
  return 'text-muted-foreground border-border bg-muted/60'
}
