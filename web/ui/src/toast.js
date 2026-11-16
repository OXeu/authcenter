import { mountToaster, toast } from '@yusufceng1z/ding'

import { createCopy } from './i18n.js'

let mounted = false

/**
 * Mounts the toast viewport once; the library injects its own stylesheet and
 * live regions, so no component renders a message element.
 * @param {string} lang
 * @returns {void}
 */
export function mountToasts(lang) {
  const { msg } = createCopy(lang)
  if (!mounted) {
    mounted = true
    mountToaster({ position: 'bottom-right', theme: 'dark' })
  }
  toast.configure({
    labels: { region: msg('toast.region'), dismiss: msg('toast.dismiss') },
    held: 'queue',
  })
}

/**
 * Shows a message as a toast instead of embedding it in the page.
 * @param {'success' | 'error' | 'warning' | 'info' | 'loading'} type
 * @param {string} title
 * @param {string} [description]
 * @returns {string} the toast id
 */
export function notify(type, title, description) {
  mountToasts('en')
  const options = description ? { description } : {}
  if (type === 'loading') return toast.loading(title, options)
  return toast[type](title, options)
}
