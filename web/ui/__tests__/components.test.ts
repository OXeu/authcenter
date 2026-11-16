import type { PageData } from '../src/data.ts'
import { describe, it, expect, vi, beforeEach } from 'vite-plus/test'
import { mount, flushPromises } from '@vue/test-utils'
import App from '../src/App.vue'
import DialogPanel from '../src/DialogPanel.vue'
import { createForm } from '../src/forms.ts'
import { createPage } from '../src/page.ts'
import { notify, mountToasts, setToastTheme } from '../src/toast.ts'
vi.mock('../src/toast.ts', () => ({
  notify: vi.fn(),
  mountToasts: vi.fn(),
  setToastTheme: vi.fn(),
}))
const data: PageData = {
  publicUrl: '',
  providerName: '',
  nonce: '',
  oauthEnabled: false,
  flash: null,
  env: '',
  project: null,
  page: 'projects',
  title: 'title.projects',
  lang: 'en',
  messages: { 'title.projects': 'Projects' },
  loggedIn: true,
  me: { id: 1, email: '', name: 'Admin', role: 'admin' },
  isAdmin: true,
  canWrite: true,
  languages: [
    { tag: 'en', label: 'English' },
    { tag: 'zh-CN', label: '中文' },
  ],
  languageLabel: 'English',
  csrf: 'csrf',
}
beforeEach(() => {
  localStorage.clear()
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        projects: [{ slug: 'billing', name: 'Billing', description: '<script>bad</script>' }],
      }),
    }),
  )
  HTMLDialogElement.prototype.showModal = function () {
    this.open = true
  }
  HTMLDialogElement.prototype.close = function () {
    this.open = false
  }
})
describe('Vue pages', () => {
  it('renders projects safely and opens the shared creation dialog', async () => {
    const wrapper = mount(App, { props: { data } })
    await flushPromises()
    expect(wrapper.get('a[href="/projects/billing"]').text()).toBe('Billing')
    expect(wrapper.find('script').exists()).toBe(false)
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Create')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get('dialog').element.open).toBe(true)
    expect(wrapper.findAll('dialog input')).toHaveLength(3)
    expect(wrapper.findAll('main input')).toHaveLength(0)
    wrapper.unmount()
  })
  it('requires More and confirmation before deleting, and allows cancellation', async () => {
    const wrapper = mount(App, { props: { data } })
    await flushPromises()
    const calls = vi.mocked(fetch).mock.calls.length
    expect(wrapper.find('[data-danger-action]').exists()).toBe(false)
    const more = wrapper.findAll('button').find((button) => button.text().startsWith('More'))!
    await more.trigger('click')
    await more.trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[data-danger-action]').exists()).toBe(false)
    await more.trigger('click')
    const action = wrapper.get('[data-danger-action="deleteProject"]')
    expect(action.classes()).toContain('btn-danger')
    await action.trigger('click')
    await flushPromises()
    expect(vi.mocked(fetch).mock.calls).toHaveLength(calls)
    expect(wrapper.get('dialog').element.open).toBe(true)
    expect(wrapper.get('dialog button[type="submit"]').classes()).toContain('btn-danger')
    await wrapper.get('dialog button[type="button"]').trigger('click')
    expect(vi.mocked(fetch).mock.calls).toHaveLength(calls)
    await more.trigger('click')
    await wrapper.get('[data-danger-action="deleteProject"]').trigger('click')
    await wrapper.get('dialog form').trigger('submit')
    await flushPromises()
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/projects/billing',
      expect.objectContaining({ method: 'DELETE' }),
    )
    wrapper.unmount()
  })
  it('hides writer actions for viewers and preserves language links', async () => {
    const wrapper = mount(App, {
      props: {
        data: {
          ...data,
          canWrite: false,
          isAdmin: false,
          me: { id: 1, email: '', name: 'Viewer', role: 'viewer' },
        },
      },
    })
    await flushPromises()
    expect(wrapper.findAll('button').some((b) => b.text() === 'Create')).toBe(false)
    expect(wrapper.find('a[href="/users"]').exists()).toBe(false)
    await wrapper.get('button[aria-controls]').trigger('click')
    expect(wrapper.find('a[href="/language?lang=zh-CN"]').exists()).toBe(true)
    wrapper.unmount()
  })
  it('reports a disabled login using a toast', async () => {
    const wrapper = mount(App, {
      props: { data: { ...data, page: 'login', loggedIn: false, oauthEnabled: false } },
    })
    await flushPromises()
    expect(notify).toHaveBeenCalledWith('info', 'OAuth is not configured on this server.')
    expect(wrapper.find('a[href="/auth/login"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('switches page, dialog and toast themes from the header and restores the choice', async () => {
    localStorage.setItem('cc_theme', 'dark')
    const wrapper = mount(App, { props: { data } })
    await flushPromises()
    expect(wrapper.attributes('data-theme')).toBe('dark')
    expect(mountToasts).toHaveBeenLastCalledWith('en', '', 'dark')
    await wrapper.get('button[aria-label="Light mode"]').trigger('click')
    expect(wrapper.attributes('data-theme')).toBe('light')
    expect(setToastTheme).toHaveBeenLastCalledWith('light')
    expect(localStorage.getItem('cc_theme')).toBe('light')
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Create')!
      .trigger('click')
    await flushPromises()
    expect(wrapper.get('dialog').element.closest('[data-theme]')!.getAttribute('data-theme')).toBe(
      'light',
    )
    expect(wrapper.find('button[aria-label="Dark mode"]').exists()).toBe(true)
    wrapper.unmount()
    const restored = mount(App, { props: { data } })
    expect(restored.attributes('data-theme')).toBe('light')
    restored.unmount()
  })
  it('renders the OAuth avatar with an icon medal and falls back after an image error', async () => {
    const wrapper = mount(App, {
      props: {
        data: { ...data, me: { ...data.me!, avatar_url: 'https://images.example.com/avatar.png' } },
      },
    })
    await flushPromises()
    const avatar = wrapper.get('.user-avatar')
    expect(avatar.attributes('aria-label')).toBe('Admin Admin')
    expect(avatar.get('img').attributes('src')).toBe('https://images.example.com/avatar.png')
    expect(avatar.get('img').attributes('referrerpolicy')).toBe('no-referrer')
    expect(avatar.get('.avatar-role').text()).toBe('')
    expect(avatar.find('.avatar-role .ri-vip-crown-line').exists()).toBe(true)
    await avatar.get('img').trigger('error')
    expect(avatar.find('img').exists()).toBe(false)
    expect(avatar.text()).toBe('A')
    wrapper.unmount()
  })
  it.each([
    ['editor', 'ri-edit-line'],
    ['viewer', 'ri-eye-line'],
  ])('shows the %s medal', async (role, icon) => {
    const wrapper = mount(App, { props: { data: { ...data, me: { ...data.me!, role } } } })
    await flushPromises()
    expect(wrapper.find(`.avatar-role .${icon}`).exists()).toBe(true)
    wrapper.unmount()
  })
  it('blocks repeated submit and escape while saving', async () => {
    const form = createForm(createPage(data))
    let finish!: () => void
    const action = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve
        }),
    )
    const wrapper = mount(DialogPanel, { props: { form, msg: (id: string) => id } })
    form.show('Save', [], {}, action)
    await flushPromises()
    const pending = form.submit()
    await form.submit()
    await wrapper.get('dialog').trigger('cancel')
    expect(form.open).toBe(true)
    expect(action).toHaveBeenCalledTimes(1)
    finish()
    await pending
    await flushPromises()
    expect(wrapper.get('dialog').element.open).toBe(false)
    wrapper.unmount()
  })
})
