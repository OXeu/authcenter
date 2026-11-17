import type { createForm } from './forms.ts'
export const dangerOperations = {
  signOut: 'nav.signOut',
  deleteClient: 'common.delete',
  revokeGrant: 'apps.revoke',
} as const
export type DangerOperation = keyof typeof dangerOperations
const confirmedAction = Symbol('confirmedAction')
export type DangerAction = {
  [confirmedAction]: true
  id: DangerOperation
  label: string
  request: () => void
}
export function dangerAction(
  form: ReturnType<typeof createForm>,
  id: DangerOperation,
  label: string,
  target: string,
  execute: () => Promise<void>,
): DangerAction {
  return {
    [confirmedAction]: true,
    id,
    label,
    request() {
      form.show(`${label}: ${target}`, [], {}, execute)
      form.danger = true
    },
  }
}
