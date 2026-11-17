export type User = {
  id: number
  name: string
  avatar_url?: string
  email: string
  role: string
  last_login_at?: string | null
  banned?: boolean
  banned_until?: string | null
}

export type PageData = {
  page: 'login' | 'applications' | 'consent' | 'account' | 'users' | 'docs'
  docPage?: string
  lang: string
  title: string
  publicUrl: string
  providerName: string
  nonce: string
  csrf: string
  loggedIn: boolean
  isAdmin: boolean
  oauthEnabled: boolean
  localLogin: boolean
  emailLogin: boolean
  registration: boolean
  authorization?: {
    id: string
    client: import('./workspace.ts').Application
    scopes: string[]
    redirect_uri: string
  }
  me: User | null
  project: { slug: string; name: string } | null
  env: string
  canWrite: boolean
  languageLabel: string
  languages: { tag: string; label: string }[]
  messages: Record<string, string>
  flash: { message: string; level: 'error' | 'success' } | null
}

export const pages = ['login', 'applications', 'consent', 'account', 'users', 'docs']
