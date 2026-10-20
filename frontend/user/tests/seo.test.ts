import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildArticleStructuredData, buildProductStructuredData, isIndexableStorefrontRoute,
  resolveCanonicalUrl, resolveSeoBaseUrl, resolveSeoImageUrl, serializeJsonLd,
} from '../src/utils/seo.ts'

test('only public storefront content can be indexed', () => {
  for (const name of ['home', 'products', 'category-products', 'product-detail', 'blog', 'blog-detail', 'notice', 'about', 'terms', 'privacy']) {
    assert.equal(isIndexableStorefrontRoute(name), true, name)
  }
  for (const name of ['cart', 'checkout', 'payment', 'personal-center', 'personal-center-orders', 'order-detail', 'recharge-order-detail', 'guest-orders', 'guest-order-detail', 'user-login', 'user-register', 'user-forgot', 'user-google-callback', 'user-telegram-callback', 'reseller-dashboard', 'not-found', 'new-route', undefined]) {
    assert.equal(isIndexableStorefrontRoute(name), false, String(name))
  }
})

test('canonical URLs use the configured storefront and discard tracking data', () => {
  const base = resolveSeoBaseUrl(' https://shop.example.com/path/?utm_source=test#fragment ', 'https://main.example.com')
  assert.equal(base, 'https://shop.example.com')
  assert.equal(resolveCanonicalUrl('/products/item?affiliate=private#details', base), 'https://shop.example.com/products/item')
  assert.equal(resolveCanonicalUrl('/products/' + encodeURIComponent('商品/a?b#c'), base), 'https://shop.example.com/products/%E5%95%86%E5%93%81%2Fa%3Fb%23c')
  for (const path of ['//other.example.com/path', '/\\other.example.com/path']) {
    assert.equal(new URL(resolveCanonicalUrl(path, base)).origin, base)
  }
})

test('invalid or unsafe site URLs fall back to the current origin', () => {
  for (const configured of ['', 'not-a-url', 'javascript:alert(1)', 'https://user:password@example.com']) {
    assert.equal(resolveSeoBaseUrl(configured, 'https://tenant.example.com'), 'https://tenant.example.com')
  }
  assert.equal(resolveSeoBaseUrl('', ''), '')
  assert.equal(resolveCanonicalUrl('/products', ''), '')
})

test('sharing and structured images are absolute HTTP URLs', () => {
  const base = 'https://shop.example.com'
  assert.equal(resolveSeoImageUrl('/uploads/item.png', base), base + '/uploads/item.png')
  assert.equal(resolveSeoImageUrl('//cdn.example.com/item.png', base), 'https://cdn.example.com/item.png')
  assert.equal(resolveSeoImageUrl('https://cdn.example.com/item.png', base), 'https://cdn.example.com/item.png')
  for (const image of ['', 'javascript:alert(1)', 'data:image/png;base64,a']) {
    assert.equal(resolveSeoImageUrl(image, base), '')
  }
})

test('product structured data preserves price precision and stock without fabricating missing prices', () => {
  const input = {
    title: '商品', description: '商品描述', url: 'https://shop.example.com/products/item',
    images: ['https://shop.example.com/uploads/item.png'], price: '19.90', currency: 'CNY',
    stockStatus: 'out_of_stock', category: '分类',
  }
  const data = buildProductStructuredData(input)
  assert.deepEqual(data.offers, {
    '@type': 'Offer', url: input.url, price: '19.90', priceCurrency: 'CNY',
    availability: 'https://schema.org/OutOfStock',
  })
  for (const price of [undefined, null, '', 'bad-price', '-1', 'Infinity']) {
    assert.equal(buildProductStructuredData({ ...input, price }).offers, undefined)
  }
  assert.equal((buildProductStructuredData({ ...input, price: 0, stockStatus: 'unlimited' }).offers as any).price, 0)
  assert.equal((buildProductStructuredData({ ...input, stockStatus: 'unknown' }).offers as any).availability, undefined)
})

test('article schema includes real publication information without inventing authors or dates', () => {
  const data = buildArticleStructuredData({
    title: '公告', url: 'https://shop.example.com/blog/notice', description: '摘要',
    image: 'https://shop.example.com/uploads/notice.png', locale: 'zh-TW',
    publishedAt: '2026-10-20T01:00:00Z', publisher: '店铺',
  })
  assert.equal(data.datePublished, '2026-10-20T01:00:00Z')
  assert.equal(data.inLanguage, 'zh-TW')
  assert.deepEqual(data.publisher, { '@type': 'Organization', name: '店铺' })
  assert.equal(data.author, undefined)
  assert.equal(data.dateModified, undefined)
  assert.equal(buildArticleStructuredData({ title: 'Test', url: 'https://example.com', locale: 'en-US', publishedAt: 'invalid', publisher: '' }).datePublished, undefined)
})

test('merchant text cannot terminate JSON-LD script elements', () => {
  const input = { name: '</script><script>alert(1)</script>\u2028\u2029' }
  const json = serializeJsonLd(input)
  assert.equal(json.includes('<'), false)
  assert.deepEqual(JSON.parse(json), input)
})
