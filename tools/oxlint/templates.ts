import { checkTemplate as commonTemplate } from '@oxeu/center-common/tools/oxlint/templates.ts'
import { checkFormFeedback } from './form-feedback.ts'

export function checkTemplate(source: string, filename = '', script = ''): string[] {
  return [
    ...commonTemplate(source, filename),
    ...(filename === 'web/ui/src/ToastViewport.vue' ? [] : checkFormFeedback(source, script)),
  ]
}
