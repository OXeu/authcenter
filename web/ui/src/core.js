/**
 * @typedef {object} ApiInit
 * @property {string} [method]
 * @property {unknown} [body]
 * @property {string} [csrf]
 */

/**
 * Calls the JSON API and turns `{ error }` failures into an Error whose cause
 * is the HTTP status (0 for a network failure).
 * @param {string} path
 * @param {ApiInit} [init]
 * @returns {Promise<any>}
 */
export async function api(path, init = {}) {
  const method = init.method ?? 'GET'
  /** @type {Record<string, string>} */
  const headers = { Accept: 'application/json', 'X-CSRF-Token': init.csrf ?? '' }
  if (init.body !== undefined) headers['Content-Type'] = 'application/json'
  const resp = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: init.body === undefined ? undefined : JSON.stringify(init.body),
  })
  if (resp.status === 204) return null
  /** @type {any} */
  const data = await resp.json().catch(() => ({}))
  if (!resp.ok)
    throw new Error(typeof data.error === 'string' ? data.error : `HTTP ${resp.status}`, {
      cause: resp.status,
    })
  return data
}

/**
 * @param {string | null | undefined} iso
 * @returns {string}
 */
export function timeOnly(iso) {
  if (!iso || iso.startsWith('0001-')) return ''
  return new Date(iso).toLocaleString()
}
