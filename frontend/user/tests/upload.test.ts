import test from 'node:test'
import assert from 'node:assert/strict'
import { isImageTooLarge, MAX_IMAGE_SIZE_BYTES } from '../src/utils/upload.ts'

test('image uploads accept 1.5 MB and reject one byte more', () => {
  assert.equal(MAX_IMAGE_SIZE_BYTES, 1572864)
  assert.equal(isImageTooLarge({ size: 1572863 }), false)
  assert.equal(isImageTooLarge({ size: 1572864 }), false)
  assert.equal(isImageTooLarge({ size: 1572865 }), true)
})
