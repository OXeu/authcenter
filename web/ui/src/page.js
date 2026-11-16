import { describeError } from './errors.js'
import { createCopy } from './i18n.js'
import { notify } from './toast.js'

/**
 * @typedef {import('./data.js').PageData} PageData
 */

/**
 * @typedef {object} PageContext
 * @property {(id: string, vars?: Record<string, string>) => string} msg
 * @property {string} csrf
 * @property {(err: unknown) => string} describe
 * @property {(err: unknown) => void} report
 */

/**
 * Copy and the CSRF token shared by every component, provided as `page`.
 * @param {PageData} data
 * @returns {PageContext}
 */
export function createPage(data) {
  const { msg } = createCopy(data.lang, data.messages)
  return {
    msg,
    csrf: data.csrf,
    describe: (err) => describeError(msg, err),
    report: (err) => notify('error', msg('error.title'), describeError(msg, err)),
  }
}
