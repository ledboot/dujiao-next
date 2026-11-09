<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { notifyError, notifySuccess } from '@/utils/notify'
import { formatDate } from '@/utils/format'

interface Config {
  enabled: boolean
  quote_currency: string
  markup_percent: string
  markup_amount: string
  refresh_minutes: number
  max_deviation_percent: string
  max_change_percent: string
  manual_rate: string
  fallback_rate: string
  sources: Array<{ id: string; enabled: boolean; url: string }>
}
interface Snapshot {
  market_rate: string; rate: string; updated_at: string; mode: string
  sources: Array<{ id: string; rate: string; published_at: string; status: string }>
}
const { t } = useI18n()
const form = ref<Config | null>(null)
const snapshot = ref<Snapshot | null>(null)
const available = ref(false)
const busy = ref(false)
const quoteDate = (value?: string) => !value || value.startsWith('0001-') ? '-' : formatDate(value)
const names: Record<string, string> = { frankfurter: 'Frankfurter (ECB)', open_er: 'open.er-api.com', currency_api: 'currency-api (jsDelivr)', exchangerate_fun: 'exchangerate.fun' }
const fields = ['markup_percent', 'markup_amount', 'refresh_minutes', 'max_deviation_percent', 'max_change_percent', 'manual_rate', 'fallback_rate'] as const
const load = async () => {
  busy.value = true
  try {
    const { data } = await adminAPI.getExchangeRateSettings()
    form.value = data.data.config
    snapshot.value = data.data.snapshot
    available.value = data.data.available
  } catch (error: any) { notifyError(error?.message || t('admin.exchangeRate.loadFailed')) }
  finally { busy.value = false }
}
const save = async () => {
  if (!form.value) return
  busy.value = true
  try {
    await adminAPI.updateExchangeRateSettings({ ...form.value, refresh_minutes: Number(form.value.refresh_minutes) })
    notifySuccess(t('admin.exchangeRate.saved'))
    await load()
  } catch (error: any) { notifyError(error?.message || t('admin.exchangeRate.saveFailed')) }
  finally { busy.value = false }
}
const refresh = async () => {
  busy.value = true
  try {
    const { data } = await adminAPI.refreshExchangeRate()
    snapshot.value = data.data
    available.value = true
  } catch (error: any) { available.value = false; notifyError(error?.message || t('admin.exchangeRate.unavailable')) }
  finally { busy.value = false }
}
onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h1 class="text-2xl font-semibold">{{ t('admin.exchangeRate.title') }}</h1>
      <Button variant="outline" :disabled="busy" @click="refresh">{{ t('admin.exchangeRate.refresh') }}</Button>
    </div>
    <section class="space-y-3 rounded-xl border border-border bg-card p-5">
      <h2 class="font-semibold">{{ t('admin.exchangeRate.current') }}</h2>
      <div class="flex flex-wrap items-baseline gap-3">
        <strong class="font-mono text-2xl">{{ available ? snapshot?.rate : '-' }}</strong>
        <span class="text-sm text-muted-foreground">{{ t('admin.exchangeRate.unit') }}</span>
      </div>
      <p v-if="!available" class="text-sm text-destructive">{{ t('admin.exchangeRate.unavailable') }}</p>
      <div v-if="snapshot" class="flex flex-wrap gap-x-8 gap-y-2 text-sm text-muted-foreground">
        <span>{{ t('admin.exchangeRate.market') }}: {{ snapshot.market_rate || '-' }}</span>
        <span v-if="snapshot.mode">{{ t(`admin.exchangeRate.modes.${snapshot.mode}`) }}</span>
        <span>{{ t('admin.exchangeRate.updated') }}: {{ quoteDate(snapshot.updated_at) }}</span>
      </div>
    </section>
    <section class="rounded-xl border border-border bg-card p-5">
      <h2 class="mb-3 font-semibold">{{ t('admin.exchangeRate.sources') }}</h2>
      <div class="overflow-x-auto">
        <Table>
          <TableHeader><TableRow><TableHead>{{ t('admin.exchangeRate.source') }}</TableHead><TableHead>{{ t('admin.exchangeRate.quote') }}</TableHead><TableHead>{{ t('admin.exchangeRate.published') }}</TableHead><TableHead>{{ t('admin.exchangeRate.status') }}</TableHead></TableRow></TableHeader>
          <TableBody><TableRow v-for="source in snapshot?.sources || []" :key="source.id"><TableCell>{{ names[source.id] }}</TableCell><TableCell>{{ Number(source.rate) > 0 ? source.rate : '-' }}</TableCell><TableCell>{{ quoteDate(source.published_at) }}</TableCell><TableCell :class="source.status !== 'ok' ? 'text-destructive' : ''">{{ t(`admin.exchangeRate.statuses.${source.status}`) }}</TableCell></TableRow></TableBody>
        </Table>
      </div>
      <a href="https://www.exchangerate-api.com" target="_blank" rel="noopener noreferrer" class="mt-3 inline-block text-xs text-muted-foreground underline">Rates By Exchange Rate API</a>
    </section>
    <form v-if="form" class="space-y-5" @submit.prevent="save">
      <section class="space-y-4 rounded-xl border border-border bg-card p-5">
        <h2 class="font-semibold">{{ t('admin.exchangeRate.conversion') }}</h2>
        <Label class="flex items-center gap-2"><Switch v-model="form.enabled" />{{ t('admin.exchangeRate.enabled') }}</Label>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div class="space-y-2"><Label>{{ t('admin.exchangeRate.currency') }}</Label><Input v-model="form.quote_currency" readonly /></div>
          <div v-for="field in fields" :key="field" class="space-y-2">
            <Label :for="field">{{ t(`admin.exchangeRate.fields.${field}`) }}</Label>
            <Input :id="field" v-model="form[field]" type="number" min="0" :step="field === 'refresh_minutes' ? '1' : '0.000001'" required />
          </div>
        </div>
        <p class="text-xs text-muted-foreground">{{ t('admin.exchangeRate.formula') }}</p>
      </section>
      <section class="space-y-5 rounded-xl border border-border bg-card p-5">
        <h2 class="font-semibold">{{ t('admin.exchangeRate.sources') }}</h2>
        <div v-for="source in form.sources" :key="source.id" class="space-y-2">
          <Label class="flex items-center gap-2"><Switch v-model="source.enabled" />{{ names[source.id] }}</Label>
          <Input v-model.trim="source.url" type="url" required :aria-label="names[source.id]" />
        </div>
      </section>
      <Button type="submit" :disabled="busy">{{ t('admin.common.save') }}</Button>
    </form>
  </div>
</template>
