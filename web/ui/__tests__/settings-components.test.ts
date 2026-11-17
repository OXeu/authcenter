import { afterEach, beforeEach, expect, it, vi } from 'vite-plus/test'
import { flushPromises, mount } from '@vue/test-utils'
import SettingsPage from '../src/SettingsPage.vue'
import DropdownSelect from '../src/DropdownSelect.vue'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
import { notify } from '../src/toast.ts'
vi.mock('../src/toast.ts', () => ({ notify: vi.fn() }))
const items = [
  {
    key: 'AC_ALLOW_REGISTRATION',
    value: 'true',
    secret: false,
    configured: true,
    restart: false,
    pending: false,
    options: ['true', 'false'],
  },
  {
    key: 'AC_RESEND_API_KEY',
    value: '',
    secret: true,
    configured: true,
    restart: false,
    pending: false,
  },
]
const reply = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(async () => reply({ items })),
  )
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => vi.unstubAllGlobals())
async function settings() {
  const wrapper = mount(SettingsPage, {
    props: { page: createPage({ lang: 'en', messages: {}, csrf: 'csrf' } as PageData) },
  })
  await flushPromises()
  return wrapper
}
it('edits configuration in a dialog and preserves blank secrets during batch edits', async () => {
  const wrapper = await settings()
  expect(wrapper.find('input').exists()).toBe(false)
  expect(wrapper.text()).toContain('Configured (hidden)')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Configure')!
    .trigger('click')
  expect(wrapper.get<HTMLInputElement>('input[type=password]').element.value).toBe('')
  wrapper.findComponent(DropdownSelect).vm.$emit('update:modelValue', 'false')
  vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/settings',
    expect.objectContaining({
      method: 'PUT',
      body: JSON.stringify({ values: { AC_ALLOW_REGISTRATION: 'false' } }),
    }),
  )
  expect(wrapper.find('dialog').exists()).toBe(false)
  expect(notify).toHaveBeenCalledWith('success', 'Configuration saved. Refresh to apply.')
  wrapper.unmount()
})
it('updates a secret without fetching its previous value', async () => {
  const wrapper = await settings()
  const row = wrapper
    .findAll('tbody tr')
    .find((entry) => entry.text().includes('AC_RESEND_API_KEY'))!
  await row.get('button').trigger('click')
  await wrapper.get('input[type=password]').setValue('replacement')
  vi.mocked(fetch).mockResolvedValueOnce(new Response(null, { status: 204 }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/settings',
    expect.objectContaining({
      body: JSON.stringify({ values: { AC_RESEND_API_KEY: 'replacement' } }),
    }),
  )
  expect(wrapper.text()).not.toContain('replacement')
  wrapper.unmount()
})
it('refreshes configuration and reports required restarts through a toast', async () => {
  const wrapper = await settings()
  vi.mocked(fetch).mockResolvedValueOnce(reply({ restart_required: ['AC_DB'] }))
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Refresh config')!
    .trigger('click')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/settings/refresh',
    expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({ 'X-CSRF-Token': 'csrf' }),
    }),
  )
  expect(notify).toHaveBeenCalledWith(
    'success',
    'Configuration refreshed',
    'Restart required for: AC_DB',
  )
  expect(wrapper.text()).not.toContain('Restart required for:')
  wrapper.unmount()
})
