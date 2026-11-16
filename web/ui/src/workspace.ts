import { dangerAction, dangerOperations } from './danger.ts'
import { reactive } from 'vue'
import { api } from './core.ts'
import { createForm } from './forms.ts'
import { notify } from './toast.ts'

/** @param {string} value */
const enc = (value: string) => encodeURIComponent(value)
/** @param {string} key @param {string} label @param {boolean} [required] */
const field = (key: string, label: string, required: boolean = true) => ({ key, label, required })

/** @param {import('./data.ts').PageData} data @param {import('./page.ts').PageContext} page */
export function createWorkspace(
  data: import('./data.ts').PageData,
  page: import('./page.ts').PageContext,
) {
  const form = createForm(page)
  const state = reactive({
    rows: /** @type {any[]} */ ([]) as any[],
    environments: /** @type {any[]} */ ([]) as any[],
    tab: 'items',
    env: data.env,
    busy: false,
  })
  const base = '/api/v1/projects/' + encodeURIComponent(data.project?.slug ?? '')
  const path = () =>
    data.page === 'projects'
      ? '/api/v1/projects'
      : data.page === 'users'
        ? '/api/v1/users'
        : state.tab === 'items'
          ? `${base}/environments/${enc(state.env)}/items`
          : `${base}/${state.tab}`
  /** @returns {Promise<void>} */
  async function load(): Promise<void> {
    state.busy = true
    try {
      if (data.page === 'project')
        state.environments = (await api(`${base}/environments`)).environments
      const result = await api(path() + (state.tab === 'proposals' ? '?status=pending' : ''))
      state.rows = result[data.page === 'project' ? state.tab : data.page] ?? []
    } finally {
      state.busy = false
    }
  }
  /** @returns {Promise<void>} */
  async function refresh(): Promise<void> {
    try {
      await load()
    } catch (err) {
      page.report(err)
    }
  }
  /** @param {string} tab */
  async function select(tab: string) {
    if (state.busy) return
    state.tab = tab
    state.rows = []
    await refresh()
  }
  const roles = () => ({
    key: 'role',
    label: 'dialog.role',
    options: ['viewer', 'editor', 'admin'].map((value) => ({
      value,
      label: page.msg(`role.${value}`),
    })),
  })
  /** @param {string} url @param {string} method @param {unknown} [body] */
  async function mutate(url: string, method: string, body?: unknown) {
    const result = await api(url, { method, body, csrf: page.csrf })
    notify('success', page.msg('toast.saved'))
    await refresh()
    return result
  }
  /** @param {any} [row] */
  function edit(row?: any) {
    const editing = Boolean(row)
    let fields,
      values,
      url = path(),
      method = 'POST'
    if (data.page === 'projects') {
      fields = [
        ...(editing ? [] : [field('slug', 'common.slug')]),
        field('name', 'common.name'),
        field('description', 'common.description', false),
      ]
      values = row ?? { slug: '', name: '', description: '' }
      if (editing) {
        url += '/' + enc(row.slug)
        method = 'PATCH'
      }
    } else if (data.page === 'users') {
      fields = [roles()]
      values = { role: row.role }
      url += '/' + row.id
      method = 'PATCH'
    } else if (state.tab === 'keys') {
      fields = [field('name', 'dialog.keyName'), roles()]
      values = { name: '', role: 'viewer' }
    } else {
      fields = [
        field('key', 'dialog.itemKey'),
        { ...field('value', 'dialog.itemValue', false), multiline: true },
        field('description', 'common.description', false),
      ]
      values = row ?? { key: '', value: '', description: '' }
      method = 'PUT'
    }
    form.show(page.msg(editing ? 'common.edit' : 'common.create'), fields, values, async () => {
      const target = method === 'PUT' ? url + '/' + enc(row?.key ?? form.values.key) : url
      const result = await mutate(target, method, form.values)
      if (result?.token) form.token = result.token
    })
  }
  function removal(row: any) {
    const id =
      data.page === 'projects' ? 'deleteProject' : state.tab === 'keys' ? 'revokeKey' : 'deleteItem'
    const url = path() + '/' + enc(row.slug ?? row.key ?? String(row.id))
    return dangerAction(
      form,
      id,
      page.msg(dangerOperations[id]),
      row.name ?? row.key ?? row.slug,
      async () => {
        await mutate(url, 'DELETE')
      },
    )
  }
  function proposal(row: any, approve: boolean) {
    const id = approve ? 'approveProposal' : 'rejectProposal'
    return dangerAction(form, id, page.msg(dangerOperations[id]), row.key, async () => {
      await mutate(`${base}/proposals/${row.id}/review`, 'POST', { approve })
    })
  }
  function remove(row: any) {
    removal(row).request()
  }
  function review(row: any, approve: boolean) {
    proposal(row, approve).request()
  }
  function actionsFor(row: any) {
    if (state.tab === 'proposals') return [proposal(row, true), proposal(row, false)]
    return data.page === 'users' ? [] : [removal(row)]
  }
  /** @returns {void} */
  function environment(): void {
    form.show(
      page.msg('projects.environment'),
      [
        {
          key: 'env',
          label: 'projects.environment',
          options: state.environments.map((e) => ({ value: e.slug, label: e.name })),
        },
      ],
      { env: state.env },
      async () => {
        state.env = form.values.env
        state.rows = []
        await load()
      },
    )
  }
  /** @returns {void} */
  function newEnvironment(): void {
    form.show(
      page.msg('common.newEnvironment'),
      [field('slug', 'common.slug'), field('name', 'common.name')],
      { slug: '', name: '' },
      async () => {
        await mutate(`${base}/environments`, 'POST', form.values)
      },
    )
  }
  return {
    actionsFor,
    state,
    form,
    refresh,
    select,
    edit,
    remove,
    review,
    environment,
    newEnvironment,
  }
}
