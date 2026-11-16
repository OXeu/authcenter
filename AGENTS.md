# AGENTS.md

Rules for anyone (human or model) changing configcenter. They are enforced where a tool exists; where a rule is judgement, it is listed as such.

## Layout

- `cmd/configcenter` — main, flags-free, environment driven. `internal/config` loads `.env.local` (or `CC_ENV_FILE`) with godotenv before reading variables; exported variables are never overridden. Never print secret values.
- `internal/config` — loads the env file and reads every environment variable here and nowhere else.
- `internal/store` — gorm models and all queries; one `Store` method per query.
- `internal/auth` — sessions, roles, CSRF, API-key and agent middleware, OAuth provider adapter.
- `internal/oauth` — the RFC 6749 + OIDC client (discovery, PKCE, id_token verification, userinfo fallback).
- `internal/server` — gin router, REST handlers, MCP endpoint, Go templates (`templates/shell/alpine.html`), embedded static files.
- `web/ui` — Alpine components and the Tailwind entry; bundled by esbuild into `internal/server/static`.
- `sdk/configcenter` — separate module with the public Go client.
- `docs/commits.md` — the commit message rules (Conventional Commits 1.0.0 plus this repository's constraints).

## Commits

Commit messages follow [`docs/commits.md`](docs/commits.md): Conventional Commits 1.0.0 with the repository's own constraints. When asked to commit, split the work into one commit per functional change and keep committing until `git status` is clean — never commit only the part of the working tree you touched. The full procedure is in that document; do not restate or shorten it here.

## Files created while working

1. **Nothing but source and test code goes into the source tree.** Scratch scripts, debug programs, logs, traces, generated databases, fixtures, coverage output, build output and one-off experiments are not created under `cmd/`, `internal/`, `sdk/`, `web/`, `tools/` or `docs/`. Test code is a committed `*_test.go` / `web/ui/__tests__/*.test.mjs` file; nothing else is added to a package directory, and a test that opens a database or a filesystem path opens it in memory or under `.tmp/`.
2. **Every artifact goes to `.tmp/` at the repository root.** Use `.tmp/<subdirectory>` for anything a command, test or experiment writes. `.tmp/` is gitignored and never committed.
3. **`/tmp` and friends are forbidden**, including the system temp dir, `t.TempDir()`, `os.MkdirTemp("", …)`, `os.CreateTemp("", …)`, `fs.mkdtempSync` outside `.tmp` and shell `mktemp`. A test that needs a directory gets one under `.tmp/`, from a path built on the repository root.
4. **Clean up.** Delete what you created under `.tmp/` when you are done; if it must stay for later inspection, say so and give its path.
5. **Go's scratch space.** `go test` and `go build` write their intermediate working directory to `$TMPDIR`, which defaults to `/tmp`; `go` removes it on exit. `make test` sets `TMPDIR` to `.tmp`, so `make test` and `make check` never touch `/tmp`. Run Go commands through `make` or set `TMPDIR=$PWD/.tmp` yourself. The build cache (`GOCACHE`) is a cache, not an artifact, and stays where the environment puts it.
6. Verify: after a task that ran tests or experiments, `git status --short` shows no new untracked paths other than those intended for the task, and `.tmp/` contains only what is still needed.

## Backend rules

1. **errcheck** — every returned error is handled. `check-blank: true`: `_ = risky()` is a lint failure too.
2. **nilerr** — an error branch never returns a nil error.
3. **wrapcheck** — errors crossing a package boundary are wrapped with `fmt.Errorf("…: %w", err)`. Same-package returns need no wrapping.
4. **errorlint** — compare with `errors.Is`/`errors.As`; only `%w` wraps.
5. **bodyclose / sqlclosecheck** — HTTP bodies and SQL rows are closed on every path.
6. **contextcheck / containedctx** — `context.Context` is the first parameter, comes from the request or the process, and is never stored in a struct.
7. **govet lostcancel / nilness, staticcheck SA5000/SA5011/SA1019** — no leaked cancels, no nil dereferences, no deprecated APIs.
8. **forcetypeassert** — type assertions use the comma-ok form.
9. **nilnil** — no `(nil, nil)` for pointer/interface + error pairs; a deliberate exception carries a `//nolint:nilnil` with the reason.
10. **exhaustive** — switches over iota enums cover every value or have an explicit default.
11. **unused / ineffassign** — no unused code or dead assignments.
12. **No comments** — code explains itself. The only comments allowed document an exported identifier, a non-obvious constraint of an external system, or a `//nolint` directive.
13. **forbidigo** — no `fmt.Print*`, no `log.Fatal*`, no `os.Getenv` outside `internal/config`. Use `log/slog` and return errors.
14. **godox** — no TODO/FIXME/HACK/XXX markers in committed code.
15. **Formatter** — gofumpt + goimports (`golangci-lint fmt`).

`make lint` runs all of the above.

## Frontend rules

1. **No eyebow elements.** Do not ship decorative markup or placeholder elements to describe what something is or to record a change. If it does not carry data or behaviour, it is not in the template.
2. **Inputs live in dialogs.** `local/input-in-dialog-only` rejects `<input>`, `<select>` and `<textarea>` anywhere outside a dialog container — a `role="dialog"` / `role="alertdialog"` element, an `aria-modal="true"` element, or a `data-dialog` container. Pages open the shared dialog defined once in `templates/shell/alpine.html` (`{{define "dialog"}}`) through the `dialogState()` machine in `web/ui/components.js`; page bodies only carry buttons and read-only views. Page-level filters and environment switches that used to be controls are plain text now.
3. **Minify elements.** Reuse the components in `app.css` (`.card`, `.btn`, `.field`, `.input`, `.table`, `.badge`, `.tabs-trigger`). No wrapper elements that exist only for styling when a class would do; no duplicate markup for successive Alpine states — toggle with `x-show`.
4. **Messages are toasts.** `local/no-inline-message` rejects any element that carries a message instead of data: `role="alert"` / `role="status"` / `role="log"`, classes such as `alert`, `notice`, `callout`, `inline-error`, `toast-container`, or a class or `msg('…')` id whose last segment is `message`, `notification`, `snackbar` or `banner` (so `<meta name="message">` stays allowed), `popover`, `tooltip` and `:x-tooltip` attributes. `local/no-inline-tooltip` extends the same ban to every way a template could build a tooltip or hint in the DOM: `role="tooltip"` (`tooltip`, `:role`, `x-bind:role`), tooltip and popover classes (`tip`, `tst`, `tut`, `tooltip`, `tooltip-content`, `popover-content`), tooltip attributes (`x-tooltip`, `x-anchor`, `x-popper`, `tippy`, `placement`, `arrow`, `data-tooltip*`, `data-toggle*`), hover and focus handlers (`@mouseenter`, `@mouseover`, `@mouseleave`, `@mouseout`, `@focusin`, `@focusout`, without `.tooltip`), `<el-tooltip>` / `<tippy>` / `<v-tooltip>` markup and `{{!-- … --}}` comments, and a bare `resolve('notify(…)')` that Alpine cannot evaluate. Nothing about a transient message is embedded in the page; client code calls `notify(type, title, description)` from `web/ui/toast.js`, which mounts [ding](https://github.com/YusufCeng1z/ding) once with its own live regions and stylesheet, and server messages reach it only through the `<meta name="message">` the shell renders from `.LoginFailed`. Page components report errors with `page.report(err)`; a success confirmation is a `notify('success', …)` after the action. Dialog validation errors stay as inert `<p class="field">` text next to the inputs, not as alerts.
5. **`alpinejs/no-raw-dom-access` (most important).** No `document.querySelector`, `document.getElementById`, `innerHTML`, `outerHTML` or `insertAdjacentHTML` in Alpine expressions. Use `x-ref`, `x-model`, `x-bind`, `x-text`, `$watch`. `eslint`'s `no-restricted-syntax` applies the same ban to component code and only allows `document.querySelector` to read root `data-*` attributes once in an `init`.
6. **`eslint-plugin-template-security`** — unsafe sinks in templates (`x-html` with dynamic input, `javascript:` URLs, `target=_blank` without `rel`) fail linting.
7. **markuplint** — every template is linted with the Alpine parser and spec. Accessibility (`require-accessible-name` on controls) is enforced; attribute rules are relaxed because Go template actions (`{{if}}`, `{{end}}`) live inside the markup.
8. **Expression length limit** — `local/max-inline-expression` fails any `x-on`, `x-bind`, `x-model`, `x-text`, `x-data` or `x-init` expression over 50 characters. Behaviour belongs in an `Alpine.data()` component in `web/ui/components.js`; even a method reference longer than 50 characters (e.g. `'querySelector("…")'`) must become a getter or a shorter call. Alpine cannot see page-scope helpers such as `notify`, so a short expression that needs one goes through `resolve('…')` (or `warn('id')` for a translated warning) on the component.
9. **Alpine v3 only** — `local/no-v2-syntax` rejects `x-spread` and friends; `local/no-invalid-directive` rejects unknown `x-*` directives and raw DOM access inside expressions.
10. **File length.** `max-lines` caps every template and component file at 400 lines, ignoring blank lines and comment lines. Split a page into its own template or its component into `Alpine.data()` factories before it grows past that.
11. **No hard-coded user-facing text.** Templates use `{{t "id" .}}`; component code uses `this.msg("id")` from `web/ui/i18n.js`. Both message tables (`internal/i18n/locales/*.json` and `web/ui/i18n.js`) must carry every id in `en`; a missing id renders as the raw id, which the language test in `internal/server/e2e_test.go` and `npm test` catch. The switch stays at `/language?lang=` plus the header dropdown; do not translate server messages from JavaScript.
12. **Icons.** Icons come from Remix Icon classes (`<i class="ri-…-line" aria-hidden="true"></i>`, stylesheet at `/static/icons/remixicon.css`, font vendored by `tools/copy-icons.mjs`). `local/no-inline-icons` rejects `<svg>` markup and emoji anywhere in a template, including inside dialogs; the one allowed `<svg>` is `/static/favicon.svg`, which is not a template.
13. **Types** — `jsconfig.json` has `checkJs: true` and `strict: true`; every function carries JSDoc types. `@types/alpinejs` does not exist on npm, so component shapes are typed as `any` at the factory and `any` payloads are cast explicitly (`/** @type {any} */`) at the API boundary. `tsc -p jsconfig.json` must pass.

Scripts in `web/ui` run before Alpine starts; register components in `register()` (`web/ui/index.js`) rather than on `alpine:init`.

## Testing

- `go test ./...` — OAuth login through a fake identity provider, project isolation, CSRF.
- `go test ./...` in `sdk/configcenter`; the live test runs only with `CC_TEST_URL` and `CC_TEST_KEY`.
- `npm test` — jsdom tests for the Alpine components (`web/ui/__tests__`), asserting the dialog actions and the requests they send, a test of the toast mount, plus linter tests for the local input-in-dialog, no-inline-message, no-inline-tooltip, max-lines and no-inline-icons rules.
- `internal/server/e2e_test.go` renders every page, logs in through a fake IdP and proves a key of one project cannot reach another. Its `TestMain` (`scratch_test.go`) points `TMPDIR` at `.tmp/go-test` so `go test` run directly, without `make`, also keeps its scratch files in the workspace.
- Test databases are `mode=memory`; tests never open a file database, so a failing run leaves nothing behind in a package directory.

## Definition of done

`make check` passes: `golangci-lint run ./...`, `go test ./...`, `eslint web/ui internal/server/templates`, `markuplint internal/server/templates`, `tsc -p jsconfig.json`, `npm test`, and the SDK's own lint and tests. Confirmed working steps are committed as described in [`docs/commits.md`](docs/commits.md), so the definition of done includes a clean `git status`, with every artifact you produced living under the ignored `.tmp/` directory as described above.
