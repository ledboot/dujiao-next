import test from 'node:test'
import assert from 'node:assert/strict'
import { webcrypto } from 'node:crypto'
import { createAnnouncementSlug } from '../src/utils/announcementSlug.ts'

test('announcement creation supports browsers without randomUUID or crypto', () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'crypto')
  try {
    for (const crypto of [
      webcrypto,
      { getRandomValues: webcrypto.getRandomValues.bind(webcrypto) },
      undefined,
    ]) {
      Object.defineProperty(globalThis, 'crypto', { configurable: true, value: crypto })
      const slugs = Array.from({ length: 100 }, () => createAnnouncementSlug())
      assert.equal(new Set(slugs).size, slugs.length)
      for (const slug of slugs) assert.match(slug, /^notice-[a-z0-9-]+$/)
    }
  } finally {
    if (original) Object.defineProperty(globalThis, 'crypto', original)
    else Reflect.deleteProperty(globalThis, 'crypto')
  }
})
