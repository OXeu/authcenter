import { afterEach, beforeEach, expect, it, vi } from 'vite-plus/test'
import { flushPromises, mount } from '@vue/test-utils'
import KeysPage from '../src/KeysPage.vue'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
import { notify } from '../src/toast.ts'
vi.mock('../src/toast.ts', () => ({ notify: vi.fn() }))
const key = {
  id: 1,
  name: 'Automation',
  prefix: 'ac_prefix…',
  scopes: ['clients:read'],
  created_by: 1,
  created_at: '2026-11-17T10:00:00Z',
  last_used_at: null,
  expires_at: null,
}
const reply = (body: unknown) => new Response(JSON.stringify(body), { status: 200 })
beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockImplementation(async (url: string) =>
        reply(
          url.endsWith('/key-scopes')
            ? { permissions: ['clients:read', 'clients:write'] }
            : { keys: [key] },
        ),
      ),
  )
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => vi.unstubAllGlobals())
async function keysPage() {
  const wrapper = mount(KeysPage, {
    props: { page: createPage({ lang: 'en', messages: {}, csrf: 'csrf' } as PageData) },
  })
  await flushPromises()
  return wrapper
}
it('creates scoped keys in the shared dialog and displays tokens only once', async () => {
  const wrapper = await keysPage()
  expect(wrapper.find('input').exists()).toBe(false)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Create')!
    .trigger('click')
  expect(wrapper.get('dialog').element.open).toBe(true)
  await wrapper.get('input:not([type])').setValue('Reader')
  await wrapper.get('[role=checkbox][value="clients:read"]').trigger('click')
  vi.mocked(fetch).mockResolvedValueOnce(reply({ key, token: 'ac_one-time-secret' }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/keys',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ name: 'Reader', scopes: ['clients:read'] }),
      headers: expect.objectContaining({ 'X-CSRF-Token': 'csrf' }),
    }),
  )
  expect(wrapper.get('dialog').text()).toContain('ac_one-time-secret')
  expect(wrapper.get('table').text()).not.toContain('ac_one-time-secret')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Cancel')!
    .trigger('click')
  expect(wrapper.find('dialog').exists()).toBe(false)
  expect(wrapper.text()).not.toContain('ac_one-time-secret')
  wrapper.unmount()
})
it('edits scopes, including an empty permission set', async () => {
  const wrapper = await keysPage()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Edit')!
    .trigger('click')
  const scope = wrapper.get('[role=checkbox][value="clients:read"]')
  expect(scope.attributes('aria-checked')).toBe('true')
  await scope.trigger('click')
  vi.mocked(fetch).mockResolvedValueOnce(reply({ key: { ...key, scopes: [] } }))
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/keys/1',
    expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ name: 'Automation', scopes: [] }),
    }),
  )
  expect(notify).toHaveBeenCalledWith('success', 'Saved')
  wrapper.unmount()
})
it('defers rotation to confirmation and prevents cancellation while submitting', async () => {
  const wrapper = await keysPage()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'More')!
    .trigger('click')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Rotate')!
    .trigger('click')
  expect(fetch).not.toHaveBeenCalledWith('/api/v1/keys/1/rotate', expect.anything())
  let resolve!: (value: Response) => void
  vi.mocked(fetch).mockImplementationOnce(
    () =>
      new Promise<Response>((done) => {
        resolve = done
      }),
  )
  await wrapper.get('form').trigger('submit')
  const cancel = new Event('cancel', { cancelable: true })
  wrapper.get('dialog').element.dispatchEvent(cancel)
  expect(cancel.defaultPrevented).toBe(true)
  resolve(reply({ token: 'ac_rotated-secret' }))
  await flushPromises()
  expect(wrapper.get('dialog').text()).toContain('ac_rotated-secret')
  wrapper.unmount()
})
it('reports submission failure through a toast and keeps the form open', async () => {
  const wrapper = await keysPage()
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Edit')!
    .trigger('click')
  vi.mocked(fetch).mockResolvedValueOnce(
    new Response(JSON.stringify({ error: 'insufficient_scope' }), { status: 403 }),
  )
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  expect(notify).toHaveBeenCalledWith('error', expect.any(String), expect.any(String))
  expect(wrapper.get('dialog').text()).not.toContain('insufficient_scope')
  expect(wrapper.get('dialog').element.open).toBe(true)
  wrapper.unmount()
})
