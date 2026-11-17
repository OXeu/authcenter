# AuthCenter

AuthCenter 为业务应用提供 OAuth 2.0 授权服务。用户通过 GitHub，或账号密码加 TOTP / 邮箱验证码登录，然后在授权页面决定是否向应用提供姓名、头像和邮箱。

注册应用时填写名称、主页、一个或多个完整回调地址，并上传 PNG 或 JPEG Logo。上述资料可以修改；客户端类型及 ID 保持稳定，密钥只在创建时展示一次。普通用户管理自己的应用，管理员可管理所有应用。

机密客户端通过授权码和客户端密钥兑换令牌。公开客户端不持有密钥，必须使用 S256 PKCE。服务支持刷新和撤销令牌，并提供用户信息接口。

AuthCenter 支持普通 OAuth 2.0 和 [OpenID Connect 授权码流程](oidc.md)。配置签名密钥并请求 `openid` 后可获得签名 ID Token；普通 OAuth 请求保持原有行为。回调地址须在应用登记列表内，OIDC 请求必须明确提供回调。

## 下一步

- [接入文档](/docs/api)：授权、令牌、用户信息及应用管理 API。
- [MCP 接入](/docs/mcp)：通过 OAuth 授权调用与管理 API 等价的工具。
- [配置](/docs/configuration)：通过 configcenter SDK 配置 GitHub、Resend 和数据库。
- [安全行为](/docs/security)：两步验证、同意、重放保护和权限边界。
