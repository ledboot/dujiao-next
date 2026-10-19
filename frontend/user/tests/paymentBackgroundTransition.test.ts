import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { compile, createRenderer, defineComponent, h, nextTick, onMounted, ref, shallowRef } from 'vue'

// Use both actual route-transition templates from App.vue and the real Vue
// Transition implementation. A background tab's animation frames never fire.
const source = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const templates = [...source.matchAll(/<Transition name="page-fade"[\s\S]*?<\/Transition>/g)].map(match => match[0])

type Node = { parent: Node | null; children: Node[]; [key: string]: any }
function node(): Node {
  const classes = new Set<string>()
  return {
    parent: null, children: [], ownerDocument: { body: { offsetHeight: 0 } },
    classList: { add: (value: string) => classes.add(value), remove: (value: string) => classes.delete(value) },
  }
}
const renderer = createRenderer<Node, Node>({
  createElement: node, createText: node, createComment: node,
  insert(child, parent, anchor = null) {
    if (child.parent) child.parent.children.splice(child.parent.children.indexOf(child), 1)
    const index = anchor ? parent.children.indexOf(anchor) : -1
    parent.children.splice(index < 0 ? parent.children.length : index, 0, child)
    child.parent = parent
  },
  remove(child) {
    if (child.parent) child.parent.children.splice(child.parent.children.indexOf(child), 1)
    child.parent = null
  },
  parentNode: child => child.parent,
  nextSibling: child => child.parent?.children[child.parent.children.indexOf(child) + 1] || null,
  setText() {}, setElementText() {}, patchProp() {},
})

async function mountInBackground(template: string, destination = 'payment') {
  const frames: FrameRequestCallback[] = []
  const original = Object.getOwnPropertyDescriptor(globalThis, 'requestAnimationFrame')
  Object.defineProperty(globalThis, 'requestAnimationFrame', { configurable: true, value: (fn: FrameRequestCallback) => frames.push(fn) })
  const route = ref({ name: 'checkout' })
  let mounted = 0
  const Previous = defineComponent({ setup: () => () => h('div') })
  const Payment = defineComponent({ setup() { onMounted(() => mounted++); return () => h('div') } })
  const Component = shallowRef(Previous)
  const app = renderer.createApp(defineComponent({ setup: () => ({ route, Component }), render: compile(template) }))
  try {
    app.mount(node())
    route.value = { name: destination }
    Component.value = Payment
    await nextTick()
    await nextTick()
    return { mounted, frames: frames.length }
  } finally {
    app.unmount()
    if (original) Object.defineProperty(globalThis, 'requestAnimationFrame', original)
    else Reflect.deleteProperty(globalThis, 'requestAnimationFrame')
  }
}

test('original out-in CSS transition blocks payment mounting when background frames pause', async () => {
  const result = await mountInBackground('<Transition name="page-fade" mode="out-in"><component :is="Component" /></Transition>')
  assert.equal(result.mounted, 0)
  assert.ok(result.frames > 0)
})

test('classic and vault mount payment immediately without background animation frames', async () => {
  assert.equal(templates.length, 2)
  for (const template of templates) {
    const result = await mountInBackground(template)
    assert.equal(result.mounted, 1)
    assert.equal(result.frames, 0)
  }
})


test('other routes retain their existing page transition', async () => {
  for (const template of templates) {
    const result = await mountInBackground(template, 'products')
    assert.equal(result.mounted, 0)
    assert.ok(result.frames > 0)
  }
})
