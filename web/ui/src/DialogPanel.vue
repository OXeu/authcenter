<script setup>
import { computed, ref, watch, nextTick, useId } from 'vue'
import DropdownSelect from './DropdownSelect.vue'
import CheckboxField from './CheckboxField.vue'
const formId = useId()
const props = defineProps({
  form: { type: Object, required: true },
  msg: { type: Function, required: true },
})
const canAddScope = computed(() =>
  props.form.scopeProjects.some(
    (/** @type {import('./forms.ts').Choice} */ option) =>
      !props.form.scopes?.some(
        (/** @type {import('./forms.ts').ScopeDraft} */ scope) => scope.project === option.value,
      ),
  ),
)
const dialog = ref(/** @type {HTMLDialogElement | null} */ (null))
watch(
  () => props.form.open,
  async (open) => {
    await nextTick()
    if (open) dialog.value?.showModal()
    else dialog.value?.close()
  },
  { immediate: true },
)
/** @param {Event} event */
function cancel(event) {
  if (props.form.busy) {
    event.preventDefault()
    return
  }
  props.form.open = false
}
</script>
<template>
  <dialog ref="dialog" class="dialog-panel" :aria-label="form.title" @cancel="cancel">
    <form class="space-y-3" @submit.prevent="form.submit">
      <h2 class="text-base font-semibold">{{ form.title }}</h2>
      <template v-if="!form.token">
        <div v-for="field in form.fields" :key="field.key" class="field">
          <label :for="`${formId}-${field.key}`">{{ msg(field.label) }}</label>
          <DropdownSelect
            v-if="field.options"
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            :label="msg(field.label)"
            :options="field.options"
            :required="field.required"
            :disabled="form.busy"
          />
          <textarea
            v-else-if="field.multiline"
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          ></textarea>
          <input
            v-else
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          />
        </div>
        <section v-if="form.scopes !== null" class="space-y-3">
          <h3 class="text-sm font-medium">{{ msg('keys.scopes') }}</h3>
          <p class="field">{{ msg('keys.scopeHelp') }}</p>
          <fieldset v-for="scope in form.scopes" :key="scope.id" class="card space-y-3">
            <div class="flex items-center gap-2">
              <DropdownSelect
                :id="`${formId}-scope-${scope.id}`"
                v-model="scope.project"
                class="min-w-0 flex-1"
                :label="msg('keys.scopeProject')"
                :options="form.scopeProjects"
                :disabled="form.busy"
                required
              />
              <button
                type="button"
                class="btn-outline btn-icon"
                :aria-label="msg('keys.excludeScope')"
                :disabled="form.busy"
                @click="form.excludeScope(scope.id)"
              >
                <i class="ri-close-line" aria-hidden="true"></i>
              </button>
            </div>
            <div class="grid gap-1 sm:grid-cols-2">
              <CheckboxField
                v-for="permission in form.scopePermissions"
                :key="permission.value"
                v-model="scope.permissions"
                :label="permission.label"
                :value="permission.value"
                :disabled="form.busy"
              />
            </div>
          </fieldset>
          <button
            v-if="canAddScope"
            type="button"
            class="btn-outline"
            :disabled="form.busy"
            @click="form.addScope"
          >
            {{ msg('keys.addScope') }}
          </button>
        </section>
      </template>
      <p v-if="form.token" class="field">
        {{ msg('projects.tokenIssued') }}<code class="break-all">{{ form.token }}</code>
      </p>
      <p v-if="form.error" class="field">{{ form.error }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <button class="btn-ghost" type="button" :disabled="form.busy" @click="form.open = false">
          {{ msg('common.cancel') }}
        </button>
        <button
          v-if="!form.token"
          :class="form.danger ? 'btn-danger' : 'btn'"
          type="submit"
          :disabled="form.busy"
        >
          {{ msg('confirm.ok') }}
        </button>
      </div>
    </form>
  </dialog>
</template>
