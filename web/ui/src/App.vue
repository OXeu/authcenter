<script setup>
import { computed, onMounted } from 'vue'
import PageHeader from './PageHeader.vue'
import DialogPanel from './DialogPanel.vue'
import { createPage } from './page.js'
import { createWorkspace } from './workspace.js'
import { mountToasts, notify } from './toast.js'

const props = defineProps({ data: { type: Object, required: true } })
const data = /** @type {import('./data.js').PageData} */ (props.data)
const page = createPage(data)
const { msg } = page
const { state, form, refresh, select, edit, remove, review, environment, newEnvironment } =
  createWorkspace(data, page)
const title = computed(
  () => data.project?.name ?? msg(data.page === 'login' ? 'login.title' : data.title),
)
const canCreate = computed(
  () => data.canWrite && data.page !== 'users' && state.tab !== 'proposals',
)
const reviewing = computed(() => data.page === 'project' && state.tab === 'proposals')
const columns = computed(() => {
  if (data.page === 'projects') return ['name', 'slug', 'description']
  if (data.page === 'users') return ['name', 'email', 'role', 'last_login_at']
  if (state.tab === 'keys') return ['name', 'prefix', 'role', 'last_used_at']
  if (state.tab === 'proposals') return ['environment', 'key', 'action', 'value', 'reason']
  return ['key', 'value', 'description', 'updated_at']
})
/** @type {Record<string, string>} */
const labels = {
  name: 'common.name',
  slug: 'common.slug',
  description: 'common.description',
  email: 'users.email',
  role: 'users.role',
  last_login_at: 'users.lastLogin',
  prefix: 'projects.prefix',
  last_used_at: 'projects.lastUsed',
  environment: 'projects.environment',
  key: 'projects.key',
  action: 'projects.proposalAction',
  value: 'projects.value',
  reason: 'projects.proposalReason',
  updated_at: 'projects.updated',
}
/** @param {any} row @param {string} column */
function cell(row, column) {
  if (column === 'role') return msg(`role.${row[column]}`)
  if (column.endsWith('_at'))
    return row[column] ? new Date(row[column]).toLocaleString(data.lang) : ''
  return row[column] ?? ''
}
onMounted(() => {
  mountToasts(data.lang, data.nonce)
  if (data.flash) notify(data.flash.level, data.flash.message)
  if (data.page !== 'login') refresh()
  else if (!data.oauthEnabled) notify('info', msg('login.notConfigured'))
})
</script>
<template>
  <PageHeader :data="data" :msg="msg" :title="title" subtitle="">
    <a v-if="data.page === 'login' && data.oauthEnabled" class="btn mt-4" href="/auth/login">{{
      msg('login.continue', { ProviderName: data.providerName })
    }}</a>
    <template v-if="data.page !== 'login'">
      <div class="flex flex-wrap gap-2 my-4">
        <template v-if="data.page === 'project'">
          <button
            v-for="tab in ['items', 'keys', 'proposals']"
            :key="tab"
            class="tabs-trigger"
            :data-active="state.tab === tab"
            :disabled="state.busy"
            @click="select(tab)"
          >
            {{ msg(`projects.${tab}`) }}
          </button>
          <button class="btn-outline" :disabled="state.busy" @click="environment">
            {{ msg('projects.environment') }}: {{ state.env }}
          </button>
          <button
            v-if="data.canWrite"
            class="btn-outline"
            :disabled="state.busy"
            @click="newEnvironment"
          >
            {{ msg('common.newEnvironment') }}
          </button>
        </template>
        <button class="btn-outline" :disabled="state.busy" @click="refresh">
          {{ msg('common.refresh') }}
        </button>
        <button v-if="canCreate" class="btn" :disabled="state.busy" @click="edit()">
          {{ msg('common.create') }}
        </button>
      </div>
      <div class="overflow-x-auto">
        <table class="table" :aria-label="title" :aria-busy="state.busy">
          <thead>
            <tr>
              <th v-for="column in columns" :key="column" scope="col">{{ msg(labels[column]) }}</th>
              <th v-if="data.canWrite || data.isAdmin" scope="col">{{ msg('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in state.rows" :key="row.id ?? row.slug ?? row.key">
              <td v-for="column in columns" :key="column">
                <a
                  v-if="data.page === 'projects' && column === 'name'"
                  :href="`/projects/${encodeURIComponent(row.slug)}`"
                  >{{ row.name }}</a
                >
                <span v-else class="whitespace-pre-wrap break-all">{{ cell(row, column) }}</span>
              </td>
              <td v-if="data.canWrite || data.isAdmin">
                <template v-if="reviewing">
                  <button class="btn-ghost" :disabled="state.busy" @click="review(row, true)">
                    {{ msg('projects.approve') }}
                  </button>
                  <button class="btn-ghost" :disabled="state.busy" @click="review(row, false)">
                    {{ msg('projects.reject') }}
                  </button>
                </template>
                <template v-else>
                  <button
                    v-if="state.tab !== 'keys'"
                    class="btn-ghost"
                    :disabled="state.busy"
                    @click="edit(row)"
                  >
                    {{ msg('common.edit') }}
                  </button>
                  <button
                    v-if="data.page !== 'users'"
                    class="btn-ghost"
                    :disabled="state.busy"
                    @click="remove(row)"
                  >
                    {{ msg(state.tab === 'keys' ? 'projects.revoke' : 'common.delete') }}
                  </button>
                </template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </PageHeader>
  <DialogPanel :form="form" :msg="msg" />
</template>
