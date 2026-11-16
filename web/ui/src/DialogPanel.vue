<script setup>
import { ref, watch, nextTick } from 'vue'
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
        <label v-for="field in form.fields" :key="field.key" class="field">
          {{ msg(field.label) }}
          <select
            v-if="field.options"
            v-model="form.values[field.key]"
            class="input"
            :disabled="form.busy"
          >
            <option v-for="option in field.options" :key="option.value" :value="option.value">
              {{ option.label }}
            </option>
          </select>
          <textarea
            v-else-if="field.multiline"
            v-model="form.values[field.key]"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          ></textarea>
          <input
            v-else
            v-model="form.values[field.key]"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          />
        </label>
      </template>
      <p v-if="form.token" class="field">
        {{ msg('projects.tokenIssued') }}<code>{{ form.token }}</code>
      </p>
      <p v-if="form.error" class="field">{{ form.error }}</p>
      <div class="flex justify-end gap-2">
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
