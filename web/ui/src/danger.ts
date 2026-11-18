import type { createForm } from './forms.ts'
export const dangerOperations = {
  signOut: 'nav.signOut',
  deleteClient: 'common.delete',
  revokeGrant: 'apps.revoke',
  disableTOTP: 'account.disable',
  removeEmail: 'account.removeEmail',
  deleteUser: 'users.delete',
  banUser: 'users.ban',
  unbanUser: 'users.unban',
  deleteKey: 'common.delete',
  rotateKey: 'keys.rotate',
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
  fields: any[] = [],
  values: Record<string, string> = {},
): DangerAction {
  return {
    [confirmedAction]: true,
    id,
    label,
    request() {
      form.show(`${label}: ${target}`, fields, values, execute)
      form.danger = true
    },
  }
}
