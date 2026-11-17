<script setup>
import { onBeforeUnmount, ref, watch, nextTick, useId } from 'vue'
import DropdownSelect from './DropdownSelect.vue'
import CheckboxField from './CheckboxField.vue'
import { attachToasts } from './toast-host.ts'
const formId = useId()
const props = defineProps({
  form: { type: Object, required: true },
  msg: { type: Function, required: true },
})
const dialog = ref(/** @type {HTMLDialogElement | null} */ (null))
let detachToasts = () => {}
onBeforeUnmount(() => detachToasts())
watch(
  () => props.form.open,
  async (open) => {
    await nextTick()
    if (open && dialog.value) {
      dialog.value.showModal()
      detachToasts = attachToasts(dialog.value)
    } else {
      detachToasts()
      dialog.value?.close()
    }
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
/** @param {Event} event @param {string} key */
async function upload(event, key) {
  const file = /** @type {HTMLInputElement} */ (event.target).files?.[0]
  if (!file) return
  if (file.size > 512 * 1024) {
    props.form.validationError = props.msg('apps.logoLimit')
    return
  }
  props.form.busy = true
  try {
    const bytes = new Uint8Array(await file.arrayBuffer())
    let binary = ''
    for (const byte of bytes) binary += String.fromCharCode(byte)
    props.form.values[key] = btoa(binary)
  } catch {
    props.form.validationError = props.msg('apps.logoLimit')
  } finally {
    props.form.busy = false
  }
}
</script>
<template>
  <dialog ref="dialog" class="dialog-panel" :aria-label="form.title" @cancel="cancel">
    <form class="space-y-3" @submit.prevent="form.submit">
      <h2 class="text-base font-semibold">{{ form.title }}</h2>
      <template v-if="!form.token">
        <div v-for="field in form.fields" :key="field.key" class="field">
          <label v-if="!field.multiOptions" :for="`${formId}-${field.key}`">{{
            msg(field.label)
          }}</label>
          <fieldset v-if="field.multiOptions" :disabled="form.busy" class="space-y-2">
            <legend>{{ msg(field.label) }}</legend>
            <CheckboxField
              v-for="option in field.multiOptions"
              :key="option.value"
              v-model="form.choices[field.key]"
              :value="option.value"
              :label="option.label"
              :disabled="form.busy"
            />
          </fieldset>
          <DropdownSelect
            v-else-if="field.options"
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
            v-else-if="field.datetime"
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            type="datetime-local"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          />
          <input
            v-else-if="field.password"
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            type="password"
            :autocomplete="field.autocomplete || 'current-password'"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          />
          <input
            v-else-if="field.email"
            :id="`${formId}-${field.key}`"
            v-model="form.values[field.key]"
            type="email"
            autocomplete="email"
            class="input"
            :required="field.required"
            :disabled="form.busy"
          />
          <input
            v-else-if="field.file"
            :id="`${formId}-${field.key}`"
            type="file"
            accept="image/png,image/jpeg"
            class="input"
            :required="field.required"
            :disabled="form.busy"
            @change="upload($event, field.key)"
          />
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
      <p v-if="form.info" class="field whitespace-pre-wrap break-all">{{ form.info }}</p>
      <p v-if="form.token" class="field">
        {{ msg('apps.secretOnce') }}<code class="break-all">{{ form.token }}</code>
      </p>
      <p v-if="form.validationError" class="field">{{ form.validationError }}</p>
      <div class="flex flex-wrap justify-end gap-2">
        <button
          v-if="form.secondaryLabel"
          class="btn-outline"
          type="button"
          :disabled="form.busy"
          @click="form.secondary"
        >
          {{ form.secondaryLabel }}
        </button>
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
