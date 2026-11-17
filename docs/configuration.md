# configcenter SDK 配置

AuthCenter 使用 `github.com/OXeu/configcenter/sdk/configcenter` 读取配置。SDK 源码随仓库保存在 `sdk/configcenter`，通过本地 Go module replace 引用，与原 configcenter SDK 的包名和协议一致，不依赖旁边的源码目录。

## 引导配置

只有下列连接参数从环境或可选的 `.env.local` 读取；已导出的环境变量优先。`AC_ENV_FILE` 可指定另一个引导文件。

```dotenv
AC_CONFIGCENTER_URL=https://config.example.com
AC_CONFIGCENTER_API_KEY=cc_project_scoped_key
AC_CONFIGCENTER_ENVIRONMENT=production
```

SDK 的读取、单项写入和批量写入统一使用 `Authorization: Bearer <API_KEY>`；`AC_CONFIGCENTER_API_KEY` 仍填写原有 `cc_` 密钥值，无需添加 `Bearer` 前缀或更换密钥。

配置中心 API 密钥至少需要目标项目的配置读取权限。请使用 authcenter 专属配置项目和权限。服务启动时读取快照，读取失败直接退出。现有应用数据存储在 authcenter 数据库中。

管理员可在用户菜单进入独立的“后台设置”页面 `/settings`。页面展示配置中心中的值、是否待应用及生效方式，支持逐项或批量编辑。写入需要连接密钥具有 `projects:read`、`environments:read` 和 `config:write` 权限。数据库连接、加密密钥和 API 密钥仅显示是否配置，不返回已有值；批量编辑时秘密字段留空保留原值，单独编辑秘密字段时留空表示清除。

保存只写入 configcenter；点击“刷新配置”后绕过 SDK 缓存读取并校验新快照，应用到当前实例。读取、校验或 OAuth 提供方初始化失败时继续使用原快照。多实例部署需分别刷新或统一重启。监听地址、数据库连接、加密密钥和首次管理员设置需要重启，刷新响应会提示这些待应用项；没有有效运行中加密密钥时，启用密码登录也需重启。首次管理员设置只在空数据库初始化时使用。

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
| `AC_OIDC_SIGNING_KEYS`     | 可选；Base64 编码的 RSA 私钥 PEM 集合，配置后启用 [OpenID Connect](oidc.md) |
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

SCF 统一使用北京时间（`TZ=Asia/Shanghai`）、64MB 内存、单实例静态 100 并发和 1 秒请求队列缓冲。SCF 组件不透传 `IntranetConfig`，部署脚本会在更新函数后调用 `tools/scf-network.py authcenter` 启用固定内网出口 IP，并等待配置生效。本地部署需要 Python 3 及 venv；脚本在 `.tmp/scf-venv` 安装固定版本的腾讯云 SCF SDK。直接执行 `scf deploy` 时也必须补跑该网络配置步骤。

## 本地部署与 GitHub Actions

`AC_CONFIGCENTER_API_KEY` 是连接密钥的标准变量名；旧名 `AC_CONFIGCENTER_KEY` 仅作为兼容别名，两个值同时存在时标准名优先。已废弃的 `AC_SESSION_SECRET` 不再使用。

本地 `.env` 中填写配置中心连接信息，以及 `.env.example` 列出的腾讯云凭据、地域、VPC 和子网参数，然后运行：

```sh
make check
npm run deploy:scf
SCF_PUBLIC_URL=https://YOUR_FUNCTION.ap-guangzhou.tencentscf.com npm run verify:scf
```

部署会构建 Linux 二进制，并使用固定版本的 `serverless-cloud-framework` CLI。函数包只包含二进制、启动脚本和配置中心引导文件；腾讯云管理凭据不会被打包。部署产物在完成后自动清理，失败日志保存在仅当前用户可读的 `.tmp/scf-deploy.log`。

GitHub 仓库需要以下配置：

| 类别     | 名称                                          | 内容                                                     |
| -------- | --------------------------------------------- | -------------------------------------------------------- |
| Secret   | `TENCENT_SECRET_ID` / `TENCENT_SECRET_KEY`    | 腾讯云部署凭据                                           |
| Secret   | `SCF_RUNTIME_ENV`                             | 三项 `AC_CONFIGCENTER_*` 引导参数组成的 dotenv 文本      |
| Variable | `SCF_REGION` / `SCF_VPC_ID` / `SCF_SUBNET_ID` | 部署网络                                                 |
| Variable | `SCF_DEPLOY_ENABLED`                          | `true` 启用 main 分支部署                                |
| Variable | `SCF_PUBLIC_URL`                              | 已部署的 HTTPS 地址，须与配置中心的 `AC_PUBLIC_URL` 一致 |

推送 main 或手动触发 `CI and deploy` 后，检查任务完成才会部署。部署任务使用与本地相同的脚本，并验证登录页及 OAuth 元数据。自定义域名需先配置 HTTPS 绑定；尚未绑定时可使用 SCF 原生 HTTPS 地址。
