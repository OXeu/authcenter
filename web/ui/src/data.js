/**
 * @typedef {object} User
 * @property {number} id
 * @property {string} name
 * @property {string} email
 * @property {string} role
 * @property {string | null} [last_login_at]
 */

/**
 * The page the Go shell serialises into `<script type="application/json" id="page-data">`.
 * @typedef {object} PageData
 * @property {'login' | 'projects' | 'project' | 'users'} page
 * @property {string} lang
 * @property {string} title message id
 * @property {string} publicUrl
 * @property {string} providerName
 * @property {string} nonce
 * @property {string} csrf
 * @property {boolean} loggedIn
 * @property {boolean} isAdmin
 * @property {boolean} oauthEnabled
 * @property {User | null} me
 * @property {{ slug: string, name: string } | null} project
 * @property {string} env
 * @property {boolean} canWrite
 * @property {string} languageLabel
 * @property {{ tag: string, label: string }[]} languages
 * @property {Record<string, string>} messages
 * @property {{ message: string, level: 'error' | 'success' } | null} flash
 */

export const pages = ['login', 'projects', 'project', 'users']
