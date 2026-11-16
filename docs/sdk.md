# Go SDK 接入

[文档首页](/docs/) · [HTTP API](/docs/api) · [Markdown 原文](/docs/sdk.md)

## 安装

SDK 是独立、无第三方依赖的 Go 模块，需要 Go 1.26 或更高版本。在调用方 Go 模块目录执行：

```sh
go get github.com/xeu/configcenter/sdk/configcenter
```

生产项目在 go.mod 中固定并审查实际解析的版本。若使用尚未发布的本地修改，可在调用方模块中增加 `replace github.com/xeu/configcenter/sdk/configcenter => /absolute/path/to/configcenter/sdk/configcenter`。

## 初始化和首次读取

以下函数可放入调用方应用，将地址和密钥从调用方配置层传入：

```go
package integration

import (
    "context"
    "fmt"
    "time"

    "github.com/xeu/configcenter/sdk/configcenter"
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

## 快照读取

| 方法                        | 行为                                               |
| --------------------------- | -------------------------------------------------- |
| `Load(ctx)`                 | 请求所有配置，成功后替换快照                       |
| `LoadPrefix(ctx, "db.")`    | 仅加载匹配前缀的配置，替换整个快照，不合并         |
| `Get(key)`                  | 返回字符串和是否存在，不发网络请求                 |
| `GetDefault(key, fallback)` | 缺失时返回默认值；空字符串仍是有效值               |
| `Int(key, fallback)`        | 去除两端空白后解析整数，缺失或解析失败时返回默认值 |
| `Bool(key, fallback)`       | 使用 Go `strconv.ParseBool`，失败时返回默认值      |
| `All()`                     | 返回按 key 排序的配置项副本                        |
| `Environment()`             | 返回当前客户端环境 slug                            |

首次 Load 前快照为空。读取和快照替换受锁保护，可并发调用。加载失败保留已有快照。需要区分缺失和无效值时，使用 `Get` 后自行解析。配置中的 JSON 也需要调用方自行反序列化。

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

此片段放在持有 `client`、`ctx` 和 `logger` 的长生命周期函数中，并在该函数结束前运行应用。Watch 会同步执行首次完整 Load；首次失败会返回错误，不启动后台循环。成功后启动后台定时轮询，Interval 小于等于零时使用 30 秒。后续错误调用 OnError，未提供回调时静默重试；旧快照保留。

取消 context 可停止轮询。一个客户端通常只启动一次 Watch。Watch 每次加载全部配置，因此会覆盖 LoadPrefix 得到的过滤快照；它不提供前缀订阅、变更回调或服务端推送。

## 错误处理

使用 `errors.Is(err, configcenter.ErrNotFound)` 判断 HTTP 404（例如环境不存在）。其他非 200 状态目前返回包含状态码的普通错误，没有导出的结构化 HTTP 错误类型。`Get` 通过 bool 表示缺失，并不返回 `ErrKeyNotFound`。为启动失败和后台刷新失败分别制定处理策略。
