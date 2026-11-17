<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DialogPanel from './DialogPanel.vue'
import { createForm } from './forms.ts'
import { api } from './core.ts'
import { notify } from './toast.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext }>()
const { msg } = props.page
const form = createForm(props.page)
const enabled = ref(false)
const password = ref(false)
async function load() {
  if (!props.data.localLogin) return
  try {
    const result = (await api('/auth/local/security')) as {
      totp_enabled: boolean
      password_enabled: boolean
    }
    enabled.value = result.totp_enabled
    password.value = result.password_enabled
  } catch (error) {
    props.page.report(error)
  }
}
function enroll() {
  form.show(
    msg('account.enable'),
    [{ key: 'password', label: 'login.password', password: true, required: true }],
    { password: '' },
    async () => {
      const result = (await api('/auth/local/totp', {
        method: 'POST',
        csrf: props.data.csrf,
        body: form.values,
      })) as { request: string; secret: string; uri: string }
      form.show(
        msg('account.verify'),
        [{ key: 'code', label: 'login.code', required: true }],
        { code: '' },
        async () => {
          await api('/auth/local/totp/verify', {
            method: 'POST',
            csrf: props.data.csrf,
            body: { request: result.request, code: form.values.code },
          })
          form.keepOpen = false
          notify('success', msg('toast.saved'))
          await load()
        },
      )
      form.info = `${msg('account.secret')}: ${result.secret}\n${result.uri}`
      form.keepOpen = true
    },
  )
}
onMounted(load)
</script>
<template>
  <section class="card mt-4 space-y-4">
    <p>{{ data.me?.email }}</p>
    <p>{{ msg(enabled ? 'account.enabled' : 'account.disabled') }}</p>
    <p class="text-sm text-muted-foreground">{{ msg('account.help') }}</p>
    <button v-if="password && !enabled" class="btn" @click="enroll">
      {{ msg('account.enable') }}
    </button>
  </section>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
