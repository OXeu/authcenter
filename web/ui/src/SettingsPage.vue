<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import DialogPanel from './DialogPanel.vue'
import { createForm } from './forms.ts'
import { api } from './core.ts'
import { notify } from './toast.ts'
import type { PageContext } from './page.ts'

type Setting = {
  key: string
  value: string
  secret: boolean
  configured: boolean
  restart: boolean
  pending: boolean
  options?: string[]
}
const props = defineProps<{ page: PageContext }>()
const { msg } = props.page
const form = createForm(props.page)
const state = reactive({ items: [] as Setting[], busy: false })
async function load() {
  state.busy = true
  try {
    const result = (await api('/api/v1/settings')) as { items: Setting[] }
    state.items = result.items
  } catch (error) {
    props.page.report(error)
  } finally {
    state.busy = false
  }
}
function label(item: Setting) {
  return msg(`settings.key.${item.key}`)
}
function value(item: Setting) {
  if (item.secret) return msg(item.configured ? 'settings.secretSet' : 'settings.empty')
  return item.value || msg('settings.empty')
}
function edit(item?: Setting) {
  const items = item ? [item] : state.items
  const fields = items.map((entry) => ({
    key: entry.key,
    label: `settings.key.${entry.key}`,
    password: entry.secret,
    autocomplete: 'new-password',
    options: entry.options?.map((option) => ({
      value: option,
      label: option || msg('settings.empty'),
    })),
  }))
  const values = Object.fromEntries(items.map((entry) => [entry.key, entry.value]))
  form.show(msg('settings.configure'), fields, values, async () => {
    const updates = Object.fromEntries(
      items
        .filter((entry) => {
          if (item?.secret) return true
          return entry.secret
            ? Boolean(form.values[entry.key])
            : form.values[entry.key] !== entry.value
        })
        .map((entry) => [entry.key, form.values[entry.key]]),
    )
    if (!Object.keys(updates).length) return
    await api('/api/v1/settings', {
      method: 'PUT',
      csrf: props.page.csrf,
      body: { values: updates },
    })
    notify('success', msg('settings.saved'))
    await load()
  })
  form.info = msg(item?.secret ? 'settings.secretHelp' : 'settings.editHelp')
}
async function refreshConfig() {
  state.busy = true
  try {
    const result = (await api('/api/v1/settings/refresh', {
      method: 'POST',
      csrf: props.page.csrf,
    })) as { restart_required: string[] | null }
    const description = result.restart_required?.length
      ? msg('settings.restartNeeded', { keys: result.restart_required.join(', ') })
      : undefined
    notify('success', msg('settings.refreshed'), description)
    await load()
  } catch (error) {
    props.page.report(error)
  } finally {
    state.busy = false
  }
}
onMounted(load)
</script>
<template>
  <p class="mt-2 text-sm text-muted-foreground">{{ msg('settings.help') }}</p>
  <div class="my-4 flex flex-wrap gap-2">
    <button class="btn" :disabled="state.busy" @click="edit()">
      {{ msg('settings.configure') }}
    </button>
    <button class="btn-outline" :disabled="state.busy" @click="refreshConfig">
      {{ msg('settings.refresh') }}
    </button>
    <button class="btn-ghost" :disabled="state.busy" @click="load">
      {{ msg('common.refresh') }}
    </button>
  </div>
  <div class="table-scroll">
    <table class="table" :aria-label="msg('title.settings')" :aria-busy="state.busy">
      <thead>
        <tr>
          <th scope="col">{{ msg('settings.key') }}</th>
          <th scope="col">{{ msg('settings.value') }}</th>
          <th scope="col">{{ msg('settings.applies') }}</th>
          <th scope="col">{{ msg('settings.state') }}</th>
          <th scope="col">{{ msg('common.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="item in state.items" :key="item.key">
          <td :data-label="msg('settings.key')">
            <p>{{ label(item) }}</p>
            <code class="text-xs">{{ item.key }}</code>
          </td>
          <td class="break-all" :data-label="msg('settings.value')">{{ value(item) }}</td>
          <td :data-label="msg('settings.applies')">
            {{ msg(item.restart ? 'settings.restart' : 'settings.live') }}
          </td>
          <td :data-label="msg('settings.state')">
            {{ msg(item.pending ? 'settings.pending' : 'settings.active') }}
          </td>
          <td :data-label="msg('common.actions')">
            <button class="btn-ghost" :disabled="state.busy" @click="edit(item)">
              {{ msg('common.edit') }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
