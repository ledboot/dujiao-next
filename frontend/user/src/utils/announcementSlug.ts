// Announcement slugs identify public content; they are not authentication tokens.
export function createAnnouncementSlug(): string {
  const crypto = globalThis.crypto
  if (typeof crypto?.randomUUID === 'function') {
    return `notice-${crypto.randomUUID()}`
  }
  if (typeof crypto?.getRandomValues === 'function') {
    const bytes = crypto.getRandomValues(new Uint8Array(16))
    return `notice-${Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')}`
  }
  return `notice-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}-${Math.random().toString(36).slice(2)}`
}
