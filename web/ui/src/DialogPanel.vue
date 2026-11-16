<script setup>
import { computed } from 'vue'

/**
 * @typedef {import('./forms.js').Form} Form
 */

const props = defineProps({
  form: { type: Object, required: true },
  msg: { type: Function, required: true },
})

const emit = defineEmits(['submit', 'cancel'])

const form = /** @type {Form} */ (props.form)
const msg = /** @type {(id: string, vars?: Record<string, string>) => string} */ (props.msg)
const open = computed(() => form.open.value)
</script>

<template>
  <div v-if="open" class="dialog-backdrop" role="presentation"></div>
  <div v-if="open" class="dialog-panel" role="dialog" aria-modal="true" :aria-label="form.title()">
    <form class="space-y-3" @submit.prevent="emit('submit')">
      <h2 class="text-base font-semibold">{{ form.title() }}</h2>
      <slot />
      <p class="field">{{ msg('projects.tokenIssued') }} <code>{{ form.token.value }}</code></p>
      <p class="field">{{ form.error.value }}</p>
      <div class="flex justify-end gap-2">
        <button class="btn-ghost" type="button" @click="emit('cancel')"><i class="ri-close-line mr-1" aria-hidden="true"></i>{{ msg('common.cancel') }}</button>
        <button class="btn" type="submit" :disabled="form.busy.value">{{ form.submitLabel() }}</button>
      </div>
    </form>
  </div>
</template>
