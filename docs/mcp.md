# MCP 服务

AuthCenter 在 `/mcp` 提供 Streamable HTTP MCP 服务。MCP 与 `/api/v1` 使用同一份操作清单、处理函数、角色与资源归属检查，覆盖账户安全、应用、授权、用户、系统设置和 API Key 管理。协议由官方 Go SDK 处理，支持 `2026-07-28` 及其兼容版本；使用无状态 JSON 响应，不创建 MCP 会话 ID。

## OAuth 接入

1. 在 Cursor 等 MCP 客户端配置服务 URL `https://auth.example/mcp`，然后点击认证。客户端通过受保护资源元数据和授权服务器元数据发现端点，再向 `POST /oauth/register` 动态注册，无需手动创建应用或配置客户端密钥。
2. 动态注册接受 `client_name`、`redirect_uris`、`grant_types`、`response_types`、`token_endpoint_auth_method` 和 `scope`。`redirect_uris` 需要 1–20 个字符串；不对 URI 内容做格式、协议、域名或路径校验，完整保留客户端提交的列表。Cursor 的 `cursor://anysphere.cursor-mcp/oauth/callback`、其他自定义协议和非本机 HTTP 地址均可注册。回调是否能被客户端接收由客户端负责。
3. 动态客户端使用 `token_endpoint_auth_method=none`，支持 `authorization_code` 和可选 `refresh_token`，响应类型为 `code`。授权范围限定为 `mcp` 和可选 `offline_access`；注册响应返回实际支持的范围。注册端点限制请求体为 16 KiB，每个来源 IP 每分钟最多成功注册 30 次。
4. 客户端通过浏览器发起授权码请求，必须使用 S256 PKCE、明确的 `redirect_uri` 和 `resource=https://auth.example/mcp`。省略 scope 时默认 `mcp`；需要刷新令牌时注册 `refresh_token` 并请求 `mcp offline_access`。授权请求必须完整匹配已注册回调之一，换码时还必须与授权码绑定的回调完全相同。
5. 用户登录并确认授权；回调包含 `code`、原始 `state` 和授权服务器 `iss`。客户端核对 `state`、`iss`，再使用同一资源地址和 PKCE verifier 兑换令牌。OAuth 响应中的 `code`、`error`、`state`、`iss` 覆盖回调 URI 内的同名参数，其他查询参数和 fragment 保留。

动态注册示例：

```json
{
  "client_name": "Cursor",
  "redirect_uris": [
    "cursor://anysphere.cursor-mcp/oauth/callback",
    "https://www.cursor.com/agents/mcp/oauth/callback",
    "http://localhost:8787/callback"
  ],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

也可继续在 `/applications` 手动预注册应用，并在客户端配置 `client_id` 和机密客户端的 `client_secret`。手动注册应用的既有 OAuth 流程保持兼容；用于 MCP 时同样需要 PKCE 和资源绑定。动态客户端不在管理页面编辑元数据，管理员可删除，用户可在“我的授权”撤销其授权。

出现 `SSE error: Non-200 status code (401)` 时，先检查动态注册、授权和令牌交换是否成功：未获得有效令牌的 MCP 请求返回 401。旧连接若缓存了缺少注册端点的元数据或失效凭据，需要重新连接并认证。元数据、注册、令牌和撤销端点支持不携带 Cookie 的 CORS 预检。

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
