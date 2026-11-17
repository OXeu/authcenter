<script setup>
import { ref, watch, nextTick, useId } from 'vue'
import DropdownSelect from './DropdownSelect.vue'
const formId = useId()
const props = defineProps({
  form: { type: Object, required: true },
  msg: { type: Function, required: true },
})
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
