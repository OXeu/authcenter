import en from '../../../internal/i18n/locales/en.json'
import zh from '../../../internal/i18n/locales/zh-CN.json'

/** @type {Record<string, Record<string, string>>} */
export const fallbackMessages = { en, 'zh-CN': zh }

/**
 * @typedef {object} Copy
 * @property {(id: string, vars?: Record<string, string>) => string} msg
 * @property {string} lang
 */

/**
 * @param {string} lang
 * @param {Record<string, string>} [serverMessages]
 * @returns {Copy}
 */
export function createCopy(lang, serverMessages = {}) {
  const table = { ...fallbackMessages.en, ...fallbackMessages[lang], ...serverMessages }
  /**
   * @param {string} id
   * @param {Record<string, string>} [vars]
   * @returns {string}
   */
  const msg = (id, vars = {}) =>
    (table[id] ?? id).replace(
      /\{\{\.?(\w+)\}\}/gu,
      (_, /** @type {string} */ name) => vars[name] ?? '',
    )
  return { msg, lang }
}
