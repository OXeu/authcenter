<script setup lang="ts">
import type { DangerAction } from './danger.ts'
defineProps<{ action: DangerAction; disabled: boolean }>()
</script>
<template>
  <button
    type="button"
    class="btn-danger"
    :disabled="disabled"
    :data-danger-action="action.id"
    @click="action.request()"
  >
    {{ action.label }}
  </button>
</template>
