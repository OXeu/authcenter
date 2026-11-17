# Deploy AuthCenter

AuthCenter needs a running ConfigCenter. Only connection/bootstrap settings come from the local environment; runtime settings are read from its ConfigCenter project. See [the complete configuration reference](configuration.md) for all keys and their refresh behavior.

## Minimal GitHub sign-in deployment

1. In ConfigCenter, create a dedicated AuthCenter project and its `production` environment. Issue an API key with configuration-read permission for that project.
2. Register a GitHub OAuth application with the callback `http://127.0.0.1:8081/oauth/callback` for a local instance.
3. Store the following runtime values in that ConfigCenter environment:

| Key                        | Value                                                         |
| -------------------------- | ------------------------------------------------------------- |
| `AC_LOCAL_LOGIN`           | `false` for GitHub-only authentication                        |
| `AC_OAUTH_CLIENT_ID`       | The registered GitHub application's client ID                 |
| `AC_OAUTH_CLIENT_SECRET`   | Its client secret                                             |
| `AC_BOOTSTRAP_ADMIN_EMAIL` | The intended administrator's verified GitHub email            |
| `AC_PUBLIC_URL`            | `http://127.0.0.1:8081` locally, or the deployed HTTPS origin |

4. In the AuthCenter checkout, run `make bootstrap` and `make build`. Git access to the common repository pinned by `common.lock.json` is required.
5. Set `AC_CONFIGCENTER_URL`, `AC_CONFIGCENTER_API_KEY` and `AC_CONFIGCENTER_ENVIRONMENT=production` in the environment or ignored `.env.local`, then run `./.tmp/bin/authcenter`.

The server listens at `127.0.0.1:8081` and stores application data in `.tmp/authcenter.db` by default. Set the initial administrator identity before the first database initialization. A ConfigCenter connection failure prevents startup.

## Password authentication

Password authentication requires `AC_LOCAL_LOGIN=true` and a persistent 32-byte Base64 `AC_ENCRYPTION_KEY`. New password registrations also require Resend mail configuration and registration to be enabled. Store these runtime values in ConfigCenter. Existing accounts with TOTP can use their authenticator; see the [security guide](security.md).

## Production

Use an HTTPS public origin, a production database and persistent encryption configuration. The [configuration guide](configuration.md) includes MySQL and Tencent Cloud Functions settings, required GitHub Actions secrets and the deployment/verification commands. Runtime changes are applied through the settings page's refresh action or a restart; database, listening address and encryption changes require a restart.

## Shared infrastructure

`make common-local COMMON_DIR=../center-common` selects a neighboring common checkout for development. `CI=true make bootstrap check build` selects the pinned revision. Update `common.lock.json` deliberately with matching adaptations, and restore both for rollback. Business policy, application models, secrets and deployment configuration remain in AuthCenter.
