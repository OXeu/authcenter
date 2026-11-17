<script setup lang="ts">
import Button from '@oxeu/center-common/ui/Button.vue'
import { navigateDocument } from '@oxeu/center-common/ui/navigation.ts'
import DialogPanel from './DialogPanel.vue'
import { createForm } from './forms.ts'
import { notify } from './toast.ts'
import { api } from './core.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'
const props = defineProps<{ data: PageData; page: PageContext }>()
const { msg } = props.page
const form = createForm(props.page)
async function sendEmail() {
  try {
    await api('/auth/local/email', { method: 'POST', csrf: props.data.csrf })
    notify('success', msg('login.codeSent'))
  } catch (error) {
    props.page.report(error)
  }
}
function secondFactor(methods: string[]) {
  form.show(
    msg('login.verify'),
    [
      {
        key: 'method',
        label: 'login.method',
        options: methods.map((value) => ({ value, label: msg(`login.${value}`) })),
      },
      { key: 'code', label: 'login.code', required: true, autocomplete: 'one-time-code' },
    ],
    { method: methods[0], code: '' },
    async () => {
      const result = (await api('/auth/local/verify', {
        method: 'POST',
        body: form.values,
        csrf: props.data.csrf,
      })) as { redirect: string }
      form.keepOpen = false
      navigateDocument(result.redirect, 'authentication')
    },
  )
  form.keepOpen = true
  form.info = msg('login.verifyHelp')
  if (methods.includes('email')) {
    form.secondaryLabel = msg('login.sendEmail')
    form.secondaryAction = sendEmail
  }
}
function open(register: boolean) {
  const fields = [
    ...(register ? [{ key: 'name', label: 'common.name', required: true }] : []),
    { key: 'email', label: 'users.email', required: true, email: true },
    { key: 'password', label: 'login.password', required: true, password: true },
  ]
  form.show(
    msg(register ? 'login.register' : 'login.passwordLogin'),
    fields,
    { email: '', password: '', name: '' },
    async () => {
      const result = (await api('/auth/local/password', {
        method: 'POST',
        body: { ...form.values, register },
        csrf: props.data.csrf,
      })) as { methods: string[] }
      secondFactor(result.methods)
    },
  )
  form.info = msg('login.passwordHelp')
}
</script>
<template>
  <div class="mt-4 flex flex-wrap gap-2">
    <Button v-if="data.oauthEnabled" href="/auth/login" variant="primary">{{
      msg('login.github')
    }}</Button>
    <template v-if="data.localLogin">
      <Button @click="open(false)" variant="secondary">{{ msg('login.passwordLogin') }}</Button>
      <Button v-if="data.registration && data.emailLogin" @click="open(true)" variant="ghost">
        {{ msg('login.register') }}
      </Button>
    </template>
  </div>
  <DialogPanel v-if="form.open" :form="form" :msg="msg" />
</template>
