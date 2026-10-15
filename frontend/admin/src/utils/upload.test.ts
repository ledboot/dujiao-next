import { describe, it, expect } from 'vitest'
import { DEFAULT_UPLOAD_MAX_SIZE_BYTES, formatFileSizeLimit, splitFilesBySize } from './upload'

describe('image size limit', () => {
  it('accepts 1.5 MB and rejects one byte more', () => {
    const files = [1572863, 1572864, 1572865].map(size => ({ size }) as File)
    expect(DEFAULT_UPLOAD_MAX_SIZE_BYTES).toBe(1572864)
    expect(splitFilesBySize(files)).toEqual({ accepted: files.slice(0, 2), rejected: files.slice(2) })
    expect(formatFileSizeLimit(DEFAULT_UPLOAD_MAX_SIZE_BYTES)).toBe('1.5 MB')
  })
})
