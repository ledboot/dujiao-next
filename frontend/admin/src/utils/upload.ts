export const DEFAULT_UPLOAD_MAX_SIZE_BYTES = 1.5 * 1024 * 1024

export function formatFileSizeLimit(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${Number((bytes / 1024 / 1024).toFixed(2))} MB`
  }
  if (bytes >= 1024) {
    return `${Math.floor(bytes / 1024)} KB`
  }
  return `${bytes} B`
}

export function splitFilesBySize(files: File[], maxSize = DEFAULT_UPLOAD_MAX_SIZE_BYTES) {
  const accepted: File[] = []
  const rejected: File[] = []
  files.forEach((file) => {
    if (file.size > maxSize) {
      rejected.push(file)
    } else {
      accepted.push(file)
    }
  })
  return { accepted, rejected }
}
