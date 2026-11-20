<script setup lang="ts">
import Button from '@oxeu/center-common/ui/Button.vue'
import DropdownMenu from '@oxeu/center-common/ui/DropdownMenu.vue'
import { computed, onMounted } from 'vue'
import DangerActions from './DangerActions.vue'
import DialogPanel from './DialogPanel.vue'
import { createWorkspace } from './workspace.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext; title: string }>()
const { data, page } = props
const { msg } = page
const { state, form, refresh, select, edit, editChoices, choose, actionsFor } = createWorkspace(
  data,
  page,
)
const choiceEditor = computed(() => data.page === 'users')
const applications = computed(() => data.page === 'applications')
const clients = computed(() => applications.value && state.tab === 'clients')
const columns = computed(() =>
  !applications.value
    ? ['name', 'email', 'role', 'banned', 'banned_until']
    : clients.value
      ? ['name', 'id', 'homepage', 'redirect_uris']
      : ['client_id', 'scope'],
)
const labels: Record<string, string> = {
  name: 'common.name',
  id: 'apps.clientId',
  homepage: 'apps.homepage',
  redirect_uris: 'apps.callbacks',
  client_id: 'apps.clientId',
  scope: 'apps.scopes',
  email: 'users.email',
  role: 'users.role',
  banned: 'users.status',
  banned_until: 'users.bannedUntil',
}
function cell(row: Record<string, any>, column: string) {
  if (column === 'banned') return msg(row.banned ? 'users.banned' : 'users.active')
  if (column === 'banned_until')
    return row.banned_until ? new Date(row.banned_until).toLocaleString() : '—'
  if (column === 'role') return msg(`role.${row[column]}`)
  return Array.isArray(row[column]) ? row[column].join('\n') : row[column]
}
function logo(row: Record<string, any>) {
  return `/applications/${encodeURIComponent(row.id)}/logo`
}
function showLogo(row: Record<string, any>, column: string) {
  return clients.value && column === 'name' && !row.dynamic
}
function canEdit(row: Record<string, any>) {
  return !applications.value || (clients.value && !row.dynamic)
}
onMounted(refresh)
</script>
<template>
  <p v-if="applications" class="mt-2 text-sm text-muted-foreground">
    {{ msg('apps.description') }}
  </p>
  <div class="my-4 flex flex-wrap gap-2">
    <template v-if="applications">
      <Button
        :data-active="clients"
        :disabled="state.busy"
        @click="select('clients')"
        variant="ghost"
        size="sm"
      >
        {{ msg('apps.clients') }}
      </Button>
      <Button
        :data-active="!clients"
        :disabled="state.busy"
        @click="select('grants')"
        variant="ghost"
        size="sm"
      >
        {{ msg('apps.grants') }}
      </Button>
    </template>
    <Button :disabled="state.busy" @click="refresh" variant="secondary">
      {{ msg('common.refresh') }}
    </Button>
    <Button v-if="clients" :disabled="state.busy" @click="edit()" variant="primary">
      {{ msg('common.create') }}
    </Button>
  </div>
  <div class="table-scroll">
    <table class="table" :aria-label="title" :aria-busy="state.busy">
      <thead>
        <tr>
          <th v-for="column in columns" :key="column" scope="col">{{ msg(labels[column]) }}</th>
          <th scope="col">{{ msg('common.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in state.rows" :key="row.id">
          <td v-for="column in columns" :key="column" :data-label="msg(labels[column])">
            <img
              v-if="showLogo(row, column)"
              :src="logo(row)"
              alt=""
              width="32"
              height="32"
              class="mb-2 rounded-sm"
            />
            <span class="whitespace-pre-wrap break-all">{{ cell(row, column) }}</span>
          </td>
          <td :data-label="msg('common.actions')">
            <DropdownMenu v-if="choiceEditor" :disabled="state.busy" :label="msg('common.edit')">
              <Button
                v-for="choice in editChoices(row)"
                :key="choice.value"
                :aria-pressed="choice.selected"
                :data-active="choice.selected"
                :disabled="state.busy"
                @click="choose(row, choice.value)"
                variant="ghost"
                block
                align="start"
              >
                {{ choice.label }}
              </Button>
            </DropdownMenu>
            <Button
              v-else-if="canEdit(row)"
              :disabled="state.busy"
              @click="edit(row)"
              variant="ghost"
            >
              {{ msg('common.edit') }}
            </Button>
            <DangerActions
              :actions="actionsFor(row)"
              :label="msg('common.more')"
              :disabled="state.busy"
            />
          </td>
        </tr>
      </tbody>
    </table>
  </div>
  <p v-if="!state.busy && !state.rows.length" class="mt-4 text-sm text-muted-foreground">
    {{ msg('apps.empty') }}
  </p>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
