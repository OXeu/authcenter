import { afterEach, beforeEach, expect, it, vi } from 'vite-plus/test'
import { flushPromises, mount } from '@vue/test-utils'
import { toast } from '@yusufceng1z/ding'
import ToastViewport from '../src/ToastViewport.vue'
import DialogPanel from '../src/DialogPanel.vue'
import { createForm } from '../src/forms.ts'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
import { toastTarget } from '../src/toast-host.ts'

vi.mock('@yusufceng1z/ding', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@yusufceng1z/ding')>()),
  bindToast: vi.fn(() => vi.fn()),
}))

beforeEach(() => {
  toast.configure({ sound: false })
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
afterEach(() => {
  toast.dismiss(undefined, { immediate: true })
})

it('keeps error toasts inside the modal, supports dismissal and restores the body host', async () => {
  const viewport = mount(ToastViewport)
  const page = createPage({ lang: 'en', messages: {}, csrf: '' } as PageData)
  const form = createForm(page)
  form.show('Save', [], {}, async () => {
    throw new Error('network')
  })
  const dialog = mount(DialogPanel, { props: { form, msg: page.msg }, attachTo: document.body })
  await flushPromises()
  await form.submit()
  await flushPromises()
  expect(toastTarget.value).toBe(dialog.get('dialog').element)
  expect(dialog.get('.tst-desc').text()).toBe('Network error, try again.')
  expect(dialog.find('form .tst-desc').exists()).toBe(false)
  expect(form.open).toBe(true)
  await dialog.get('[data-toast-dismiss]').trigger('click')
  expect(['closing', 'exit']).toContain(toast.getState().toasts[0].phase)
  dialog.unmount()
  await flushPromises()
  expect(toastTarget.value).toBe('body')
  expect(document.body.querySelector(':scope > .tst-viewport')).not.toBeNull()
  viewport.unmount()
  expect(document.body.querySelector('.tst-viewport')).toBeNull()
})

it('dismisses toasts with Escape without cancelling the form', async () => {
  const viewport = mount(ToastViewport)
  const page = createPage({ lang: 'en', messages: {}, csrf: '' } as PageData)
  const form = createForm(page)
  form.show('Save', [], {}, async () => {})
  const dialog = mount(DialogPanel, { props: { form, msg: page.msg }, attachTo: document.body })
  await flushPromises()
  toast.error('Failed')
  await flushPromises()
  await dialog.get('[data-toast-dismiss]').trigger('keydown', { key: 'Escape' })
  expect(form.open).toBe(true)
  expect(['closing', 'exit']).toContain(toast.getState().toasts[0].phase)
  dialog.unmount()
  await flushPromises()
  viewport.unmount()
})
