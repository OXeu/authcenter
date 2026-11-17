# Go SDK 接入

[文档首页](/docs/) · [HTTP API](/docs/api) · [MCP 接口](/docs/mcp) · [Markdown 原文](/docs/sdk.md)

SDK 使用只授权一个具体项目的 API Key，配置读取需要 `config:read`；调用项目环境发现方法还需要 `projects:read` 和 `environments:read`。多项目或 `*` scope 请使用 [HTTP API](/docs/api) 或 [MCP](/docs/mcp)，SDK 不提供选择项目的参数。

## 安装

SDK 是独立 Go 模块，需要 Go 1.26 或更高版本。本地存储使用 [bbolt](https://github.com/etcd-io/bbolt)，dotenv 文件使用 [godotenv](https://github.com/joho/godotenv) 解析。在调用方 Go 模块目录执行：

```sh
go get github.com/OXeu/configcenter/sdk/configcenter
```

生产项目在 go.mod 中固定并审查实际解析的版本。若使用尚未发布的本地修改，可在调用方模块中增加 `replace github.com/OXeu/configcenter/sdk/configcenter => /absolute/path/to/configcenter/sdk/configcenter`。

## 初始化和首次读取

以下函数可放入调用方应用，将地址和密钥从调用方配置层传入：

```go
package integration

import (
    "context"
    "fmt"
    "time"

    "github.com/OXeu/configcenter/sdk/configcenter"
)

func LoadPool(ctx context.Context, baseURL, apiKey string) (int, error) {
    client, err := configcenter.New(configcenter.Config{
        BaseURL: baseURL,
        APIKey: apiKey,
        Environment: "production",
        Timeout: 5 * time.Second,
    })
    if err != nil {
        return 0, fmt.Errorf("create configuration client: %w", err)
    }
    if err := client.Load(ctx); err != nil {
        return 0, fmt.Errorf("load configuration: %w", err)
    }
    return client.Int("db.pool", 10), nil
}
```

`BaseURL` 是服务根地址，不包含 `/api/v1/agent`。`New` 仅校验本地配置，不发请求，也不验证密钥是否有效。`Environment` 为空时使用 `production`。默认请求超时为 30 秒；提供 `HTTPClient` 时使用该客户端，`Timeout` 字段不再生效。

## 多级 fallback 与远程缓存

SDK 按键合并数据源，优先级依次为：

1. ConfigCenter 当前返回的配置。
2. `LocalPath` 指定的 fallback KV：优先使用最近成功读取或写入的远程缓存，其次使用本地配置。
3. godotenv 解析的 dotenv 文件。
4. `Config.Defaults`；仍不存在时使用 `GetDefault`、`Int`、`Bool` 的调用方默认值。

空字符串是已配置的值，不会继续向下查找。`Int`、`Bool` 对选中的值解析失败时直接返回调用方默认值，不再尝试较低优先级的数据源。

```go
client, err := configcenter.New(configcenter.Config{
    BaseURL: baseURL,
    APIKey: apiKey,
    Environment: "production",
    LocalPath: ".tmp/app-config.kv",
    DotenvFiles: []string{".env.local", ".env"},
    Defaults: map[string]string{"db.pool": "10"},
})
if err != nil { return fmt.Errorf("create configuration client: %w", err) }
if err := client.Load(ctx); err != nil {
    logger.Warn("configuration source unavailable", "error", err)
}
pool := client.Int("db.pool", 10)
```

此片段放在持有 `ctx`、`baseURL`、`apiKey` 和 `logger` 的调用方函数中。发生数据源错误时，`Load` 仍合并可用来源并提供快照，同时返回错误，调用方可自行决定是否允许降级启动。所有来源都未提供值且有错误时保留旧快照；context 取消时也保留旧快照。部分失败的快照不会混入旧的内存值，持久化的远程缓存除外。

`BaseURL` 和 `APIKey` 同时为空时跳过远端；仅填其中一个仍报错。远程请求失败不会将写操作转存到本地。

### KV 存储和缓存

`LocalPath` 为空时使用当前工作目录下 `.tmp/configcenter.kv`；可指定固定路径供生产持久化。文件首次读取或写入时创建，权限为 `0600`。每次操作使用 bbolt 事务并关闭文件，不需要额外调用 Close。并发进程竞争文件锁时最多等待一秒，context 截止时间更短则优先使用它。

本地值按 `Environment` 隔离。远程缓存也存放在同一个 KV 文件中，另外按服务地址和 API Key 的哈希隔离，防止不同项目相互读取缓存。复用缓存需要相同的服务地址、API Key、环境和 `LocalPath`；更换密钥或移除远端配置不会读到旧远端的缓存。不同应用应使用不同 `LocalPath`。

远程读取和 `Put` 成功后更新缓存，包括空字符串和配置元数据。远端未返回的键保留最近成功缓存的值；前缀刷新也不会删除其他前缀的缓存。dotenv 和 `Defaults` 不写入 KV。缓存保存失败会返回错误，已读取的远程值或已成功写入的值仍可从当前快照读取。

`LocalDefaults` 是兼容的 KV 初始化选项：在事务中只创建缺失的本地键，已有值不会在重启时被覆盖；配置远端时也会生效。这些值处于 KV 层，高于 dotenv。需要最低优先级且不持久化的默认值时，使用 `Defaults`。

### dotenv 文件

`DotenvFiles` 为 `nil` 时读取当前目录下可选的 `.env`，文件不存在不报错。显式空切片 `[]string{}` 禁用 dotenv；显式文件列表中，前面的文件优先，缺失或无效文件会返回错误，其余来源仍可使用。

每次 `Load` / `LoadPrefix` / Watch 刷新都会重新读取文件。godotenv 支持引号、`export` 和文件内变量展开；键名保持原样，不自动将 `db.pool` 转成 `DB_POOL`。SDK 使用 `godotenv.Read`，不导入或修改进程环境变量；应用需要进程变量时由自己的配置层处理。解析错误不会包含 dotenv 文件内容。

### 写入和默认值初始化

`Put(ctx, key, value)` 在有远端配置时写入远端，否则写入本地 KV，并更新匹配当前加载前缀的快照。远端写入需要 `projects:read`、`environments:read` 和 `config:write`。`IsLocal()` 表示是否未配置远端。配置键遵循服务端的 1–191 字符规则。

`EnsureDefaults(ctx, defaults)` 在当前写入后端中原子初始化缺失的键，然后重新加载多级快照；已有值不会被覆盖。远端初始化需要支持 `/defaults` 接口的服务器，以及 `config:read`、`projects:read`、`environments:read` 和 `config:write` 权限。低优先级来源已有值不会阻止初始化远端缺失项。它会持久化值，作用不同于只参与读取的 `Config.Defaults`。

## 统一数据源抽象

远端、KV、dotenv 和默认值均使用同一个接口：

```go
type DataSource interface {
    Read(ctx context.Context, prefix string) ([]Item, error)
}
```

公开的 `Sources` 可组合任意实现该接口的数据源，按顺序为每个键选取第一个值，并聚合各来源错误。来源可以同时返回可用数据和错误。`MapSource` 提供内存配置，`DotenvSource` 提供文件配置；SDK 内部用相同机制组合远端和 KV。

```go
var source configcenter.DataSource = configcenter.Sources{
    configcenter.DotenvSource{Files: []string{".env"}},
    configcenter.MapSource{"db.pool": "10"},
}
items, err := source.Read(ctx, "db.")
```

`Read` 返回匹配前缀的配置项，不修改客户端快照；`Sources.Read` 按键排序。调用期间不要修改传入的 map 或文件列表。`New` 会复制 `Config` 中的默认值 map 和文件列表。

## 快照读取

| 方法                        | 行为                                               |
| --------------------------- | -------------------------------------------------- |
| `Load(ctx)`                 | 合并所有来源，提供可用快照并返回来源错误                       |
| `LoadPrefix(ctx, "db.")`    | 逐层合并匹配前缀的配置，替换整个快照         |
| `Get(key)`                  | 返回字符串和是否存在，不发网络请求                 |
| `GetDefault(key, fallback)` | 缺失时返回默认值；空字符串仍是有效值               |
| `Int(key, fallback)`        | 去除两端空白后解析整数，缺失或解析失败时返回默认值 |
| `Bool(key, fallback)`       | 使用 Go `strconv.ParseBool`，失败时返回默认值      |
| `All()`                     | 返回按 key 排序的配置项副本                        |
| `Environment()`             | 返回当前客户端环境 slug                            |

首次 Load 前快照为空。读取和快照替换受锁保护，可并发调用。来源失败时可使用其他来源的值；存在来源错误且没有可用值、或 context 取消时保留已有快照。需要区分缺失和无效值时，使用 `Get` 后自行解析。配置中的 JSON 也需要调用方自行反序列化。

## 自动刷新

```go
watchCtx, cancel := context.WithCancel(ctx)
defer cancel()
if err := client.Watch(watchCtx, configcenter.WatchOptions{
    Interval: 30 * time.Second,
    OnError: func(err error) {
        logger.Error("refresh configuration", "error", err)
    },
}); err != nil {
    return fmt.Errorf("start configuration watcher: %w", err)
}
```

此片段放在持有 `client`、`ctx` 和 `logger` 的长生命周期函数中，并在该函数结束前运行应用。Watch 会同步执行首次完整 Load；首次失败会返回错误，不启动后台循环。成功后启动后台定时轮询，Interval 小于等于零时使用 30 秒。后续错误调用 OnError，未提供回调时静默重试；有可用 fallback 时快照仍会更新。

取消 context 可停止轮询。一个客户端通常只启动一次 Watch。Watch 每次加载全部配置，因此会覆盖 LoadPrefix 得到的过滤快照；它不提供前缀订阅、变更回调或服务端推送。

## 错误处理

使用 `errors.Is(err, configcenter.ErrNotFound)` 判断 HTTP 404（例如环境不存在）。其他非 200 状态目前返回包含状态码的普通错误，没有导出的结构化 HTTP 错误类型。`Get` 通过 bool 表示缺失，并不返回 `ErrKeyNotFound`。为启动失败和后台刷新失败分别制定处理策略。
