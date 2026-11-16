# configuration center

Self-hosted configuration service: projects as isolation boundaries, environments, OAuth single sign-on, project-scoped API keys, a Vue web UI and an MCP endpoint for agents.

## Stack

Go 1.26, gin, gorm (SQLite), a Go HTML/JSON shell plus Vue 3 (built with Vite Plus), shadcn/ui-style Tailwind CSS with Remix Icon (font vendored, no inline SVG), transient messages as [ding](https://github.com/YusufCeng1z/ding) toasts instead of inline alert elements, a plain OAuth 2.0 authorization-code + PKCE client that works with any OIDC issuer (or a plain OAuth server with a userinfo endpoint).

## Quick start

```sh
make build          # bundles the web assets, then builds .tmp/bin/configcenter
CC_OAUTH_ISSUER=https://idp.example.com \
CC_OAUTH_CLIENT_ID=configcenter \
CC_OAUTH_CLIENT_SECRET=... \
.tmp/bin/configcenter
```

Open http://127.0.0.1:8080. Without OAuth configured the server still starts; only the login link is disabled.

GitHub can be used as the identity provider (`CC_OAUTH_ISSUER=https://github.com`, a GitHub OAuth app with the callback `CC_PUBLIC_URL/oauth/callback`): GitHub publishes no OIDC discovery document, so the client falls back to GitHub's documented OAuth endpoints, sends the client ID and secret in the token request form, skips `openid`/PKCE, reads the profile from `/user` and the primary address from `/user/emails`. Accounts without a public email get `<id>@github.com`.

## Configuration

All settings come from the environment (read once in `internal/config`). The binary loads `.env.local` itself with [godotenv](https://github.com/joho/godotenv) (`CC_ENV_FILE` selects another file); variables already exported win over the file, so `CC_ADDR=127.0.0.1:9000 .tmp/bin/configcenter` and `CC_ADDR=… make run` both work, and a missing file is fine. `CC_OAUTH_ISSUER` and `CC_PUBLIC_URL` are trimmed of trailing slashes, and startup logs the resulting address, public URL and OAuth state.

| Variable                                        | Default                       | Meaning                                                                  |
| ----------------------------------------------- | ----------------------------- | ------------------------------------------------------------------------ |
| `CC_ADDR`                                       | `127.0.0.1:8080`              | Listen address                                                           |
| `CC_DB`                                         | `file:configcenter.db`        | SQLite DSN                                                               |
| `CC_PUBLIC_URL`                                 | `http://127.0.0.1:8080`       | Public base URL                                                          |
| `CC_OAUTH_ISSUER`                               |                               | OIDC issuer (discovery at `/.well-known/openid-configuration`)           |
| `CC_OAUTH_CLIENT_ID` / `CC_OAUTH_CLIENT_SECRET` |                               | OAuth client; redirect URI is `CC_PUBLIC_URL/oauth/callback`             |
| `CC_OAUTH_SCOPES`                               | `openid profile email groups` | Requested scopes                                                         |
| `CC_OAUTH_REDIRECT_URL`                         | derived                       | Override the callback URL                                                |
| `CC_OAUTH_GROUPS_CLAIM`                         | `groups`                      | Claim used for group mapping                                             |
| `CC_OAUTH_ADMIN_GROUPS`                         | `configcenter-admin`          | Groups that grant the admin role                                         |
| `CC_OAUTH_GROUP_MAP`                            |                               | `group:role,…` for editor/viewer defaults                                |
| `CC_SESSION_SECRET`                             |                               | Required when `CC_ENV=production`                                        |
| `CC_LANGUAGES`                                  | `en,zh-CN`                    | Enabled UI languages; tags must match `internal/i18n/locales/<tag>.json` |
| `CC_BOOTSTRAP_ADMIN_EMAIL` / `_NAME`            | `admin@localhost` / `admin`   | First administrator, created when the user table is empty                |

First login creates the user: linked by OAuth subject, adopted by verified email, otherwise new. Roles are managed afterwards in **Users** (admin only); group membership decides the role only when an account is created.

The header theme button switches between light and dark modes. Before a manual choice, the UI follows the system setting; the choice is saved in browser storage and shared across tabs.

## Languages

The UI ships in English and Simplified Chinese (`internal/i18n/locales`). The header dropdown lists `CC_LANGUAGES`, `/language?lang=<tag>` stores the choice in the `cc_lang` cookie and redirects back to the page; without a choice the server negotiates `Accept-Language`. Go supplies translated messages in the page JSON. Vue uses `web/ui/src/i18n.ts` and the same locale catalogs for dialogs, role badges and errors. Add a language by dropping `<tag>.json` into the locales directory and listing the tag in `CC_LANGUAGES`.

## Isolation model

- A **project** is the boundary. Every item, environment, key and proposal belongs to exactly one project.
- An **API key** belongs to one project. Requests with a key for project `billing` cannot address project `payments` at all: the key's project is implied by the key itself, the path only names the environment.
- **Agents** (MCP) and **humans** (web session) share the same authorization rules: viewer keys read, editor keys write, browsers get the mapped role.

## REST API (session, CSRF token in `X-CSRF-Token`)

| Method       | Path                                                                   |
| ------------ | ---------------------------------------------------------------------- |
| GET          | `/api/v1/me`                                                           |
| GET/POST     | `/api/v1/projects`                                                     |
| PATCH/DELETE | `/api/v1/projects/:project`                                            |
| GET/POST     | `/api/v1/projects/:project/environments`                               |
| DELETE       | `/api/v1/projects/:project/environments/:env`                          |
| GET          | `/api/v1/projects/:project/environments/:env/items?prefix=`            |
| PUT/DELETE   | `/api/v1/projects/:project/environments/:env/items/:key`               |
| GET/POST     | `/api/v1/projects/:project/keys`, DELETE `…/keys/:id`                  |
| GET          | `/api/v1/projects/:project/proposals?status=pending`                   |
| POST         | `/api/v1/projects/:project/proposals/:id/review` (`{"approve": true}`) |
| GET          | `/api/v1/agent/project` (API key)                                      |
| GET          | `/api/v1/agent/environments/:env/values?prefix=` (API key, read-only)  |
| GET          | `/api/v1/users`, PATCH `/api/v1/users/:id` (admin)                     |

## MCP

`POST /mcp/:project` speaks JSON-RPC 2.0 (Streamable HTTP shape, stateless). Authenticate with `Authorization: ApiKey <token>`; the web UI calls the same endpoint with its session cookie and CSRF token, so every change an agent or a person makes takes the same path.

Tools: `list_environments`, `list_config`, `get_config`, `delete_config`, `set_config`, `propose_config`, `list_proposals`. Viewer keys only see the read tools; `propose_config` records a change for human review, which the **Proposals** tab in the UI applies or rejects.

## Go SDK

`sdk/configcenter` is a separate module (`github.com/xeu/configcenter/sdk/configcenter`), dependency-free:

```go
client, err := configcenter.New(configcenter.Config{
	BaseURL: os.Getenv("CC_URL"), APIKey: os.Getenv("CC_KEY"),
})
if err != nil { /* handle */ }
if err := client.Load(ctx); err != nil { /* handle */ }
pool := client.Int("db.pool", 10)
if err := client.Watch(ctx, configcenter.WatchOptions{Interval: 30 * time.Second}); err != nil { /* handle */ }
```

The key decides the project, so the SDK never takes a project name.

## Development

Use Node 22.18+ or 24.11+ (supported Vite Plus LTS versions), npm and Go 1.26.10, with golangci-lint installed.

```sh
npm ci
make check   # build, Go lint/tests, Oxlint, Vue type checking, Vitest, SDK lint/tests
make build   # embed .tmp/ui and produce .tmp/bin/configcenter
make run     # build and start the Go server
npm run dev # in a second terminal; open http://localhost:5173/projects
```

The Vite development server gets authenticated page JSON from Go at `127.0.0.1:8080` and serves Vue with hot reload. API, OAuth, language and logout requests are proxied to Go. Set `CC_PUBLIC_URL=http://localhost:5173` and register that origin's `/oauth/callback` with your provider when testing OAuth through Vite. Production serves embedded JavaScript and CSS directly from Go; Node is only needed at build time.

Vite Plus provides the build, Oxlint, Oxfmt and Vitest. `vue-tsc` checks both scripts and templates. Oxlint custom plugins inspect script ASTs; separate compiler-based Vitest checks enforce template constraints. See [AGENTS.md](AGENTS.md).

Build output stays in ignored `.tmp/ui` and is embedded by `assets.go`. Run `make assets` before direct Go commands on a fresh checkout; Make targets do this automatically. Go scratch files use `.tmp`, and binaries use `.tmp/bin`. The live SDK test is opt-in through `CC_TEST_URL` and `CC_TEST_KEY`.

Type checking uses TypeScript 7 (`typescript-native`) for modules, tools and non-component tests. Vue template and component tests use `vue-tsc` with TypeScript 6 because Volar requires its JavaScript compiler API. Run both with `npm run typecheck`.
