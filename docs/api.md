# OAuth 与管理 API

## 应用注册

登录 `/applications`，创建应用，填写名称、主页、多个回调 URL（每行一个）和 Logo。Logo 支持 PNG/JPEG，最大 512 KiB、1024 × 1024 像素。回调和主页支持 HTTPS；本机开发可使用 localhost、127.0.0.1、::1 的 HTTP 地址。

机密应用保存创建时返回的 `client_secret`。公开客户端没有密钥，只能通过 PKCE 兑换授权码。创建后可以编辑名称、主页、Logo 和回调列表。

## 授权

```http
GET /oauth/authorize?client_id=CLIENT_ID&redirect_uri=https%3A%2F%2Fapp.example%2Fcallback&scope=profile%20email%20offline_access&state=RANDOM_STATE&response_type=code
```

`response_type` 可省略，默认 `code`。`redirect_uri` 可省略，使用第一个已登记回调；传入时必须与某个已登记地址完整匹配。`scope` 默认 `profile email`。`state` 原样返回，业务应用应生成随机值并核对，以防登录请求伪造。未登录用户在完成登录后自动继续原授权请求。

- `profile`：姓名和头像。
- `email`：邮箱。
- `offline_access`：签发刷新令牌，允许持续访问。

不需要 `nonce` 或 issuer 参数，不返回 `id_token`，也不要求客户端进行 OIDC issuer 验证。机密客户端 PKCE 可选；提供 challenge 时必须完成验证。公开客户端必须附带 `code_challenge` 和 `code_challenge_method=S256`。challenge 为 `BASE64URL(SHA256(code_verifier))`，verifier 为 43–128 个合法字符。

用户明确允许后回调附带 `code` 和原始 `state`；拒绝则附带 `error=access_denied`。无效客户端或未登记的回调直接返回 HTTP 400，不跳转。

## 兑换和刷新

```sh
curl https://auth.example/oauth/token \
  -u 'CLIENT_ID:CLIENT_SECRET' \
  -d grant_type=authorization_code \
  -d code=AUTHORIZATION_CODE \
  --data-urlencode redirect_uri=https://app.example/callback
```

可改用表单 `client_id`、`client_secret`；不要同时使用 Basic 与表单凭据。公开客户端只传 `client_id` 和 `code_verifier`。请求必须使用 `application/x-www-form-urlencoded`。

```json
{
  "access_token": "opaque-token",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "opaque-refresh-token",
  "scope": "profile email offline_access"
}
```

只有授权包含 `offline_access` 才返回刷新令牌。访问令牌有效期 1 小时，授权码有效期 5 分钟且只能兑换一次。

```sh
curl https://auth.example/oauth/token \
  -u 'CLIENT_ID:CLIENT_SECRET' \
  -d grant_type=refresh_token \
  -d refresh_token=REFRESH_TOKEN
```

每次刷新返回新的刷新令牌，客户端必须原子保存并串行刷新。刷新令牌族最长有效 30 天；轮换不会延长该期限。重复使用已消耗的授权码或刷新令牌会撤销所属授权。刷新不支持修改 scope。

## 用户信息及撤销

```sh
curl https://auth.example/oauth/userinfo -H 'Authorization: Bearer ACCESS_TOKEN'
curl https://auth.example/oauth/revoke -u 'CLIENT_ID:CLIENT_SECRET' -d token=TOKEN
```

用户信息始终包含稳定的 `sub` 和数值 `id`；按 scope 返回 `name`、`picture`、`avatar_url`、`email`。撤销接口接受访问令牌或刷新令牌并撤销整组授权；未知令牌也返回 200。令牌只在它所属的客户端下可撤销。用户也可在“我的授权”撤销访问；删除应用立即使其令牌失效。

`GET /.well-known/oauth-authorization-server` 提供 OAuth 授权服务器元数据，供支持发现的客户端选用。

## 管理 API

管理 API 只接受浏览器登录会话。写请求需要 `/api/v1/me` 返回的 `csrf` 作为 `X-CSRF-Token`。访问令牌不能代替管理会话。

| 方法           | 路径                     | 作用            |
| -------------- | ------------------------ | --------------- |
| GET            | `/api/v1/me`             | 当前用户与 CSRF |
| GET / POST     | `/api/v1/clients`        | 列出 / 注册应用 |
| PATCH / DELETE | `/api/v1/clients/:id`    | 编辑 / 删除应用 |
| GET            | `/applications/:id/logo` | 应用 Logo       |
| GET            | `/api/v1/grants`         | 当前用户的授权  |
| DELETE         | `/api/v1/grants/:id`     | 撤销自己的授权  |
| GET            | `/api/v1/users`          | 管理员列出用户  |
| PATCH          | `/api/v1/users/:id`      | 管理员修改角色  |

创建应用 JSON：

```json
{
  "name": "Example",
  "homepage": "https://app.example",
  "redirect_uris": ["https://app.example/callback", "http://localhost:3000/callback"],
  "logo": "BASE64_PNG_OR_JPEG_WITHOUT_DATA_URL_PREFIX",
  "public": false
}
```

PATCH 使用相同资料字段，`logo` 为空或省略时保留原图；不修改客户端密钥、ID、所有者或客户端类型。每个应用最多 20 个回调。
