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
  emails: [{ id: 1, email: 'old@example.com', primary: true, verified: true }],
  password_enabled: false,
  totp_enabled: false,
  totp_devices: [],
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
it('verifies the new email before binding it without changing the primary', async () => {
  const wrapper = await account()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Add email')!
    .trigger('click')
  await wrapper.get('input[type=email]').setValue('new@example.com')
  vi.mocked(fetch).mockResolvedValueOnce(reply({ request: 'email-request' }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(wrapper.get('dialog').text()).toContain('Verify new email')
  await wrapper.get('dialog input').setValue('123456')
  vi.mocked(fetch)
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(
      reply({
        ...security,
        emails: [
          ...security.emails,
          { id: 2, email: 'new@example.com', primary: false, verified: true },
        ],
      }),
    )
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/emails/verify',
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
  await wrapper.get('dialog input').setValue('Phone')
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
it('adds another authenticator and removes only the selected device after confirmation', async () => {
  const devices = [
    { id: 1, name: 'Phone', created_at: '' },
    { id: 2, name: 'Tablet', created_at: '' },
  ]
  vi.mocked(fetch).mockResolvedValue(
    reply({ ...security, totp_enabled: true, totp_devices: devices }),
  )
  const wrapper = await account()
  expect(wrapper.text()).toContain('Phone')
  expect(wrapper.text()).toContain('Tablet')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Set up authenticator')!
    .trigger('click')
  await wrapper.get('dialog input').setValue('Backup')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Cancel')!
    .trigger('click')
  const tablet = wrapper.findAll('li').find((row) => row.text().includes('Tablet'))!
  await tablet
    .findAll('button')
    .find((button) => button.text() === 'More')!
    .trigger('click')
  await tablet
    .findAll('button')
    .find((button) => button.text() === 'Remove device')!
    .trigger('click')
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(wrapper.get('dialog').text()).toContain('Tablet')
  await wrapper.get('dialog input').setValue('123456')
  vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/totp',
    expect.objectContaining({
      method: 'DELETE',
      body: JSON.stringify({ password: '', code: '123456', id: 2 }),
    }),
  )
  wrapper.unmount()
})
it('selects a verified primary email in a dialog and protects the current primary from removal', async () => {
  const emails = [
    ...security.emails,
    { id: 2, email: 'second@example.com', primary: false, verified: true },
  ]
  vi.mocked(fetch).mockResolvedValue(reply({ ...security, password_enabled: true, emails }))
  const wrapper = await account()
  const original = wrapper.findAll('li').find((row) => row.text().includes('old@example.com'))!
  expect(original.find('button').exists()).toBe(false)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Make primary')!
    .trigger('click')
  expect(wrapper.get('dialog').text()).toContain('second@example.com')
  await wrapper.get('input[type=password]').setValue('current password')
  vi.mocked(fetch)
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(
      reply({
        ...security,
        emails: emails.map((address) => ({ ...address, primary: address.id === 2 })),
      }),
    )
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/emails/2/primary',
    expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ password: 'current password' }),
    }),
  )
  expect(
    wrapper
      .findAll('li')
      .find((row) => row.text().includes('second@example.com'))!
      .text(),
  ).toContain('Primary email')
  wrapper.unmount()
})
it('defers removing an email until its confirmation dialog is submitted', async () => {
  vi.mocked(fetch).mockResolvedValue(
    reply({
      ...security,
      emails: [
        ...security.emails,
        { id: 2, email: 'second@example.com', primary: false, verified: true },
      ],
    }),
  )
  const wrapper = await account()
  const row = wrapper.findAll('li').find((item) => item.text().includes('second@example.com'))!
  await row
    .findAll('button')
    .find((button) => button.text() === 'More')!
    .trigger('click')
  await row
    .findAll('button')
    .find((button) => button.text() === 'Remove email')!
    .trigger('click')
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(wrapper.get('dialog').text()).toContain('second@example.com')
  vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/emails/2',
    expect.objectContaining({ method: 'DELETE' }),
  )
  wrapper.unmount()
})
it('renames the selected authenticator in a prefilled dialog and refreshes its name', async () => {
  const devices = [
    { id: 1, name: 'Phone', created_at: '' },
    { id: 2, name: 'Tablet', created_at: '' },
  ]
  vi.mocked(fetch).mockResolvedValue(
    reply({ ...security, totp_enabled: true, totp_devices: devices }),
  )
  const wrapper = await account()
  const row = wrapper.findAll('li').find((item) => item.text().includes('Tablet'))!
  const rename = row.findAll('button').find((button) => button.text() === 'Rename device')!
  await rename.trigger('click')
  expect((wrapper.get('dialog input').element as HTMLInputElement).value).toBe('Tablet')
  await wrapper.get('dialog input').setValue('Cancelled name')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Cancel')!
    .trigger('click')
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(row.text()).toContain('Tablet')
  await rename.trigger('click')
  await wrapper.get('dialog input').setValue('Backup tablet')
  vi.mocked(fetch)
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(
      reply({
        ...security,
        totp_enabled: true,
        totp_devices: [devices[0], { ...devices[1], name: 'Backup tablet' }],
      }),
    )
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/account/totp/2',
    expect.objectContaining({ method: 'PATCH', body: JSON.stringify({ name: 'Backup tablet' }) }),
  )
  expect(wrapper.find('dialog').exists()).toBe(false)
  expect(wrapper.text()).toContain('Backup tablet')
  expect(wrapper.text()).toContain('Phone')
  expect(notify).toHaveBeenCalledWith('success', 'Saved')
  wrapper.unmount()
})
