<script setup lang="ts">
import { nextTick, ref, useId } from 'vue'
const props = defineProps<{ label: string; disabled?: boolean; compact?: boolean }>()
const open = ref(false)
const trigger = ref<HTMLButtonElement | null>(null)
const id = useId()
function select(event: MouseEvent) {
  if (!(event.target instanceof Element)) return
  const action = event.target.closest('a[href], button')
  if (!action || action.matches(':disabled, [aria-disabled="true"]')) return
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
      :class="{ 'btn-icon': props.compact }"
      :aria-label="props.label"
      :disabled="props.disabled"
      :aria-expanded="open"
      :aria-controls="id"
      @click="open = !open"
    >
      <slot name="trigger">{{ props.label }}</slot>
    </button>
    <div v-if="open" :id="id" class="dropdown-content" tabindex="-1" @click="select">
      <slot />
    </div>
  </div>
</template>
