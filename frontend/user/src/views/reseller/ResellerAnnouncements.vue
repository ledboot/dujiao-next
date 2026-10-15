<template>
  <div class="space-y-6">
    <ResellerSectionHeader :title="t(`${key}.title`)" :description="t(`${key}.description`)">
      <template #actions><Button size="sm" :disabled="busy" @click="edit()">{{ t(`${key}.create`) }}</Button></template>
    </ResellerSectionHeader>
    <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
    <p v-if="success" role="status" class="text-sm text-success">{{ success }}</p>
    <Card v-if="form" class="space-y-5 p-5">
      <h2 class="text-base font-bold text-foreground">{{ t(`${key}.${form.id ? 'editTitle' : 'create'}`) }}</h2>
      <div class="block">
        <label for="announcement-type" class="mb-2 block text-sm font-medium text-muted-foreground">{{ t(`${key}.type`) }}</label>
        <Select v-model="form.type">
          <SelectTrigger id="announcement-type" class="h-11">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="notice">{{ t(`${key}.notice`) }}</SelectItem>
            <SelectItem value="blog">{{ t(`${key}.blog`) }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <label class="block">
        <span class="mb-2 block text-sm font-medium text-muted-foreground">{{ t(`${key}.heading`) }}</span>
        <Input v-model="form.title[language]" class="h-11 text-base font-semibold" required />
        <span class="mt-2 block text-xs text-muted-foreground">{{ postTextLength(form.title[language] || '') }}/120 · {{ t(`${key}.titleHint`) }}</span>
      </label>
      <label class="block">
        <span class="mb-2 block text-sm font-medium text-muted-foreground">{{ t(`${key}.summary`) }}</span>
        <Input v-model="form.summary[language]" class="h-11" />
        <span class="mt-2 block text-xs text-muted-foreground">{{ postTextLength(form.summary[language] || '') }}/300 · {{ t(`${key}.optional`) }}</span>
      </label>
      <div class="block">
        <p class="mb-2 text-sm font-medium text-muted-foreground">{{ t(`${key}.content`) }}</p>
        <ResellerRichText :key="`${form.id}-${language}-${editorVersion}`" :model-value="form.content[language] || ''" @update:model-value="form.content[language] = $event" />
        <p class="mt-2 text-xs text-muted-foreground">{{ postContentLength(form.content[language] || '') }}/10000 · {{ t(`${key}.contentHint`) }}</p>
      </div>
      <div class="flex flex-wrap gap-3 border-t pt-4">
        <Button size="sm" :disabled="busy" @click="save(true)">{{ t(`${key}.publish`) }}</Button>
        <Button variant="outline" size="sm" :disabled="busy" @click="save(false)">{{ t(`${key}.saveDraft`) }}</Button>
        <Button variant="ghost" size="sm" :disabled="busy" @click="form = null">{{ t(`${key}.cancel`) }}</Button>
      </div>
    </Card>
    <p v-if="loading" role="status" class="text-muted-foreground">{{ t(`${key}.loading`) }}</p>
    <Card v-else-if="!rows.length" class="p-8 text-center text-muted-foreground">{{ t(`${key}.empty`) }}</Card>
    <div v-else class="space-y-3">
      <Card v-for="row in rows" :key="row.id" class="flex flex-wrap items-center justify-between gap-4 p-5">
        <div class="min-w-0 flex-1">
          <h2 class="break-words font-semibold">{{ localized(row.title) }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t(`${key}.${row.type}`) }} · {{ t(`${key}.${row.is_published ? 'published' : 'draft'}`) }}</p>
        </div>
        <div class="flex gap-2">
          <Button variant="outline" size="sm" :disabled="busy" @click="edit(row)">{{ t(`${key}.edit`) }}</Button>
          <Button variant="destructive" size="sm" :disabled="busy" @click="remove(row)">{{ t(`${key}.delete`) }}</Button>
        </div>
      </Card>
      <PaginationNav :current-page="page" :total-pages="totalPages" :loading="loading || busy" @change-page="load" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { postTextLength, postContentLength, validateResellerPost } from '../../utils/resellerPostValidation'
import { createAnnouncementSlug } from '../../utils/announcementSlug'
import { useI18n } from 'vue-i18n'
import { resellerAPI, type ResellerAnnouncement } from '../../api/reseller'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import PaginationNav from '../../components/PaginationNav.vue'
import ResellerSectionHeader from '../../components/reseller-console/ResellerSectionHeader.vue'
import ResellerRichText from '../../components/reseller/ResellerRichText.vue'

const { t, locale } = useI18n()
const key = 'resellerConsole.announcements'
const rows = ref<ResellerAnnouncement[]>([])
const form = ref<ResellerAnnouncement | null>(null)
const language = ref('zh-CN')
const editorVersion = ref(0)
const page = ref(1)
const totalPages = ref(1)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const success = ref('')
const localized = (value: Record<string, string>) => value[locale.value] || value['zh-CN'] || value['en-US'] || value['zh-TW'] || ''
const emptyText = () => ({ 'zh-CN': '', 'zh-TW': '', 'en-US': '' })

async function load(nextPage = page.value) {
  loading.value = true
  error.value = ''
  try {
    const response = await resellerAPI.announcements(nextPage)
    rows.value = response.data.data || []
    page.value = nextPage
    totalPages.value = response.data.pagination?.total_page || 1
  } catch (cause) { error.value = cause instanceof Error ? cause.message : t(`${key}.error`) }
  finally { loading.value = false }
}
function edit(row?: ResellerAnnouncement) {
  error.value = ''
  success.value = ''
  editorVersion.value++
  language.value = locale.value
  form.value = row ? JSON.parse(JSON.stringify(row)) : {
    id: 0, slug: createAnnouncementSlug(), type: 'notice',
    title: emptyText(), summary: emptyText(), content: emptyText(), is_published: false,
  }
  if (form.value) {
    form.value.title = { ...emptyText(), ...form.value.title }
    form.value.summary = { ...emptyText(), ...form.value.summary }
    form.value.content = { ...emptyText(), ...form.value.content }
  }
}
async function save(published: boolean) {
  if (!form.value || busy.value) return
  const validationError = validateResellerPost(form.value)
  if (validationError) {
    error.value = t(`${key}.${validationError}`)
    return
  }
  busy.value = true
  error.value = ''
  success.value = ''
  const { id, ...data } = form.value
  try {
    const payload = { ...data, is_published: published }
    if (id) await resellerAPI.updateAnnouncement(id, payload)
    else await resellerAPI.createAnnouncement(payload)
    form.value = null
    success.value = t(`${key}.saved`)
    await load(id ? page.value : 1)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : t(`${key}.error`) }
  finally { busy.value = false }
}
async function remove(row: ResellerAnnouncement) {
  if (busy.value || !window.confirm(t(`${key}.confirmDelete`))) return
  busy.value = true
  success.value = ''
  try {
    await resellerAPI.deleteAnnouncement(row.id)
    if (form.value?.id === row.id) form.value = null
    await load(rows.value.length === 1 ? Math.max(1, page.value - 1) : page.value)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : t(`${key}.error`) }
  finally { busy.value = false }
}
onMounted(() => load())
</script>
