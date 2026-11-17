import { reactive } from 'vue'
import { api } from './core.ts'
import { createForm } from './forms.ts'
import { dangerAction } from './danger.ts'
import { notify } from './toast.ts'
import type { PageContext } from './page.ts'

type Security = {
  email: string
  password_enabled: boolean
  totp_enabled: boolean
  local_login: boolean
  email_available: boolean
}

export function createAccount(page: PageContext) {
  const { msg } = page
  const form = createForm(page)
  const state = reactive({
    ready: false,
    busy: false,
    email: '',
    password_enabled: false,
    totp_enabled: false,
    local_login: false,
    email_available: false,
  })
  async function load() {
    state.busy = true
    try {
      Object.assign(state, (await api('/api/v1/account/security')) as Security)
      state.ready = true
    } catch (error) {
      page.report(error)
    } finally {
      state.busy = false
    }
  }
  function passwordFields(key = 'password') {
    return state.password_enabled
      ? [{ key, label: 'account.currentPassword', password: true, required: true }]
      : []
  }
  async function saved() {
    form.keepOpen = false
    notify('success', msg('toast.saved'))
    await load()
  }
  function password() {
    form.show(
      msg('account.setPassword'),
      [
        ...passwordFields('current_password'),
        {
          key: 'password',
          label: 'account.newPassword',
          password: true,
          required: true,
          autocomplete: 'new-password',
        },
        {
          key: 'confirm',
          label: 'account.confirmPassword',
          password: true,
          required: true,
          autocomplete: 'new-password',
        },
      ],
      { current_password: '', password: '', confirm: '' },
      async () => {
        if (form.values.password !== form.values.confirm) {
          form.validationError = msg('account.passwordMismatch')
          form.keepOpen = true
          return
        }
        await api('/api/v1/account/password', {
          method: 'PUT',
          csrf: page.csrf,
          body: { current_password: form.values.current_password, password: form.values.password },
        })
        await saved()
      },
    )
    form.info = msg('account.passwordHelp')
  }
  function email() {
    form.show(
      msg('account.changeEmail'),
      [
        ...passwordFields(),
        { key: 'email', label: 'account.newEmail', email: true, required: true },
      ],
      { email: '', password: '' },
      async () => {
        const result = (await api('/api/v1/account/email', {
          method: 'POST',
          csrf: page.csrf,
          body: form.values,
        })) as { request: string }
        form.show(
          msg('account.verifyEmail'),
          [{ key: 'code', label: 'login.code', required: true, autocomplete: 'one-time-code' }],
          { code: '' },
          async () => {
            await api('/api/v1/account/email/verify', {
              method: 'POST',
              csrf: page.csrf,
              body: { request: result.request, code: form.values.code },
            })
            await saved()
          },
        )
        form.keepOpen = true
        notify('success', msg('login.codeSent'))
      },
    )
  }
  function enroll() {
    form.show(msg('account.enable'), passwordFields(), { password: '' }, async () => {
      const result = (await api('/auth/local/totp', {
        method: 'POST',
        csrf: page.csrf,
        body: form.values,
      })) as { request: string; secret: string; qr_code: string }
      form.show(
        msg('account.verify'),
        [{ key: 'code', label: 'login.code', required: true, autocomplete: 'one-time-code' }],
        { code: '' },
        async () => {
          await api('/auth/local/totp/verify', {
            method: 'POST',
            csrf: page.csrf,
            body: { request: result.request, code: form.values.code },
          })
          await saved()
        },
      )
      form.image = {
        src: result.qr_code,
        alt: msg('account.qrCode'),
        caption: msg('account.scan'),
      }
      form.info = `${msg('account.secret')}: ${result.secret}`
      form.keepOpen = true
    })
  }
  function actions() {
    if (!state.totp_enabled) return []
    return [
      dangerAction(
        form,
        'disableTOTP',
        msg('account.disable'),
        state.email,
        async () => {
          await api('/api/v1/account/totp', {
            method: 'DELETE',
            csrf: page.csrf,
            body: form.values,
          })
          await saved()
        },
        [...passwordFields(), { key: 'code', label: 'login.code', required: true }],
        { password: '', code: '' },
      ),
    ]
  }
  return { state, form, load, password, email, enroll, actions }
}
