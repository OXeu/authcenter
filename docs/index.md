# Configcenter 接入文档

将配置集中存储在项目的不同环境中，通过 HTTP API 或 Go SDK 读取。

- [HTTP API 接入](/docs/api)：鉴权、环境发现、读取配置与错误处理。
- [Go SDK 接入](/docs/sdk)：初始化、类型转换、快照和自动刷新。
- Markdown 原文：[首页](/docs/index.md)、[API](/docs/api.md)、[SDK](/docs/sdk.md)。Agent 可直接 GET 这些地址，无需登录或执行 JavaScript。

## 准备工作

1. 部署并启动 Configcenter，通过 OAuth 登录管理界面。
2. 创建项目（自动创建 `production` 环境），在目标环境中添加配置。
3. 在项目 API Keys 页面创建密钥，读取配置使用 viewer 权限即可。完整 token 只在创建时返回，请保存到应用的秘密配置中。
4. 将服务根地址、密钥和环境 slug 传给调用方。项目由密钥决定，不需要传项目名。

以下示例使用 `https://config.example.com`、`production` 环境与 `db.pool` 配置项。替换为真实部署信息。生产调用使用 HTTPS，不要把密钥放进前端代码、URL、日志或版本库。

## 选择接入方式

任意语言可使用只读 REST API；Go 应用可使用独立 SDK 模块。SDK 不提供配置写入、密钥管理或项目管理接口。

需要修改配置时使用管理界面；Agent 自动化可使用 `POST /mcp/:project`，携带同样的 `Authorization: ApiKey <token>`。MCP 是 JSON-RPC 2.0，支持读取、直接修改及提交变更提案；写入受角色限制，提案由人工审核。不要向只读 REST 路径发送 PUT 或 POST。
