import { describe, expect, it } from 'vite-plus/test'
import { checkTemplate } from '../../../tools/oxlint/templates.ts'

describe('form feedback requires toasts', () => {
  it.each([
    '<p>{{ error }}</p>',
    '<span>{{ successMessage }}</span>',
    '<div>{{ submitResult }}</div>',
    '<p v-if="submitted">Saved</p>',
    '<p v-if="saveFailed">Failed</p>',
    '<p v-if="isSaved">Saved</p>',
    '<p v-if="submitStatus">Saved</p>',
    '<p v-if="response.ok">Saved</p>',
    '<p v-if="form.error">Try again</p>',
    '<p v-else-if="form.success">Saved</p>',
    '<p v-show="state.feedback">Saved</p>',
    '<div v-text="response.message" />',
    '<p>{{ form?.error }}</p>',
    '<p>{{ form["error"] }}</p>',
    '<p>{{ error.value }}</p>',
    '<p>{{ submit_result }}</p>',
    '<template v-if="form.status"><p>Saved</p></template>',
    '<Feedback :text="form.error" />',
    '<Feedback v-bind="result" />',
    '<p>{{ msg("toast.saved") }}</p>',
    '<p>{{ msg("error.badRequest") }}</p>',
    '<p>{{ msg("authError.invalid_code") }}</p>',
    '<p>{{ msg("login.codeSent") }}</p>',
    '<p class="submission-result">Saved</p>',
    '<p class="success-message">Saved</p>',
    '<p :class="{ success: form.success }">Saved</p>',
    '<ul><li v-for="item in form.errors">{{ item }}</li></ul>',
    '<dialog><p v-if="form.error" class="field">{{ form.error }}</p></dialog>',
    '<dialog><p class="field">{{ msg("error.badRequest") }}</p></dialog>',
    '<dialog><p>{{ form.validationError }}</p></dialog>',
    '<p class="field">{{ form.validationError }}</p>',
    '<dialog><button class="field">{{ field.error }}</button></dialog>',
  ])('rejects feedback rendered by %s', (template) => {
    expect(checkTemplate(template)).toContain('form-feedback-requires-toast')
  })

  it.each([
    ['<p>{{ copy }}</p>', 'const copy = form.error'],
    ['<p>{{ copy }}</p>', 'const copy = computed(() => form.error)'],
    ['<p v-if="visible">Saved</p>', 'const visible = computed(() => form.success)'],
    ['<p>{{ copy }}</p>', 'const copy = computed(() => msg("toast.saved"))'],
    ['<p>{{ copy }}</p>', 'const copy = computed(() => { return form.error })'],
    ['<p>{{ copy }}</p>', 'const original = form.error; const copy = computed(() => original)'],
    ['<p>{{ copy }}</p>', 'const copy = computed(() => busy ? "" : form.error)'],
  ])('checks script aliases in %s with %s', (template, script) => {
    expect(checkTemplate(template, '', script)).toContain('form-feedback-requires-toast')
  })

  it.each([
    '<dialog><p class="field">{{ form.validationError }}</p></dialog>',
    '<dialog><div class="field"><span>{{ field.error }}</span></div></dialog>',
    '<dialog><p class="field">Name is required.</p></dialog>',
    '<dialog><p>{{ form.info }}</p><code>{{ form.token }}</code></dialog>',
    '<p>{{ row.name }} {{ row.status }}</p>',
    '<p :class="{ success: row.enabled }">{{ row.status }}</p>',
    '<p>{{ result.client.id }}</p>',
    '<p>{{ msg("account.enabled") }}</p>',
    '<p>{{ msg("account.help") }}</p>',
    '<button :disabled="form.busy" @click="submit">Save</button>',
    '<img v-if="!avatarFailed" @error="avatarFailed = true" />',
  ])('accepts non-feedback content in %s', (template) => {
    expect(checkTemplate(template)).toEqual([])
  })

  it('accepts toast calls from event handlers without rendering their messages', () => {
    expect(
      checkTemplate(
        '<button @click="submit">Save</button>',
        '',
        'const submit = () => notify("success", msg("toast.saved"))',
      ),
    ).toEqual([])
  })

  it('handles cyclic aliases without recursing indefinitely', () => {
    expect(
      checkTemplate('<p>{{ first }}</p>', '', 'const first = second; const second = first'),
    ).toEqual([])
  })

  it('rejects custom live regions even without feedback variable names', () => {
    expect(checkTemplate('<p aria-live="polite">Saved</p>')).toContain('inline-message')
  })
  it('reserves feedback rendering for the shared toast viewport', () => {
    const template = '<div role="alert">{{ state.alert }}</div>'
    expect(checkTemplate(template)).toContain('inline-message')
    expect(checkTemplate(template, 'web/ui/src/ToastViewport.vue')).toEqual([])
  })
})
