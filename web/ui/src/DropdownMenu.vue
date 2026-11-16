<script setup lang="ts">
import { nextTick, ref, useId } from 'vue'
const props = defineProps<{ label: string; disabled?: boolean }>()
const open = ref(false)
const trigger = ref<HTMLButtonElement | null>(null)
const id = useId()
function select() {
  close()
  trigger.value?.focus()
}
function close() {
  open.value = false
}
async function escape() {
  close()
  await nextTick()
  trigger.value?.focus()
}
function blur(event: FocusEvent) {
  if (!(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node | null)) close()
}
</script>
<template>
  <div class="dropdown" @keydown.esc.stop="escape" @focusout="blur">
    <button
      ref="trigger"
      type="button"
      class="btn-ghost"
      :disabled="props.disabled"
      :aria-expanded="open"
      :aria-controls="id"
      @click="open = !open"
    >
      {{ props.label }}<i class="ri-arrow-down-s-line" aria-hidden="true"></i>
    </button>
    <div v-if="open" :id="id" class="dropdown-content" @click="select">
      <slot />
    </div>
  </div>
</template>
