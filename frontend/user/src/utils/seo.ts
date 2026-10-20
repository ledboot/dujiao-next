/** Only public content routes belong in search results. New routes default to noindex. */
export function isIndexableStorefrontRoute(name: unknown): boolean {
  return typeof name === 'string' && [
    'home', 'products', 'category-products', 'product-detail',
    'blog', 'blog-detail', 'notice', 'about', 'terms', 'privacy',
  ].includes(name)
}

export function resolveSeoBaseUrl(configured: string, origin: string): string {
  for (const candidate of [configured, origin]) {
    try {
      const url = new URL(candidate.trim())
      if (['http:', 'https:'].includes(url.protocol) && !url.username && !url.password) {
        return url.origin
      }
    } catch { /* Try the current origin when configuration is invalid. */ }
  }
  return ''
}

export function resolveCanonicalUrl(path: string, baseUrl: string): string {
  if (!baseUrl) return ''
  // Keep the URL on this storefront, including paths beginning with //.
  const pathname = (path.split(/[?#]/, 1)[0] || '/').replace(/\\/g, '%5C')
  return new URL('/' + pathname.replace(/^\/+/, ''), baseUrl).href
}

export function resolveSeoImageUrl(image: string, baseUrl: string): string {
  if (!image.trim()) return ''
  try {
    const url = new URL(image.trim(), baseUrl || undefined)
    return ['http:', 'https:'].includes(url.protocol) ? url.href : ''
  } catch {
    return ''
  }
}

/** Keep merchant-supplied text inside its JSON script element, including in SSR. */
export function serializeJsonLd(value: Record<string, unknown>): string {
  return JSON.stringify(value).replace(/</g, '\\u003c').replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029')
}

interface ContentSeo {
  title: string
  description?: string
  url: string
  image?: string
}

export function buildProductStructuredData(input: ContentSeo & {
  images: string[]
  price: string | number | null | undefined
  currency: string
  stockStatus: string
  category: string
}): Record<string, unknown> {
  const data: Record<string, unknown> = {
    '@context': 'https://schema.org', '@type': 'Product', name: input.title, url: input.url,
  }
  if (input.description) data.description = input.description
  if (input.images.length) data.image = input.images
  if (input.category) data.category = input.category
  // Missing prices must not be advertised as free products.
  if (input.price !== null && input.price !== undefined && String(input.price).trim() !== '' &&
      Number.isFinite(Number(input.price)) && Number(input.price) >= 0) {
    const offer: Record<string, unknown> = {
      '@type': 'Offer', url: input.url, price: input.price, priceCurrency: input.currency,
    }
    if (input.stockStatus === 'out_of_stock') offer.availability = 'https://schema.org/OutOfStock'
    else if (['in_stock', 'low_stock', 'unlimited'].includes(input.stockStatus)) offer.availability = 'https://schema.org/InStock'
    data.offers = offer
  }
  return data
}

export function buildArticleStructuredData(input: ContentSeo & {
  locale: string
  publishedAt?: string | null
  publisher: string
}): Record<string, unknown> {
  const data: Record<string, unknown> = {
    '@context': 'https://schema.org', '@type': 'Article',
    headline: input.title, mainEntityOfPage: input.url, url: input.url, inLanguage: input.locale,
  }
  if (input.description) data.description = input.description
  if (input.image) data.image = [input.image]
  if (input.publishedAt && !Number.isNaN(Date.parse(input.publishedAt))) data.datePublished = input.publishedAt
  if (input.publisher) data.publisher = { '@type': 'Organization', name: input.publisher }
  return data
}
