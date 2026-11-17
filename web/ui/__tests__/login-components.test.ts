import { afterEach, beforeEach, expect, it, vi } from 'vite-plus/test'
import { flushPromises, mount } from '@vue/test-utils'
import LoginPanel from '../src/LoginPanel.vue'
import AccountPage from '../src/AccountPage.vue'
import ConsentPage from '../src/ConsentPage.vue'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
vi.mock('../src/toast.ts', () => ({ notify: vi.fn() }))
const data = {
  page: 'login',
  lang: 'en',
  messages: {},
  csrf: 'csrf',
  localLogin: true,
  emailLogin: true,
  registration: true,
  oauthEnabled: true,
} as PageData
const reply = (body: unknown) => ({ ok: true, status: 200, json: async () => body }) as Response
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn())
  vi.stubGlobal('location', { assign: vi.fn() })
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => vi.unstubAllGlobals())
it('keeps password and verification inputs in the shared dialog until both factors succeed', async () => {
  vi.mocked(fetch)
    .mockResolvedValueOnce(reply({ methods: ['totp', 'email'] }))
    .mockResolvedValueOnce(reply({ redirect: '/oauth/authorize?request=resume' }))
  const wrapper = mount(LoginPanel, { props: { data, page: createPage(data) } })
  expect(wrapper.get('a').attributes('href')).toBe('/auth/login')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Password login')!
    .trigger('click')
  await wrapper.get('dialog input[type="email"]').setValue('alice@example.com')
  await wrapper.get('dialog input[type="password"]').setValue('correct horse battery')
  await wrapper.get('dialog form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('dialog').element.open).toBe(true)
  expect(wrapper.get('dialog').text()).toContain('Two-step verification')
  expect(wrapper.get('dialog').text()).toContain('Send email code')
  expect(location.assign).not.toHaveBeenCalled()
  await wrapper.get('dialog input').setValue('123456')
  await wrapper.get('dialog form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenLastCalledWith(
    '/auth/local/verify',
    expect.objectContaining({ body: JSON.stringify({ method: 'totp', code: '123456' }) }),
  )
  expect(location.assign).toHaveBeenCalledWith('/oauth/authorize?request=resume')
  wrapper.unmount()
})
it('shows the authenticator secret only inside the enrollment dialog and verifies before enabling', async () => {
  vi.mocked(fetch)
    .mockResolvedValueOnce(reply({ totp_enabled: false, password_enabled: true }))
    .mockResolvedValueOnce(
      reply({ request: 'request', secret: 'SECRET', uri: 'otpauth://totp/example' }),
    )
    .mockResolvedValueOnce({ ok: true, status: 204 } as Response)
    .mockResolvedValueOnce(reply({ totp_enabled: true, password_enabled: true }))
  const wrapper = mount(AccountPage, { props: { data, page: createPage(data) } })
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await wrapper.get('input[type="password"]').setValue('correct horse battery')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('dialog').text()).toContain('SECRET')
  await wrapper.get('input').setValue('123456')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.find('dialog').exists()).toBe(false)
  expect(wrapper.text()).toContain('Authenticator enabled')
  wrapper.unmount()
})
it('submits consent once and follows the validated server callback', async () => {
  let finish!: (response: Response) => void
  vi.mocked(fetch).mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  const consent = {
    ...data,
    authorization: {
      id: 'request',
      client: {
        id: 'app',
        name: 'Example',
        homepage: 'https://app.example',
        redirect_uris: [],
        scopes: [],
        public: false,
      },
      scopes: ['profile'],
      redirect_uri: 'https://app.example/callback',
    },
  }
  const wrapper = mount(ConsentPage, { props: { data: consent, page: createPage(consent) } })
  const approve = wrapper.findAll('button')[1]
  await approve.trigger('click')
  await approve.trigger('click')
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(approve.attributes('disabled')).toBeDefined()
  finish(reply({ redirect: 'https://app.example/callback?code=code' }))
  await flushPromises()
  expect(location.assign).toHaveBeenCalledWith('https://app.example/callback?code=code')
  wrapper.unmount()
})
