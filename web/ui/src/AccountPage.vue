<script setup lang="ts">
import Button from '@oxeu/center-common/ui/Button.vue'
import { onMounted } from 'vue'
import DialogPanel from './DialogPanel.vue'
import DangerActions from './DangerActions.vue'
import { createAccount } from './account.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext }>()
const { msg } = props.page
const { state, form, load, password, email, enroll, actions } = createAccount(props.page)
onMounted(load)
</script>
<template>
  <div class="mt-4 space-y-4" :aria-busy="state.busy">
    <Button :disabled="state.busy" @click="load" variant="secondary">
      {{ msg('common.refresh') }}
    </Button>
    <template v-if="state.ready">
      <section class="card space-y-3">
        <h2 class="font-semibold">{{ msg('account.password') }}</h2>
        <p>{{ msg(state.password_enabled ? 'account.passwordSet' : 'account.passwordUnset') }}</p>
        <p v-if="!state.local_login" class="text-sm text-muted-foreground">
          {{ msg('account.localDisabled') }}
        </p>
        <Button v-if="state.local_login" :disabled="state.busy" @click="password" variant="primary">
          {{ msg('account.setPassword') }}
        </Button>
      </section>
      <section class="card space-y-3">
        <h2 class="font-semibold">{{ msg('users.email') }}</h2>
        <p>{{ state.email }}</p>
        <p v-if="!state.email_available" class="text-sm text-muted-foreground">
          {{ msg('account.emailUnavailable') }}
        </p>
        <Button
          v-if="state.email_available"
          :disabled="state.busy"
          @click="email"
          variant="primary"
        >
          {{ msg('account.changeEmail') }}
        </Button>
      </section>
      <section class="card space-y-3">
        <h2 class="font-semibold">{{ msg('account.authenticator') }}</h2>
        <p>{{ msg(state.totp_enabled ? 'account.enabled' : 'account.disabled') }}</p>
        <p class="text-sm text-muted-foreground">{{ msg('account.help') }}</p>
        <Button
          v-if="state.local_login && !state.totp_enabled"
          :disabled="state.busy"
          @click="enroll"
          variant="primary"
        >
          {{ msg('account.enable') }}
        </Button>
        <DangerActions :actions="actions()" :label="msg('common.more')" :disabled="state.busy" />
      </section>
    </template>
  </div>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
