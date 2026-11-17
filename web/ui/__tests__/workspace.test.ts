import { beforeEach, expect, it, vi } from 'vite-plus/test'
import { createWorkspace } from '../src/workspace.ts'
import { createPage } from '../src/page.ts'
import type { PageData } from '../src/data.ts'
import { notify } from '../src/toast.ts'
vi.mock('../src/toast.ts', () => ({ notify: vi.fn() }))
const data = { page: 'applications', lang: 'en', messages: {}, csrf: 'csrf' } as PageData
beforeEach(() =>
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ clients: [] }) }),
  ),
)
it('creates applications with multiple callbacks and preserves the one-time secret', async () => {
  vi.mocked(fetch).mockResolvedValueOnce({
    ok: true,
    status: 201,
    json: async () => ({ client: { id: 'client' }, client_secret: 'one-time' }),
  } as Response)
  const w = createWorkspace(data, createPage(data))
  w.edit()
  Object.assign(w.form.values, {
    name: 'App',
    homepage: 'https://app.example',
    callbacks: 'https://app.example/callback\nhttp://localhost:3000/callback',
    logo: 'base64',
  })
  await w.form.submit()
  const init = vi.mocked(fetch).mock.calls[0][1]!
  expect(JSON.parse(init.body as string).redirect_uris).toHaveLength(2)
  expect(init.headers).toMatchObject({ 'X-CSRF-Token': 'csrf' })
  expect(w.form.token).toContain('one-time')
  expect(w.form.open).toBe(true)
})
it('edits application metadata without replacing its credentials', async () => {
  const w = createWorkspace(data, createPage(data))
  w.edit({
    id: 'app',
    name: 'Name',
    homepage: 'https://app.example',
    redirect_uris: ['https://app.example/cb'],
    public: false,
  })
  w.form.values.name = 'Renamed'
  await w.form.submit()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/clients/app',
    expect.objectContaining({ method: 'PATCH' }),
  )
  expect(w.form.open).toBe(false)
  expect(notify).toHaveBeenCalledWith('success', 'Saved')
})
it('defers deletion until confirmation and preserves failed forms', async () => {
  const w = createWorkspace(data, createPage(data))
  w.actionsFor({ id: 'app', name: 'Name' })[0].request()
  expect(fetch).not.toHaveBeenCalled()
  vi.mocked(fetch).mockRejectedValueOnce(new Error('network'))
  await w.form.submit()
  expect(w.form.open).toBe(true)
  expect(notify).toHaveBeenCalledWith('error', 'Something went wrong', 'Network error, try again.')
  expect(w.form.validationError).toBe('')
  expect(w.form.busy).toBe(false)
})
it('revokes grants through the scoped grants endpoint', async () => {
  const w = createWorkspace(data, createPage(data))
  await w.select('grants')
  w.actionsFor({ id: 'grant', client_id: 'app' })[0].request()
  await w.form.submit()
  expect(fetch).toHaveBeenCalledWith(
    '/api/v1/grants/grant',
    expect.objectContaining({ method: 'DELETE' }),
  )
})
