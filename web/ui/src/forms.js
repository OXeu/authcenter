import { reactive } from 'vue'

/** @param {import('./page.js').PageContext} page */
export function createForm(page) {
  return reactive({
    open: false,
    title: '',
    busy: false,
    error: '',
    token: '',
    fields: /** @type {any[]} */ ([]),
    values: /** @type {Record<string, string>} */ ({}),
    action: /** @type {() => Promise<void>} */ (async () => {}),
    /** @param {string} title @param {any[]} fields @param {Record<string,string>} values @param {() => Promise<void>} action */
    show(title, fields, values, action) {
      this.title = title
      this.fields = fields
      this.values = { ...values }
      this.action = action
      this.error = ''
      this.token = ''
      this.open = true
    },
    async submit() {
      if (this.busy) return
      this.busy = true
      this.error = ''
      try {
        await this.action()
        if (!this.token) this.open = false
      } catch (err) {
        this.error = page.describe(err)
      } finally {
        this.busy = false
      }
    },
  })
}
