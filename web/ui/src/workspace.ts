import { reactive } from 'vue'
import { api } from './core.ts'
import { createForm } from './forms.ts'
import { dangerAction } from './danger.ts'
import { notify } from './toast.ts'
import type { PageData } from './data.ts'
import type { PageContext } from './page.ts'

export type Application = {
  id: string
  name: string
  homepage: string
  redirect_uris: string[]
  public: boolean
  scopes: string[]
}
export type Grant = { id: string; client_id: string; scope: string }
export function createWorkspace(data: PageData, page: PageContext) {
  const form = createForm(page)
  const state = reactive({ rows: [] as Record<string, any>[], tab: 'clients', busy: false })
  const path = () => (data.page === 'users' ? '/api/v1/users' : `/api/v1/${state.tab}`)
  async function refresh() {
    state.busy = true
    try {
      const result = (await api(path())) as Record<string, Record<string, any>[]>
      state.rows = result[data.page === 'users' ? 'users' : state.tab] ?? []
    } catch (error) {
      page.report(error)
    } finally {
      state.busy = false
    }
  }
  async function select(tab: string) {
    if (state.busy) return
    state.tab = tab
    state.rows = []
    await refresh()
  }
  async function mutate(url: string, method: string, body?: unknown) {
    const result = (await api(url, { method, body, csrf: page.csrf })) as {
      client_secret?: string
      client?: Application
    } | null
    notify('success', page.msg('toast.saved'))
    await refresh()
    return result
  }
  function edit(row?: Record<string, any>) {
    if (data.page === 'users') {
      form.show(
        page.msg('common.edit'),
        [
          {
            key: 'role',
            label: 'dialog.role',
            options: ['user', 'admin'].map((value) => ({
              value,
              label: page.msg(`role.${value}`),
            })),
          },
        ],
        { role: row!.role },
        async () => {
          await mutate(`${path()}/${row!.id}`, 'PATCH', form.values)
        },
      )
      return
    }
    const fields = [
      { key: 'name', label: 'common.name', required: true },
      { key: 'homepage', label: 'apps.homepage', required: true },
      { key: 'callbacks', label: 'apps.callbacks', multiline: true, required: true },
      { key: 'logo', label: 'apps.logo', file: true, required: !row },
      ...(!row
        ? [
            {
              key: 'public',
              label: 'apps.type',
              options: [
                { value: 'false', label: page.msg('apps.confidential') },
                { value: 'true', label: page.msg('apps.public') },
              ],
            },
          ]
        : []),
    ]
    form.show(
      page.msg(row ? 'common.edit' : 'common.create'),
      fields,
      {
        name: row?.name ?? '',
        homepage: row?.homepage ?? '',
        callbacks: row?.redirect_uris.join('\n') ?? '',
        logo: '',
        public: String(row?.public ?? false),
      },
      async () => {
        const body = {
          name: form.values.name,
          homepage: form.values.homepage,
          redirect_uris: form.values.callbacks
            .split('\n')
            .map((value) => value.trim())
            .filter(Boolean),
          logo: form.values.logo,
          public: form.values.public === 'true',
        }
        const result = await mutate(
          row ? `${path()}/${row.id}` : path(),
          row ? 'PATCH' : 'POST',
          body,
        )
        if (result?.client)
          form.token = `${page.msg('apps.clientId')}: ${result.client.id}\n${page.msg('apps.secret')}: ${result.client_secret || page.msg('apps.noSecret')}`
      },
    )
  }
  function actionsFor(row: Record<string, any>) {
    if (data.page === 'users') {
      if (row.id === data.me?.id) return []
      const target = `${row.name} (${row.email})`
      const url = `/api/v1/users/${row.id}`
      const until = new Date(Date.now() + 86400000 - new Date().getTimezoneOffset() * 60000)
        .toISOString()
        .slice(0, 16)
      const ban = row.banned
        ? dangerAction(form, 'unbanUser', page.msg('users.unban'), target, async () => {
            await mutate(`${url}/ban`, 'DELETE')
          })
        : dangerAction(
            form,
            'banUser',
            page.msg('users.ban'),
            target,
            async () => {
              const expiry = Date.parse(form.values.until)
              if (!Number.isFinite(expiry) || expiry <= Date.now()) {
                form.validationError = page.msg('users.expiryRequired')
                form.keepOpen = true
                return
              }
              form.keepOpen = false
              await mutate(`${url}/ban`, 'PUT', { until: new Date(expiry).toISOString() })
            },
            [{ key: 'until', label: 'users.bannedUntil', datetime: true, required: true }],
            { until },
          )
      return [
        ban,
        dangerAction(form, 'deleteUser', page.msg('users.delete'), target, async () => {
          await mutate(url, 'DELETE')
        }),
      ]
    }
    const id = state.tab === 'grants' ? 'revokeGrant' : 'deleteClient'
    return [
      dangerAction(
        form,
        id,
        page.msg(id === 'revokeGrant' ? 'apps.revoke' : 'common.delete'),
        row.name ?? row.client_id,
        async () => {
          await mutate(`${path()}/${row.id}`, 'DELETE')
        },
      ),
    ]
  }
  return { state, form, refresh, select, edit, actionsFor }
}

export type APIKey = {
  id: number
  name: string
  prefix: string
  scopes: string[]
  created_by: number
  created_at: string
  last_used_at: string | null
  expires_at: string | null
}
export function createKeysWorkspace(page: PageContext) {
  const form = createForm(page)
  const state = reactive({ rows: [] as APIKey[], permissions: [] as string[], busy: false })
  async function refresh() {
    state.busy = true
    try {
      const [keys, scopes] = await Promise.all([api('/api/v1/keys'), api('/api/v1/key-scopes')])
      state.rows = (keys as { keys: APIKey[] }).keys
      state.permissions = (scopes as { permissions: string[] }).permissions
    } catch (error) {
      page.report(error)
    } finally {
      state.busy = false
    }
  }
  async function mutate(path: string, method: string, body?: unknown) {
    const result = (await api(path, { method, body, csrf: page.csrf })) as { token?: string } | null
    if (result?.token) form.token = result.token
    notify('success', page.msg('toast.saved'))
    await refresh()
  }
  function edit(row?: APIKey) {
    const fields = [
      { key: 'name', label: 'common.name', required: true },
      {
        key: 'scopes',
        label: 'keys.scopes',
        multiOptions: state.permissions.map((value) => ({
          value,
          label: page.msg(`scope.${value}`),
        })),
      },
      ...(!row ? [{ key: 'expires_at', label: 'keys.expires', datetime: true }] : []),
    ]
    form.show(
      page.msg(row ? 'common.edit' : 'dialog.key'),
      fields,
      { name: row?.name ?? '', expires_at: '' },
      async () => {
        let expires_at: string | undefined
        if (!row && form.values.expires_at) {
          const expiry = Date.parse(form.values.expires_at)
          if (!Number.isFinite(expiry) || expiry <= Date.now()) {
            form.validationError = page.msg('keys.expiryRequired')
            form.keepOpen = true
            return
          }
          expires_at = new Date(expiry).toISOString()
        }
        form.keepOpen = false
        await mutate(row ? `/api/v1/keys/${row.id}` : '/api/v1/keys', row ? 'PATCH' : 'POST', {
          name: form.values.name,
          scopes: form.choices.scopes,
          ...(!row ? { expires_at } : {}),
        })
      },
    )
    form.choices.scopes = [...(row?.scopes ?? [])]
    form.info = page.msg('keys.scopeHelp')
  }
  function actionsFor(row: APIKey) {
    return [
      dangerAction(form, 'rotateKey', page.msg('keys.rotate'), row.name, async () => {
        await mutate(`/api/v1/keys/${row.id}/rotate`, 'POST')
      }),
      dangerAction(form, 'deleteKey', page.msg('common.delete'), row.name, async () => {
        await mutate(`/api/v1/keys/${row.id}`, 'DELETE')
      }),
    ]
  }
  return { state, form, refresh, edit, actionsFor }
}
