import test from 'node:test'
import assert from 'node:assert/strict'
import { filterInvoiceChannels } from '../src/utils/invoiceChannels.ts'

const channels = [{ id: 1, supports_invoice: false }, { id: 2, supports_invoice: true }, { id: 3 }]

test('invoice selection only shows channels explicitly supporting invoices', () => {
  assert.deepEqual(filterInvoiceChannels(channels, true).map(channel => channel.id), [2])
})

test('disabling invoices restores every available channel', () => {
  assert.deepEqual(filterInvoiceChannels(channels, false).map(channel => channel.id), [1, 2, 3])
})

test('legacy channels default to unsupported, with no fallback when invoicing', () => {
  assert.deepEqual(filterInvoiceChannels([{ id: 1 }], true), [])
  assert.deepEqual(filterInvoiceChannels([], true), [])
})
