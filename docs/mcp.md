# MCP 接口

[文档首页](/docs/) · [HTTP API](/docs/api) · [Go SDK](/docs/sdk) · [Markdown 原文](/docs/mcp.md)

## 地址与认证

向 `POST /mcp/:project` 发送 JSON-RPC 2.0 请求，`:project` 是项目 slug，需要按 URL 路径段编码。请求头为：

```http
Authorization: ApiKey cc_REPLACE_WITH_YOUR_KEY
Content-Type: application/json
Accept: application/json
```

所有 API Key 使用统一的 scope 模型。路径中的项目必须被 Key 的某项 scope 覆盖，工具执行还需要该项目下的操作权限；`project: "*"` 覆盖所有项目。单项目、多项目和全部项目的 Key 都必须在 MCP 路径中指定目标项目，每项授权覆盖项目内的全部环境。详见 [scope 配置与权限表](/docs/api)。

浏览器会话也可调用读取工具，但必须携带 `X-CSRF-Token` 且具备该项目的访问权限；写入工具要求 API Key。外部 Agent 使用 API Key，不需要浏览器 Cookie 或 CSRF Token。完整密钥只在创建或轮换时显示，吊销、过期或轮换后的旧密钥立即失效。

## 连接与初始化

当前端点逐次接收 HTTP POST 并返回 JSON，没有 SSE 推送、会话 ID、GET 事件流、DELETE 会话关闭或 JSON-RPC 批处理。客户端需支持这种无状态调用方式。服务端返回的协议版本为 `2025-06-18`；初始化请求中的版本不会改变服务端版本。

示例中的 `CC_URL`（无尾部斜杠）和 `CC_KEY` 由调用方安全提供，不是新增的服务端环境配置。将 `billing` 替换为实际项目 slug：

```sh
curl --fail-with-body --silent --show-error \
  -H "Authorization: ApiKey $CC_KEY" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json' \
  "$CC_URL/mcp/billing" \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"config-agent","version":"1.0.0"}}}'
```

响应：

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-06-18",
    "capabilities": { "tools": { "listChanged": false } },
    "serverInfo": { "name": "configcenter", "version": "1.0.0" }
  }
}
```

随后可发送 `{"jsonrpc":"2.0","method":"notifications/initialized"}`。合法且不带 `id` 的请求作为通知处理，返回 HTTP 202、无响应体，也不执行工具。需要结果或实际执行写入时必须携带 `id`。

## 方法与工具发现

| 方法                             | 参数                | 结果                                                |
| -------------------------------- | ------------------- | --------------------------------------------------- |
| `initialize`                     | 见上例              | 协议版本、能力和服务端信息                          |
| `ping`                           | 无                  | 空对象                                              |
| `tools/list`                     | 无                  | 当前凭据可用的 `tools` 数组及各工具的 `inputSchema` |
| `tools/call`                     | `name`、`arguments` | 工具执行结果                                        |
| `resources/list`、`prompts/list` | 无                  | 空列表；当前未提供资源或提示词                      |

```json
{ "jsonrpc": "2.0", "id": 2, "method": "tools/list" }
```

所有参数均位于 `params.arguments` 中，以下字段均为字符串。工具 schema 声明 `additionalProperties: false`；客户端应只发送列出的字段。

| 工具                | 必填参数                       | 可选参数          | 权限与行为                                                |
| ------------------- | ------------------------------ | ----------------- | --------------------------------------------------------- |
| `list_environments` | 无                             | 无                | `environments:read`；返回项目 slug 和环境的 slug、名称    |
| `list_config`       | `environment`                  | `prefix`          | `config:read`；按配置键前缀过滤，返回环境 slug 和配置数组 |
| `get_config`        | `environment`、`key`           | 无                | `config:read`；读取单个配置项                             |
| `list_proposals`    | 无                             | 无                | `proposals:read`；读取当前项目待审核提案                  |
| `set_config`        | `environment`、`key`、`value`  | `description`     | `config:write`；立即创建或更新配置                        |
| `delete_config`     | `environment`、`key`           | 无                | `config:delete`；立即删除配置                             |
| `propose_config`    | `environment`、`key`、`action` | `value`、`reason` | `proposals:write`；创建待审核提案，不立即修改配置         |

`tools/list` 只返回当前项目 scope 允许调用的工具；直接调用未授权工具也会被拒绝。例如 `config:read` 只显示 `list_config` 与 `get_config`，`config:write` 只显示 `set_config`，不会自动包含读取或删除。不同项目的权限分别计算，不会合并成跨项目授权。

MCP 不提供项目、环境或 Key 的管理工具，这些操作使用管理界面或 [HTTP API](/docs/api)。旧 viewer / editor / 全局 Key 的 token 可继续使用，服务启动时会将原权限迁移为对应 scopes。

## 读取与写入示例

按前缀读取：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "list_config",
    "arguments": { "environment": "production", "prefix": "db." }
  }
}
```

`result.content` 是文本内容数组；其中 `text` 是序列化后的 JSON 字符串，调用方需要再解析一次：

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      { "type": "text", "text": "{\n  \"environment\": \"production\",\n  \"items\": []\n}" }
    ],
    "isError": false
  }
}
```

配置项包含 `key`、`value`、`description`、`updated_at`。`value` 始终是字符串，传数字、布尔值或 JSON 时自行序列化。配置键最长 191 字节，具体字符限制与管理界面一致。

立即写入：

```json
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "set_config",
    "arguments": {
      "environment": "production",
      "key": "db.pool",
      "value": "20",
      "description": "数据库连接池大小"
    }
  }
}
```

`set_config` 成功返回配置项；省略 `description` 会将描述设为空字符串。`delete_config` 使用相同的环境和键参数，成功返回 `{"deleted":"db.pool"}`。

## 变更提案

```json
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "propose_config",
    "arguments": {
      "environment": "production",
      "key": "db.pool",
      "action": "set",
      "value": "30",
      "reason": "提高并发连接数，等待审核"
    }
  }
}
```

`action` 只能为 `set` 或 `delete`。`set` 的 `value` 省略时为空字符串；删除提案不需要 `value`。成功结果包含 `id`、`key`、`action` 和 `status: "pending"`。项目的「变更提案」页可批准或拒绝；批准后应用配置变更，拒绝则保留原配置。MCP 不提供审核工具。

`list_proposals` 返回 `proposals` 数组，字段包括 `id`、`environment_id`、`key`、`action`、`value`、`reason`、`status`。每次 `propose_config` 都会创建新提案，没有幂等键；网络结果不确定时先查待审核列表，避免盲目重试产生重复提案。

## 错误处理

请求体上限为 1 MiB。依次检查 HTTP 状态、顶层 `error` 和 `result.isError`，HTTP 200 并不代表工具成功。

| 情况                                                        | 响应                                                        |
| ----------------------------------------------------------- | ----------------------------------------------------------- |
| 密钥缺失、错误、过期、已吊销，或目标项目不存在              | HTTP 401，JSON `error`                                      |
| 项目 Key 访问其他已存在项目                                 | HTTP 403，JSON `error`                                      |
| 无法解析 JSON，包括请求体超限                               | HTTP 400，JSON-RPC `error.code = -32700`                    |
| 非法 JSON-RPC 版本或缺少方法                                | HTTP 200，`error.code = -32600`                             |
| 未知方法或工具名                                            | HTTP 200，`error.code = -32601`                             |
| 工具调用参数类型无法解析                                    | HTTP 200，`error.code = -32602`                             |
| 结果编码失败                                                | HTTP 200，`error.code = -32603`                             |
| 环境不存在、键不存在、缺少工具所需 scope 或其他工具执行失败 | HTTP 200，`result.isError = true`，`content` 中提供错误文本 |

设置超时并保留请求 ID 以匹配响应。密钥、配置值和提案内容不应写入诊断日志。只对适合重试的操作做有界重试；认证或参数错误需先修正请求。
