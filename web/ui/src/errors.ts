/** @type {Record<string, string>} */
const statusMessages: Record<string, string> = {
  400: 'error.badRequest',
  401: 'error.unauthorized',
  403: 'error.forbidden',
  404: 'error.notFound',
  409: 'error.conflict',
  429: 'error.rateLimit',
  500: 'error.500',
  502: 'error.502',
}

/**
 * @param {unknown} err
 * @returns {string}
 */
export function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

/**
 * @param {unknown} err
 * @returns {number}
 */
function codeOf(err: unknown): number {
  const code = err instanceof Error ? err.cause : undefined
  return typeof code === 'number' ? code : 0
}

/**
 * Translates a failed request by its status; an unknown status keeps the
 * server's own text.
 * @param {(id: string) => string} msg
 * @param {unknown} err
 * @returns {string}
 */
export function describeError(msg: (id: string) => string, err: unknown): string {
  const authErrors = [
    'invalid_credentials',
    'invalid_email',
    'email_in_use',
    'local_login_disabled',
    'enrollment_unavailable',
    'invalid_code',
    'invalid_challenge',
    'password_length',
    'registration_unavailable',
    'second_factor_unavailable',
    'email_delivery_failed',
    'email_unavailable',
    'rate_limited',
  ]
  const message = messageOf(err)
  if (authErrors.includes(message)) return msg(`authError.${message}`)
  const code = codeOf(err)
  const key = code === 0 ? 'error.network' : statusMessages[code]
  if (key === undefined) return messageOf(err)
  const known = msg(key)
  return known === key ? messageOf(err) : known
}
