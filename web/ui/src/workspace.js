import { reactive } from 'vue'
import { api } from './core.js'
import { createForm } from './forms.js'
import { notify } from './toast.js'

/** @param {string} value */
const enc = (value) => encodeURIComponent(value)
/** @param {string} key @param {string} label @param {boolean} [required] */
const field = (key, label, required = true) => ({ key, label, required })

/** @param {import('./data.js').PageData} data @param {import('./page.js').PageContext} page */
export function createWorkspace(data, page) {
  const form = createForm(page)
  const state = reactive({
    rows: /** @type {any[]} */ ([]),
    environments: /** @type {any[]} */ ([]),
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
  async function load() {
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
  async function refresh() {
    try {
      await load()
    } catch (err) {
      page.report(err)
    }
  }
  /** @param {string} tab */
  async function select(tab) {
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
  async function mutate(url, method, body) {
    const result = await api(url, { method, body, csrf: page.csrf })
    notify('success', page.msg('toast.saved'))
    await refresh()
    return result
  }
  /** @param {any} [row] */
  function edit(row) {
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
  /** @param {any} row */
  function remove(row) {
    const url = path() + '/' + enc(row.slug ?? row.key ?? String(row.id))
    form.show(
      page.msg('common.delete') + ': ' + (row.name ?? row.key ?? row.slug),
      [],
      {},
      async () => {
        await mutate(url, 'DELETE')
      },
    )
  }
  /** @param {any} row @param {boolean} approve */
  function review(row, approve) {
    form.show(
      page.msg(approve ? 'projects.approve' : 'projects.reject') + ': ' + row.key,
      [],
      {},
      async () => {
        await mutate(`${base}/proposals/${row.id}/review`, 'POST', { approve })
      },
    )
  }
  /** @returns {void} */
  function environment() {
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
  function newEnvironment() {
    form.show(
      page.msg('common.newEnvironment'),
      [field('slug', 'common.slug'), field('name', 'common.name')],
      { slug: '', name: '' },
      async () => {
        await mutate(`${base}/environments`, 'POST', form.values)
      },
    )
  }
  return { state, form, refresh, select, edit, remove, review, environment, newEnvironment }
}
