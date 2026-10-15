import test from 'node:test'
import assert from 'node:assert/strict'
import { postTextLength, validateResellerPost } from '../src/utils/resellerPostValidation.ts'

test('post limits count Unicode characters and allow optional summary and body', () => {
  assert.equal(postTextLength('中文😀'), 3)
  const post = { title: { 'zh-CN': '字'.repeat(120) }, summary: {}, content: {} }
  assert.equal(validateResellerPost(post), null)
  assert.equal(validateResellerPost({ ...post, title: { 'zh-CN': ' ' } }), 'titleError')
  assert.equal(validateResellerPost({ ...post, title: { 'zh-CN': '😀'.repeat(121) } }), 'titleError')
  assert.equal(validateResellerPost({ ...post, summary: { 'zh-CN': '😀'.repeat(300) } }), null)
  assert.equal(validateResellerPost({ ...post, summary: { 'en-US': 'a'.repeat(301) } }), 'summaryError')
})
