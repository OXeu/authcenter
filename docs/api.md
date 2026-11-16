# HTTP API 接入

[文档首页](/docs/) · [Go SDK](/docs/sdk) · [Markdown 原文](/docs/api.md)

## 鉴权与项目隔离

每次请求携带 `Authorization: ApiKey cc_…`，注意认证方案是 `ApiKey`。密钥绑定一个项目，以下路径没有项目参数。viewer 密钥即可读取该项目的所有环境；密钥不是按环境隔离的。

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

| HTTP 状态 | 处理方式 |
| --- | --- |
| 200 | 解析 JSON 并更新本地快照 |
| 401 | 检查认证头、密钥是否有效、过期或被删除；修复凭据后重试 |
| 404 | 检查环境 slug 是否存在 |
| 500 | 服务端故障；记录状态码，采用有界退避重试 |

设置请求超时；首次加载失败时由应用决定停止启动或使用明确默认值。刷新失败可保留上一次成功快照，但应监测其过期时间。避免将配置值或完整密钥写入诊断日志。

## 与管理 API 的区别

`/api/v1/projects/...` 等管理接口使用浏览器会话，修改还需要 CSRF 校验及写权限；不能用项目 API Key 代替浏览器会话。程序读取应使用本文的 `/api/v1/agent/...` 接口。
