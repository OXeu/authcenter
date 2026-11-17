<script setup lang="ts">
import DropdownMenu from './DropdownMenu.vue'
import DangerActionButton from './DangerActionButton.vue'
import type { DangerAction } from './danger.ts'
defineProps<{ actions: DangerAction[]; label: string; disabled: boolean }>()
</script>
<template>
  <DropdownMenu v-if="actions.length" :label="label" :disabled="disabled">
    <DangerActionButton
      v-for="action in actions"
      :key="action.id"
      :action="action"
      :disabled="disabled"
    />
  </DropdownMenu>
</template>
