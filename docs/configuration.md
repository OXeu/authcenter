# configcenter SDK 配置

AuthCenter 使用 `github.com/OXeu/configcenter/sdk/configcenter` 读取配置。SDK 源码随仓库保存在 `sdk/configcenter`，通过本地 Go module replace 引用，与原 configcenter SDK 的包名和协议一致，不依赖旁边的源码目录。

## 引导配置

只有下列连接参数从环境或可选的 `.env.local` 读取；已导出的环境变量优先。`AC_ENV_FILE` 可指定另一个引导文件。

```dotenv
AC_CONFIGCENTER_URL=https://config.example.com
AC_CONFIGCENTER_KEY=cc_project_scoped_key
AC_CONFIGCENTER_ENVIRONMENT=production
```

配置中心 API 密钥至少需要目标项目的配置读取权限。请使用 authcenter 专属配置项目和权限。服务启动时读取快照，读取失败直接退出。调整配置后重启，现有应用数据存储在 authcenter 数据库中。

## 配置中心中的键

| 键                         | 默认值 / 用途                                                                  |
| -------------------------- | ------------------------------------------------------------------------------ |
| `AC_ADDR`                  | `127.0.0.1:8081`                                                               |
| `AC_PUBLIC_URL`            | `http://127.0.0.1:8081`；生产必须 HTTPS origin，不带路径                       |
| `AC_ENV`                   | 设置 `production` 启用生产配置检查                                             |
| `AC_DB_DRIVER`             | `sqlite` 或 `mysql`                                                            |
| `AC_DB`                    | `file:.tmp/authcenter.db`；MySQL 时为独立库 DSN                                |
| `AC_LANGUAGES`             | `en,zh-CN`                                                                     |
| `AC_LOCAL_LOGIN`           | `true`；开启密码 + 两步验证                                                    |
| `AC_ALLOW_REGISTRATION`    | `true`；控制新账号注册                                                         |
| `AC_ENCRYPTION_KEY`        | 开启本地登录时必填，32 个随机字节的标准 Base64；用于 TOTP 加密和邮箱验证码摘要 |
| `AC_RESEND_API_KEY`        | Resend 发信 API Key                                                            |
| `AC_RESEND_FROM`           | 已验证发信域的发件人，如 `AuthCenter <auth@example.com>`                       |
| `AC_OAUTH_ISSUER`          | `https://github.com/login/oauth`                                               |
| `AC_OAUTH_CLIENT_ID`       | GitHub OAuth App Client ID                                                     |
| `AC_OAUTH_CLIENT_SECRET`   | GitHub OAuth App Client Secret                                                 |
| `AC_OAUTH_SCOPES`          | `read:user user:email`                                                         |
| `AC_OAUTH_REDIRECT_URL`    | 默认公网地址 + `/oauth/callback`                                               |
| `AC_BOOTSTRAP_ADMIN_EMAIL` | `admin@localhost`；首次创建管理员的邮箱                                        |
| `AC_BOOTSTRAP_ADMIN_NAME`  | `admin`                                                                        |
| `AC_OAUTH_ADMIN_GROUPS`    | `authcenter-admin`；仅用于另行配置的支持 groups 的上游                         |
| `AC_OAUTH_GROUP_MAP`       | 可选 `group:role` 映射                                                         |
| `AC_OAUTH_GROUPS_CLAIM`    | `groups`                                                                       |

示例配置（写入 configcenter，不是本地环境文件）：

```dotenv
AC_PUBLIC_URL=https://auth.example.com
AC_ADDR=0.0.0.0:8081
AC_ENV=production
AC_DB_DRIVER=mysql
AC_DB=authcenter:YOUR_PASSWORD@tcp(mysql:3306)/authcenter?charset=utf8mb4
AC_LOCAL_LOGIN=true
AC_ALLOW_REGISTRATION=true
AC_ENCRYPTION_KEY=BASE64_ENCODED_32_RANDOM_BYTES
AC_RESEND_API_KEY=re_YOUR_KEY
AC_RESEND_FROM=AuthCenter <auth@example.com>
AC_OAUTH_CLIENT_ID=YOUR_GITHUB_CLIENT_ID
AC_OAUTH_CLIENT_SECRET=YOUR_GITHUB_CLIENT_SECRET
AC_BOOTSTRAP_ADMIN_EMAIL=admin@example.com
```

在 GitHub OAuth App 中设置 AuthCenter 主页及 `/oauth/callback`。GitHub 登录与下游业务应用注册互相独立。邮箱验证码需要配置 Resend API Key 和已验证的发件域；未配置 Resend 时，已经绑定 TOTP 的本地账号仍可使用 TOTP 登录，新密码账号无法注册。

加密密钥应持久保存，所有实例使用相同值。直接替换会使已绑定的 TOTP 密钥无法解密。生产共享数据库与此密钥，不使用临时 SQLite 文件。初始管理员邮箱应使用实际可收信的地址。

腾讯 SCF 部署沿用仓库的打包流程；在配置中心将 `AC_ADDR` 设置为 `0.0.0.0:9000`，使用 MySQL 和 HTTPS 公网地址。部署文件仅包含配置中心引导参数，SCF 不覆盖运行配置。
