import { describe, expect, it } from 'vitest'
import type { AdminResellerProductSettingPayloadItem } from '@/api/types'
import { normalizeResellerProductSettingPayload } from './resellerProductSettings'

const setting: AdminResellerProductSettingPayloadItem = {
  sku_id: 0, is_listed: true, pricing_mode: 'inherit', markup_percent: '0.00',
  fixed_markup_amount: '0.00', fixed_price_amount: '0.00', sort_order: 0,
}

describe('reseller price request serialization', () => {
  it.each([115, 115.25, 0])('serializes numeric input %s as a string for the API', (amount) => {
    // Number inputs can emit numbers despite the form declaring string fields.
    const input = { ...setting, supply_price_amount: amount } as unknown as AdminResellerProductSettingPayloadItem
    const payload = JSON.parse(JSON.stringify({ settings: [normalizeResellerProductSettingPayload(input)] }))
    expect(payload.settings[0].supply_price_amount).toBe(String(amount))
    expect(input.supply_price_amount).toBe(amount)
  })

  it('preserves decimal precision and omitted supply prices', () => {
    expect(normalizeResellerProductSettingPayload({ ...setting, supply_price_amount: '123456789012345678.12' }).supply_price_amount)
      .toBe('123456789012345678.12')
    expect(normalizeResellerProductSettingPayload(setting)).not.toHaveProperty('supply_price_amount')
  })
})
