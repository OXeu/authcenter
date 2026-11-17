# MCP 服务

AuthCenter 在 `/mcp` 提供 Streamable HTTP MCP 服务。MCP 与 `/api/v1` 使用同一份操作清单、处理函数、角色与资源归属检查，覆盖账户安全、应用、授权、用户、系统设置和 API Key 管理。协议由官方 Go SDK 处理，支持 `2026-07-28` 及其兼容版本；使用无状态 JSON 响应，不创建 MCP 会话 ID。

## OAuth 接入

1. 登录 `/applications`，为 MCP 客户端创建应用，登记客户端提供的完整回调地址。桌面客户端使用公开客户端；能够安全保存密钥的服务端客户端可选择机密客户端。保存 `client_id`，机密客户端还需保存创建时展示的 `client_secret`。现有应用若未包含 `mcp` scope，需要新建应用。
2. 在 MCP 客户端配置服务 URL `https://auth.example/mcp`，并配置上一步的客户端凭据。支持预注册 OAuth 客户端的工具可直接接入；本服务不提供动态客户端注册。
3. 客户端从 HTTP 401 的 `WWW-Authenticate` 发现 `/.well-known/oauth-protected-resource/mcp`，再从 `/.well-known/oauth-authorization-server` 获取授权与令牌端点。
4. 客户端通过浏览器发起授权码请求。无论公开或机密客户端，都必须使用 S256 PKCE、明确的 `redirect_uri`、`scope=mcp` 和 `resource=https://auth.example/mcp`。需要刷新令牌时增加 `offline_access`。
5. 用户登录并确认授权；回调包含 `code`、原始 `state` 和授权服务器 `iss`。客户端核对 `state`、`iss`，再使用同一资源地址和 PKCE verifier 兑换令牌。

```http
GET /oauth/authorize?response_type=code&client_id=CLIENT_ID&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&scope=mcp%20offline_access&resource=https%3A%2F%2Fauth.example%2Fmcp&code_challenge=CHALLENGE&code_challenge_method=S256&state=RANDOM_STATE
```

公开客户端兑换示例：

```sh
curl https://auth.example/oauth/token \
  -d grant_type=authorization_code \
  -d client_id=CLIENT_ID \
  -d code=AUTHORIZATION_CODE \
  -d code_verifier=PKCE_VERIFIER \
  --data-urlencode redirect_uri=https://client.example/callback \
  --data-urlencode resource=https://auth.example/mcp
```

刷新请求同样必须携带 `resource=https://auth.example/mcp`。令牌有效期、轮换与重放撤销规则见 [OAuth API](/docs/api)。授权码与刷新令牌不能被换成其他资源的令牌。OAuth 用户信息令牌、浏览器 Cookie 和 API Key 均不能用于 `/mcp`；MCP 令牌也不能直接用于管理 REST API 或 `/oauth/userinfo`。

`mcp` scope 代表代理当前用户执行管理 API 操作，授权页面会明确说明此权限。工具每次调用按用户当前角色和资源归属执行；普通用户无法获取或调用管理员工具。账户封禁、删除应用或撤销授权使访问立即失效。修改密码、邮箱或停用 TOTP 会撤销当前授权，需要重新授权。

## 调用

每个请求使用 `Authorization: Bearer ACCESS_TOKEN`。`initialize` 完成版本协商后，后续请求带上协商出的 `MCP-Protocol-Version`；同时发送 JSON Content-Type 和支持 JSON/SSE 的 Accept。

```sh
curl https://auth.example/mcp \
  -H 'Authorization: Bearer ACCESS_TOKEN' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2026-07-28' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_clients","arguments":{}}}'
```

工具结果包含 `structuredContent.http_status` 和原 API JSON `structuredContent.body`，同时提供文本 JSON。HTTP 204 的结果没有 `body`。原 API 的 4xx/5xx 设置 `isError=true`；工具参数由 JSON Schema 校验。URL 路径中的 `id` 使用字符串参数，其他 JSON 字段直接作为工具参数传入。

| 功能           | 工具                                                                                                                |
| -------------- | ------------------------------------------------------------------------------------------------------------------- |
| 账户           | `get_identity`、`get_account_security`、`set_password`、`start_email_change`、`verify_email_change`、`disable_totp` |
| 应用           | `list_clients`、`create_client`、`update_client`、`delete_client`                                                   |
| 授权           | `list_grants`、`revoke_grant`                                                                                       |
| API Key        | `get_key_scopes`、`list_keys`、`create_key`、`update_key`、`delete_key`、`rotate_key`                               |
| 用户（管理员） | `list_users`、`update_user_role`、`ban_user`、`unban_user`、`delete_user`                                           |
| 设置（管理员） | `get_settings`、`update_settings`、`refresh_settings`                                                               |

邮箱修改的发起和确认绑定同一 OAuth 授权；另一个客户端或同一用户的另一份授权不能确认。轮换访问令牌保留这项绑定。API Key 或客户端密钥仅在创建／轮换工具结果中返回一次，列表不包含密钥。

请求体上限 1 MiB。携带 Origin 时必须与 `AC_PUBLIC_URL` 相同。有效授权下 GET 和 DELETE 返回 405；没有服务器推送流或会话删除接口。服务无需额外启动进程，沿用 AuthCenter 的 HTTPS 地址和 configcenter 配置。

协议参考：[MCP 授权规范](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization)、[官方 Go SDK](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.8.0)。
