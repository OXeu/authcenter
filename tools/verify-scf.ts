import { setTimeout } from 'node:timers/promises'

const origin = process.env.SCF_PUBLIC_URL?.replace(/\/$/u, '')
if (!origin || new URL(origin).protocol !== 'https:')
  throw new Error('SCF_PUBLIC_URL must be an HTTPS origin')

async function verify(): Promise<boolean> {
  try {
    const login = await fetch(`${origin}/login`, { signal: AbortSignal.timeout(65000) })
    if (!login.ok) {
      process.stdout.write(`Login endpoint returned HTTP ${login.status}.\n`)
      return false
    }
    if (!(await login.text()).includes('id="page-data"')) {
      process.stdout.write('Login response does not contain the application page.\n')
      return false
    }
    const discovery = await fetch(`${origin}/.well-known/oauth-authorization-server`, {
      signal: AbortSignal.timeout(20000),
    })
    if (!discovery.ok) {
      process.stdout.write(`OAuth metadata endpoint returned HTTP ${discovery.status}.\n`)
      return false
    }
    const metadata = (await discovery.json()) as {
      issuer?: string
      authorization_endpoint?: string
      token_endpoint?: string
    }
    const matches =
      metadata.issuer === origin &&
      metadata.authorization_endpoint === `${origin}/oauth/authorize` &&
      metadata.token_endpoint === `${origin}/oauth/token`
    if (!matches) process.stdout.write('OAuth endpoint URLs do not match SCF_PUBLIC_URL.\n')
    return matches
  } catch {
    process.stdout.write('Verification request failed or timed out.\n')
    return false
  }
}

async function verifyWithRetries(attempts: number): Promise<boolean> {
  if (await verify()) return true
  if (attempts === 1) return false
  await setTimeout(5000)
  return verifyWithRetries(attempts - 1)
}

if (!(await verifyWithRetries(6)))
  throw new Error('Deployed login page or OAuth metadata failed verification')
process.stdout.write(`Verified login page and OAuth metadata at ${origin}\n`)
