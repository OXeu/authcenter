import { describe, it, expect, vi, beforeEach } from 'vite-plus/test'
import { createWorkspace } from '../src/workspace.js'
import { createPage } from '../src/page.js'
import { api } from '../src/core.js'
vi.mock('../src/toast.js', () => ({ notify: vi.fn() }))
const data = {
  page: 'project',
  project: { slug: 'billing' },
  env: 'production',
  csrf: 'csrf',
  lang: 'en',
  messages: {},
}
const fetchMock = vi.fn()
beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock)
  fetchMock.mockReset().mockResolvedValue({
    ok: true,
    status: 200,
    json: async () => ({ items: [], keys: [], proposals: [], environments: [] }),
  })
})
describe('workspace mutations', () => {
  it('encodes the original item key and sends edited values with CSRF', async () => {
    const w = createWorkspace(data, createPage(data))
    w.edit({ key: 'old.key', value: 'one', description: '' })
    w.form.values.key = 'new.key'
    w.form.values.value = 'two'
    await w.form.submit()
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/api/v1/projects/billing/environments/production/items/old.key',
    )
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: 'PUT',
      headers: { 'X-CSRF-Token': 'csrf' },
      body: JSON.stringify({ key: 'new.key', value: 'two', description: '' }),
    })
    expect(w.form.open).toBe(false)
  })
  it('keeps a newly issued token visible until the dialog closes', async () => {
    const w = createWorkspace(data, createPage(data))
    w.state.tab = 'keys'
    w.edit()
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({ token: 'cc_once' }),
    })
    await w.form.submit()
    expect(w.form.token).toBe('cc_once')
    expect(w.form.open).toBe(true)
  })
  it('keeps failed edits open with a translated error', async () => {
    const w = createWorkspace(data, createPage(data))
    w.edit()
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: async () => ({ error: 'duplicate' }),
    })
    await w.form.submit()
    expect(w.form.open).toBe(true)
    expect(w.form.error).toBe('That name is already taken.')
  })
  it('requires confirmation before deleting or reviewing a proposal', async () => {
    const w = createWorkspace(data, createPage(data))
    w.state.tab = 'proposals'
    w.review({ id: 7, key: 'a' }, false)
    expect(fetchMock).not.toHaveBeenCalled()
    await w.form.submit()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/projects/billing/proposals/7/review')
    expect(fetchMock.mock.calls[0][1].body).toBe('{"approve":false}')
  })
  it('changes environment before requesting its items', async () => {
    const w = createWorkspace(data, createPage(data))
    w.environment()
    w.form.values.env = 'staging'
    await w.form.submit()
    expect(fetchMock.mock.calls[1][0]).toContain('/environments/staging/items')
  })
  it('updates user roles through the admin API', async () => {
    const w = createWorkspace({ ...data, page: 'users' }, createPage(data))
    w.edit({ id: 9, role: 'viewer' })
    w.form.values.role = 'editor'
    await w.form.submit()
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/users/9')
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'PATCH', body: '{"role":"editor"}' })
  })
  it('accepts empty successful deletion responses', async () => {
    fetchMock.mockResolvedValueOnce({ ok: true, status: 204 })
    expect(await api('/item', { method: 'DELETE' })).toBeNull()
  })
})

it('creates, updates and deletes projects through their own routes', async () => {
  const w = createWorkspace({ ...data, page: 'projects' }, createPage(data))
  w.edit()
  Object.assign(w.form.values, { slug: 'new', name: 'New', description: '' })
  await w.form.submit()
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/projects')
  expect(fetchMock.mock.calls[0][1].method).toBe('POST')
  fetchMock.mockClear()
  w.edit({ slug: 'new', name: 'New', description: '' })
  w.form.values.name = 'Renamed'
  await w.form.submit()
  expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/projects/new')
  expect(fetchMock.mock.calls[0][1].method).toBe('PATCH')
  fetchMock.mockClear()
  w.remove({ slug: 'new', name: 'Renamed' })
  expect(fetchMock).not.toHaveBeenCalled()
  await w.form.submit()
  expect(fetchMock.mock.calls[0][1].method).toBe('DELETE')
})

it('reports network failures without discarding form values', async () => {
  const w = createWorkspace(data, createPage(data))
  w.edit()
  w.form.values.key = 'keep'
  fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
  await w.form.submit()
  expect(w.form.values.key).toBe('keep')
  expect(w.form.error).toBe('Network error, try again.')
  expect(w.form.busy).toBe(false)
})
