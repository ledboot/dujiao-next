export const MAX_IMAGE_SIZE_BYTES = 1.5 * 1024 * 1024

export function isImageTooLarge(file: { size: number }): boolean {
  return file.size > MAX_IMAGE_SIZE_BYTES
}
