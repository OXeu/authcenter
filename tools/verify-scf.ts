import { setTimeout } from 'node:timers/promises'

const origin = process.env.SCF_PUBLIC_URL?.replace(/\/$/u, '')
if (!origin || new URL(origin).protocol !== 'https:')
  throw new Error('SCF_PUBLIC_URL must be an HTTPS origin')

async function verify(): Promise<boolean> {
  try {
    const login = await fetch(`${origin}/login`, { signal: AbortSignal.timeout(65000) })
    if (!login.ok || !(await login.text()).includes('id="page-data"')) return false
    const discovery = await fetch(`${origin}/.well-known/oauth-authorization-server`, {
      signal: AbortSignal.timeout(20000),
    })
    if (!discovery.ok) return false
    const metadata = (await discovery.json()) as {
      issuer?: string
      authorization_endpoint?: string
      token_endpoint?: string
    }
    return (
      metadata.issuer === origin &&
      metadata.authorization_endpoint === `${origin}/oauth/authorize` &&
      metadata.token_endpoint === `${origin}/oauth/token`
    )
  } catch {
    return false
  }
}

let passed = false
for (let attempt = 0; attempt < 6; attempt++) {
  if (await verify()) {
    passed = true
    break
  }
  await setTimeout(5000)
}
if (!passed) throw new Error('Deployed login page or OAuth metadata failed verification')
process.stdout.write(`Verified login page and OAuth metadata at ${origin}\n`)
