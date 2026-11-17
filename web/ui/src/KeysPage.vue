<script setup lang="ts">
import { onMounted } from 'vue'
import DangerActions from './DangerActions.vue'
import DialogPanel from './DialogPanel.vue'
import { createKeysWorkspace, type APIKey } from './workspace.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ page: PageContext }>()
const { msg } = props.page
const { state, form, refresh, edit, actionsFor } = createKeysWorkspace(props.page)
function scopes(row: APIKey) {
  return row.scopes.map((scope) => msg(`scope.${scope}`)).join(', ') || msg('keys.noScopes')
}
function date(value: string | null) {
  return value ? new Date(value).toLocaleString() : '—'
}
onMounted(refresh)
</script>
<template>
  <p class="mt-2 text-sm text-muted-foreground">{{ msg('keys.description') }}</p>
  <div class="my-4 flex flex-wrap gap-2">
    <button class="btn" :disabled="state.busy" @click="edit()">{{ msg('common.create') }}</button>
    <button class="btn-outline" :disabled="state.busy" @click="refresh">
      {{ msg('common.refresh') }}
    </button>
  </div>
  <div class="table-scroll">
    <table class="table" :aria-label="msg('title.keys')" :aria-busy="state.busy">
      <thead>
        <tr>
          <th scope="col">{{ msg('common.name') }}</th>
          <th scope="col">{{ msg('keys.prefix') }}</th>
          <th scope="col">{{ msg('keys.scopes') }}</th>
          <th scope="col">{{ msg('keys.created') }}</th>
          <th scope="col">{{ msg('keys.lastUsed') }}</th>
          <th scope="col">{{ msg('keys.expires') }}</th>
          <th scope="col">{{ msg('common.actions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in state.rows" :key="row.id">
          <td>{{ row.name }}</td>
          <td><code>{{ row.prefix }}</code></td>
          <td>{{ scopes(row) }}</td>
          <td>{{ date(row.created_at) }}</td>
          <td>{{ date(row.last_used_at) }}</td>
          <td>{{ date(row.expires_at) }}</td>
          <td>
            <button class="btn-ghost" :disabled="state.busy" @click="edit(row)">
              {{ msg('common.edit') }}
            </button>
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
    {{ msg('keys.empty') }}
  </p>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
