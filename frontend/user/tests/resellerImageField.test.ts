import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'
import { computed, ref, watch } from 'vue'

const source = readFileSync(new URL('../src/components/reseller/ResellerImageField.vue', import.meta.url), 'utf8')
function setup(uploadImage: (file: unknown) => Promise<unknown>) {
  const events: Array<[string, unknown]> = []
  const script = source.match(/<script setup lang="ts">([\s\S]*?)<\/script>/)![1]
  const compiled = ts.transpileModule(`${script}\nexport { onFileChange, uploading, error };`, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const exports: any = {}
  const mocks: Record<string, unknown> = {
    vue: { computed, ref, watch },
    'vue-i18n': { useI18n: () => ({ t: (key: string) => key }) },
    '../../api': { resellerAPI: { uploadImage } },
    '../../utils/image': { getImageUrl: (url: string) => url },
  }
  runInNewContext(compiled, {
    exports, require: (name: string) => mocks[name] || {},
    defineProps: () => ({ modelValue: '' }),
    defineEmits: () => (event: string, value: unknown) => events.push([event, value]),
  })
  return { ...exports, events }
}

test('image selection has one directly clickable native file input', () => {
  const template = source.split('<script')[0]
  assert.equal((template.match(/type="file"/g) || []).length, 1)
  assert.match(template, /class="absolute inset-0 h-full w-full/)
  assert.doesNotMatch(template, /triggerUpload|class="hidden"/)
  assert.equal((template.match(/\{\{ uploadLabel \}\}/g) || []).length, 1)
})

test('image upload emits URL and releases busy state; cancellation does not upload', async () => {
  const files: unknown[] = []
  const field = setup(async file => { files.push(file); return { data: { data: { url: '/uploads/test.png' } } } })
  await field.onFileChange({ target: { files: [], value: '' } })
  assert.equal(files.length, 0)
  const file = { name: 'test.png' }
  const input = { files: [file], value: 'test.png' }
  await field.onFileChange({ target: input })
  assert.equal(input.value, '')
  assert.equal(files[0], file)
  assert.deepEqual(field.events, [['uploading', true], ['update:modelValue', '/uploads/test.png'], ['uploading', false]])
  assert.equal(field.uploading.value, false)
})

for (const missingURL of [false, true]) {
  test(`failed upload remains retryable (missing URL: ${missingURL})`, async () => {
    const field = setup(async () => { if (missingURL) return { data: { data: {} } }; throw Error('upload rejected') })
    await field.onFileChange({ target: { files: [{}], value: 'test.png' } })
    assert.ok(field.error.value)
    assert.equal(field.uploading.value, false)
    assert.deepEqual(field.events, [['uploading', true], ['uploading', false]])
  })
}
