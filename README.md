# AuthCenter

从 configcenter 派生的 OAuth 2.0 授权中心。Go + Gin + GORM，Vue 3 管理界面，支持 SQLite 和 MySQL。

- GitHub OAuth 登录；账号密码 + TOTP 身份验证器或 Resend 邮箱验证码。
- 登录用户可注册应用，上传 Logo，填写名称、主页和多个回调地址，并在后续编辑。
- 授权码、S256 PKCE、用户同意页面、用户信息、刷新令牌轮换及撤销。
- 普通 OAuth 2.0 接入：无需 `nonce`、issuer 校验或 `id_token`。机密客户端可不使用 PKCE；公开客户端必须使用 PKCE。
- 所有运行配置由 configcenter Go SDK 加载，中英文界面、共享原生对话框和危险操作确认。

## 启动

先在 configcenter 创建 authcenter 配置项目及读取密钥，将运行参数写入对应环境。最小配置和完整配置表见 [配置文档](docs/configuration.md)。

本地 `.env.local` 只需要配置中心的连接信息（不提交）：

```dotenv
AC_CONFIGCENTER_URL=https://config.example.com
AC_CONFIGCENTER_API_KEY=cc_your_project_key
AC_CONFIGCENTER_ENVIRONMENT=production
```

```sh
npm ci
make check
make build
./.tmp/bin/authcenter
```

默认监听 `127.0.0.1:8081`，默认 SQLite 路径为 `.tmp/authcenter.db`。生产环境在配置中心设置 HTTPS 公网地址和独立 MySQL 数据库。首次密码管理员使用 `AC_BOOTSTRAP_ADMIN_EMAIL` 指定的邮箱注册，通过验证码验证后获得管理员身份。首次配置需开启注册并配置 Resend；GitHub 登录使用上游已验证的邮箱匹配预置管理员。

服务启动时读取一次配置中心快照；修改运行配置后重启服务。连接失败会阻止启动。环境变量中的 `AC_ADDR`、GitHub 密钥、Resend 密钥等业务配置不会覆盖配置中心。

## 文档

- [OAuth 接入与管理 API](docs/api.md)
- [configcenter SDK 与运行配置](docs/configuration.md)
- [登录与安全行为](docs/security.md)

`make check` 包含 Go lint、Go 测试、Vue 类型检查、模板 lint、Vitest、生产资源构建及 configcenter SDK 检查。`make build` 输出 `.tmp/bin/authcenter`。测试使用内存数据库和模拟身份提供方、配置中心及 Resend，不依赖真实外部凭据。

## 共享基础设施

本项目通过 `common.lock.json` 固定 center-common 的提交，公共源码与 CI 步骤均来自同一个版本。首次检出先执行 `make bootstrap`，再运行 `make check build`。私有依赖使用本机 Git 凭据；CI 配置只读 `COMMON_REPO_SSH_KEY` deploy key（或 `COMMON_REPO_TOKEN`）secret。

本地同时开发四个独立仓库时，执行 `make common-local COMMON_DIR=../center-common && make bootstrap`。三个项目的 `.tmp/common` 会直接引用同一份公共源码，修改无需复制。共享 Vue 控件在开发服务器热更新，Go 修改需重新构建。可复现部署使用 `CI=true make bootstrap check build`；该模式忽略本地覆盖并拉取锁定提交。

升级时先提交并发布 common，再更新本项目锁文件和必要的适配；回滚恢复原锁文件与适配。公共能力、工具链和维护流程见 [center-common README](https://github.com/OXeu/center-common#readme)。业务页面、表单、权限和部署凭据仍在本仓库维护。
