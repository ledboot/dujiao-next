import type { AdminResellerProductSettingPayloadItem } from '@/api/types'

export const resellerProductSettingsPermission = 'GET:/admin/resellers/product-settings'

type AdminResellerProductSettingLike = {
  reseller_id: number
  profile?: { user?: { email?: string; display_name?: string } }
}

export const buildResellerProductSettingStatusClass = (listed?: boolean) => {
  if (listed === false) return 'border-rose-200 bg-rose-50 text-rose-700'
  return 'border-emerald-200 bg-emerald-50 text-emerald-700'
}

export const getAdminResellerProductSettingOwnerLabel = (row: AdminResellerProductSettingLike) => {
  const email = row.profile?.user?.email?.trim()
  if (email) return email
  const displayName = row.profile?.user?.display_name?.trim()
  if (displayName) return displayName
  return `#${row.reseller_id}`
}

export const normalizeResellerProductSettingPayload = (item: AdminResellerProductSettingPayloadItem): AdminResellerProductSettingPayloadItem => ({
  ...item,
  markup_percent: String(item.markup_percent),
  fixed_markup_amount: String(item.fixed_markup_amount),
  fixed_price_amount: String(item.fixed_price_amount),
  ...(item.supply_price_amount == null ? {} : { supply_price_amount: String(item.supply_price_amount) }),
})
