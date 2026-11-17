# HTTP API 接入

[文档首页](/docs/) · [Go SDK](/docs/sdk) · [Markdown 原文](/docs/api.md)

## 鉴权与项目隔离

每次请求携带 `Authorization: ApiKey cc_…`，注意认证方案是 `ApiKey`。项目密钥绑定一个项目，以下读取路径无需项目参数。viewer 密钥即可读取该项目的所有环境；密钥不是按环境隔离的。全局密钥可跨项目使用，调用 `/api/v1/agent/...` 时必须通过 `?project=billing` 指定项目，否则返回 HTTP 400。

以下命令假设调用环境已安全提供 `CC_URL`（服务根地址，无尾部斜杠）和 `CC_KEY`（完整密钥）。这些是调用方示例变量，不是服务器配置项。

## 查询项目和环境

```sh
curl --fail-with-body --silent --show-error \
  -H "Authorization: ApiKey $CC_KEY" \
  "$CC_URL/api/v1/agent/project"
```

成功返回 HTTP 200，JSON 包含 `project` 对象和 `environments` 数组。使用环境对象的 `slug` 读取配置，不使用展示名称或数字 ID。

## 读取配置

```sh
curl --fail-with-body --silent --show-error \
  -H "Authorization: ApiKey $CC_KEY" \
  "$CC_URL/api/v1/agent/environments/production/values"
```

响应示例：

```json
{
  "project": "billing",
  "environment": "production",
  "items": [
    {
      "key": "db.pool",
      "value": "20",
      "description": "数据库连接池大小",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

`value` 始终是字符串，数字、布尔值或 JSON 由调用方解析。没有匹配项时 `items` 是空数组。可通过 `prefix` 限制返回的键：

```sh
curl --fail-with-body --silent --show-error --get \
  -H "Authorization: ApiKey $CC_KEY" \
  --data-urlencode 'prefix=db.' \
  "$CC_URL/api/v1/agent/environments/production/values"
```

环境 slug 作为路径段时需要 URL 编码，prefix 作为查询参数编码。此接口没有分页或推送订阅，调用方可定时重新读取。

## 错误与重试

错误响应为 JSON，包含 `error` 字段。不要依赖错误文案做逻辑判断。

| HTTP 状态 | 处理方式                                               |
| --------- | ------------------------------------------------------ |
| 200       | 解析 JSON 并更新本地快照                               |
| 401       | 检查认证头、密钥是否有效、过期或被删除；修复凭据后重试 |
| 404       | 检查环境 slug 是否存在                                 |
| 500       | 服务端故障；记录状态码，采用有界退避重试               |

设置请求超时；首次加载失败时由应用决定停止启动或使用明确默认值。刷新失败可保留上一次成功快照，但应监测其过期时间。避免将配置值或完整密钥写入诊断日志。

## 全局 API Key 管理接口

管理员登录后，在「全局密钥」页面 `/keys` 创建密钥。全局 Key 具有所有项目的管理权限，可调用下表中的接口；项目 Key 仍只能使用项目范围内的 Agent REST / MCP 接口，不能调用这些管理接口。全局 Key 也可访问 `POST /mcp/:project`，项目由路径指定。

使用 `Authorization: ApiKey <token>` 鉴权时不需要 Cookie 或 CSRF Token；显式提供认证头后不会回退到浏览器会话。使用浏览器会话时，修改请求仍需要 `X-CSRF-Token` 和写权限。

| 方法 | 路径 | 请求体 / 行为 |
| --- | --- | --- |
| GET | `/api/v1/projects` | 返回 `projects`；默认列出所有项目，`?view=mine` 只列出密钥创建者拥有的项目 |
| POST | `/api/v1/projects` | `slug`、`name`、可选 `description`；自动创建 `production` 环境，所有者为密钥创建者 |
| PATCH | `/api/v1/projects/:project` | `name`、`description`；更新名称和描述，不能更改 slug |
| DELETE | `/api/v1/projects/:project` | 删除项目及其配置、环境、项目 Key 和提案 |
| GET / POST | `/api/v1/projects/:project/environments` | 读取环境，或用 `slug`、`name` 创建环境 |
| DELETE | `/api/v1/projects/:project/environments/:env` | 删除环境及其内容 |
| GET | `/api/v1/projects/:project/environments/:env/items` | 读取配置，支持 `?prefix=` |
| PUT | `/api/v1/projects/:project/environments/:env/items/:key` | `value`（必填字符串）、可选 `description`；创建或覆盖配置 |
| DELETE | `/api/v1/projects/:project/environments/:env/items/:key` | 删除配置 |
| GET / POST | `/api/v1/projects/:project/keys` | 列出项目 Key，或用 `name`、`role`（viewer / editor）、可选 `expires_at` 创建 |
| POST | `/api/v1/projects/:project/keys/:id/rotate` | 轮换项目 Key，返回一次性可见的 `token` |
| DELETE | `/api/v1/projects/:project/keys/:id` | 吊销项目 Key |
| GET | `/api/v1/projects/:project/proposals` | 列出提案，支持 `?status=pending` |
| POST | `/api/v1/projects/:project/proposals/:id/review` | `approve` 布尔值；批准或拒绝提案 |

路径中的 project、env 使用 slug，key 和各路径段需 URL 编码。新建项目、环境、密钥返回 HTTP 201；读取、更新、写入配置和轮换返回 HTTP 200；删除返回 HTTP 204。缺失或失效凭据返回 401，项目 Key 调用管理接口返回 403，参数错误返回 400，资源不存在返回 404。

```sh
curl --fail-with-body --silent --show-error \
  -H "Authorization: ApiKey $CC_KEY" \
  -H 'Content-Type: application/json' \
  -X PUT "$CC_URL/api/v1/projects/billing/environments/production/items/db.pool" \
  --data '{"value":"20","description":"数据库连接池大小"}'
```

全局 Key 自身的管理接口仅限管理员浏览器会话：`GET /api/v1/keys`、`POST /api/v1/keys`、`POST /api/v1/keys/:id/rotate`、`DELETE /api/v1/keys/:id`。创建请求为 `{"name":"automation"}`，可添加 RFC 3339 格式的 `expires_at`；角色固定为 admin。创建响应为 `{"key":{...},"token":"cc_..."}`，密钥元数据包含 `scope: "global"`。列表不返回完整 token 或摘要。完整 token 仅在创建和轮换时返回，旧 token 在轮换或吊销后失效。

全局 Key 不能管理用户、服务器设置或签发其他全局 Key；这些操作仍要求管理员会话。删除某个项目不会吊销全局 Key，删除密钥创建者的用户账号会撤销该账号创建的全部 Key。
