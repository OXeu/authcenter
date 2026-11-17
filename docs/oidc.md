# OpenID Connect

AuthCenter 支持 OpenID Connect 授权码流程。配置签名密钥后，业务应用可请求 `openid profile email`，通过签名 ID Token 验证用户身份。[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html) 和 [Discovery](https://openid.net/specs/openid-connect-discovery-1_0.html) 定义了客户端的验证要求。

## 启用与密钥管理

1. 在仓库根目录生成 RSA 私钥；下面的命令将临时文件限制在 `.tmp/`，不向终端输出密钥。

   ```sh
   mkdir -p .tmp/oidc
   chmod 700 .tmp/oidc
   umask 077
   openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:3072 -out .tmp/oidc/signing.pem
   openssl base64 -A -in .tmp/oidc/signing.pem -out .tmp/oidc/signing.b64
   ```

2. 将 `signing.b64` 的完整内容写入 ConfigCenter 中 AuthCenter 项目的 `AC_OIDC_SIGNING_KEYS`，并妥善备份。此设置是密钥字段，管理 API 不返回原值；它不是本地环境变量。
3. 刷新 AuthCenter 配置或重启；多实例需要分别刷新或统一重启。确认 `/.well-known/openid-configuration` 包含 `openid`、`jwks_uri` 和 `RS256`，再删除已安全保存的临时密钥文件。

签名密钥支持标准 Base64 编码的 PEM 集合，包含一至五个 2048–8192 位 RSA 私钥，格式为 PKCS1 或 PKCS8。第一个密钥用于签发，其余密钥的公钥保留在 JWKS 中。所有实例必须使用同一份配置；不要在每次启动时重新生成密钥。

轮换流程：

1. 将新密钥追加到集合并刷新所有实例，让客户端有机会缓存新公钥。
2. 等待客户端的 JWKS 缓存刷新后，将新密钥移至首位并再次刷新。
3. 旧密钥至少保留到最后签发的 ID Token 过期，并额外考虑客户端允许的时钟偏差和实例切换时间。

未配置密钥时，普通 OAuth 和 MCP 继续可用，发现文档不声明 `openid`，请求该 scope 返回 `invalid_scope`。无效密钥会使配置校验失败；保存或刷新失败时保留当前有效配置。

## 应用接入

在“应用”中登记客户端和回调地址。升级时会给已有手动注册的应用补充 `openid` scope，保留其客户端 ID、密钥、回调和禁用状态；既有授权不会因此获得新的 scope。MCP 动态注册不会获得 OpenID 权限。

授权请求示例：

```http
GET /oauth/authorize?response_type=code&client_id=CLIENT_ID&redirect_uri=https%3A%2F%2Fapp.example%2Fcallback&scope=openid%20profile%20email&state=RANDOM_STATE&nonce=RANDOM_NONCE&code_challenge=CHALLENGE&code_challenge_method=S256
```

- 授权端点接受 GET 查询参数和 POST 表单参数；用户确认仍通过受会话及 CSRF 保护的授权页面完成。
- `response_type=code` 和 `redirect_uri` 必须明确提供，回调必须完整匹配登记值。OIDC 回调接受 HTTPS、本机 HTTP，以及公开客户端的绝对自定义协议 URI；不接受 fragment、用户凭据或相对地址。
- `nonce` 可选，建议使用随机值；传入时必须是非空字符串且不超过 1024 字节。它绑定到授权请求，并原样出现在对应 ID Token 中。
- 公开客户端必须使用 S256 PKCE。机密客户端可选用；一旦发送 challenge，兑换时必须提供匹配的 verifier。
- 换码时必须再次提供原始 `redirect_uri`。授权码为单次使用，客户端、回调、PKCE 或资源不匹配时不能兑换。

成功换码返回 `access_token` 和 `id_token`。ID Token 使用 RS256，携带 `kid`，有效期为五分钟，包含 `iss`、`sub`、`aud`、`iat`、`exp` 和 `at_hash`；发送过 nonce 时还包含 `nonce`。`profile` 授权姓名和头像，`email` 授权邮箱及真实的 `email_verified` 状态。

客户端应使用发现文档中的 `/oauth/jwks` 公钥验证签名，并检查 issuer、audience、有效期和原始 nonce。`GET` 或 `POST /oauth/userinfo` 接受 Bearer access token，返回相同的稳定 `sub` 和 scope 允许的资料。JWKS 与发现端点支持匿名访问和 CORS，JWKS 只公开 RSA 公钥参数。

## 支持范围

- 请求不含 `openid` 时继续使用普通 OAuth，不签发 ID Token。
- `offline_access` 经用户确认后授予刷新令牌；刷新保留原 scope 并轮换令牌，不返回新的 ID Token。
- `prompt=consent` 使用正常确认流程。`prompt=none` 不显示交互页面：未登录时返回 `login_required`，已登录但仍需确认时返回 `consent_required`。
- 当前不支持 implicit、hybrid、`prompt=login`、`prompt=select_account`、`max_age`、`claims`、`id_token_hint`、Request Object 或 OIDC 动态注册；相应请求会被拒绝。
- `/oauth/register` 仍是 MCP 动态注册端点，仅在 OAuth 授权服务器元数据中公布。`openid` 不与 `mcp` 或资源绑定授权混用。
