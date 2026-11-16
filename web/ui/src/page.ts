import { describeError } from './errors.ts'
import { createCopy } from './i18n.ts'
import { notify } from './toast.ts'

export type PageData = import('./data.ts').PageData

export type PageContext = {
  msg: (id: string, vars?: Record<string, string>) => string
  csrf: string
  describe: (err: unknown) => string
  report: (err: unknown) => void
}

/**
 * Copy and the CSRF token shared by every component, provided as `page`.
 * @param {PageData} data
 * @returns {PageContext}
 */
export function createPage(data: PageData): PageContext {
  const { msg } = createCopy(data.lang, data.messages)
  return {
    msg,
    csrf: data.csrf,
    describe: (err) => describeError(msg, err),
    report: (err) => notify('error', msg('error.title'), describeError(msg, err)),
  }
}
