import { beforeEach, afterEach, expect, it, vi } from 'vite-plus/test'
import { mount, flushPromises } from '@vue/test-utils'
import AccountPage from '../src/AccountPage.vue'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
import { notify } from '../src/toast.ts'
vi.mock('../src/toast.ts', () => ({ notify: vi.fn() }))
const data = { lang: 'en', csrf: 'csrf', messages: {} } as PageData
const security = {
  email: 'old@example.com',
  password_enabled: false,
  totp_enabled: false,
  local_login: true,
  email_available: true,
}
const reply = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(async () => reply(security)),
  )
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => vi.unstubAllGlobals())
async function account() {
  const wrapper = mount(AccountPage, { props: { data, page: createPage(data) } })
  await flushPromises()
  return wrapper
}
it('lets an OAuth user set a password and validates confirmation before sending', async () => {
  const wrapper = await account()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Set password')!
    .trigger('click')
  const inputs = wrapper.findAll('dialog input')
  expect(inputs).toHaveLength(2)
  await inputs[0].setValue('a new secure password')
  await inputs[1].setValue('different')
  await wrapper.get('form').trigger('submit')
  expect(fetch).toHaveBeenCalledTimes(1)
  await inputs[1].setValue('a new secure password')
  vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/password',
    expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ current_password: '', password: 'a new secure password' }),
    }),
  )
  expect(wrapper.find('dialog').exists()).toBe(false)
  expect(notify).toHaveBeenCalledWith('success', 'Saved')
  wrapper.unmount()
})
it('verifies the new email before completing the change', async () => {
  const wrapper = await account()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Change email')!
    .trigger('click')
  await wrapper.get('input[type=email]').setValue('new@example.com')
  vi.mocked(fetch).mockResolvedValueOnce(reply({ request: 'email-request' }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('dialog').text()).toContain('Verify new email')
  await wrapper.get('dialog input').setValue('123456')
  vi.mocked(fetch)
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(reply({ ...security, email: 'new@example.com' }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/email/verify',
    expect.objectContaining({ body: JSON.stringify({ request: 'email-request', code: '123456' }) }),
  )
  expect(wrapper.text()).toContain('new@example.com')
  expect(wrapper.find('dialog').exists()).toBe(false)
  wrapper.unmount()
})
it('allows authenticator enrollment without a local password', async () => {
  const wrapper = await account()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Set up authenticator')!
    .trigger('click')
  expect(wrapper.find('dialog input').exists()).toBe(false)
  vi.mocked(fetch).mockResolvedValueOnce(
    reply({
      request: 'totp-request',
      secret: 'SECRET',
      uri: 'otpauth://example',
      qr_code: 'data:image/png;base64,cXI=',
    }),
  )
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('dialog').text()).toContain('SECRET')
  expect(wrapper.get('dialog img').attributes('src')).toBe('data:image/png;base64,cXI=')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Cancel')!
    .trigger('click')
  expect(wrapper.find('img').exists()).toBe(false)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Set password')!
    .trigger('click')
  expect(wrapper.find('dialog img').exists()).toBe(false)
  expect(wrapper.get('dialog').text()).not.toContain('SECRET')
  wrapper.unmount()
})
