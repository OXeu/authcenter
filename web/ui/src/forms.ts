import { reactive } from 'vue'

/** @param {import('./page.ts').PageContext} page */
export function createForm(page: import('./page.ts').PageContext) {
  return reactive({
    open: false,
    keepOpen: false,
    info: '',
    secondaryLabel: '',
    secondaryAction: async () => {},
    async secondary() {
      if (this.busy) return
      this.busy = true
      try {
        await this.secondaryAction()
      } finally {
        this.busy = false
      }
    },
    danger: false,
    title: '',
    busy: false,
    error: '',
    token: '',
    fields: /** @type {any[]} */ ([]) as any[],
    values: /** @type {Record<string, string>} */ ({}) as Record<string, string>,
    action: /** @type {() => Promise<void>} */ (async () => {}) as () => Promise<void>,
    /** @param {string} title @param {any[]} fields @param {Record<string,string>} values @param {() => Promise<void>} action */
    show(
      title: string,
      fields: any[],
      values: Record<string, string>,
      action: () => Promise<void>,
    ) {
      this.danger = false
      this.keepOpen = false
      this.info = ''
      this.secondaryLabel = ''
      this.secondaryAction = async () => {}
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
        if (!this.token && !this.keepOpen) this.open = false
      } catch (err) {
        this.error = page.describe(err)
      } finally {
        this.busy = false
      }
    },
  })
}
