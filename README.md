# gologger-starter

基于配置驱动的 `log/slog` 启动器，默认使用 [zap](https://github.com/uber-go/zap) 作为底层引擎。提供两种使用方式：

- **root 包** (`gologgerstarter`) — 轻量 `LoggerModule`，适合非 Fx 项目
- **fx 子模块** (`github.com/kordar/gologger-starter/fx/v2`) — 完整 Fx 集成，与 [gologger](https://github.com/kordar/gologger) 生态协作

## 安装

```bash
# 非 Fx 项目
go get github.com/kordar/gologger-starter

# Fx 项目
go get github.com/kordar/gologger-starter/fx/v2
```

---

## root 包 — LoggerModule

适合非 Fx 项目，直接构造 `*slog.Logger` 并回调。

```go
import gologgerstarter "github.com/kordar/gologger-starter"

m := gologgerstarter.NewLoggerModule("logger", func(id string, logger *slog.Logger) {
    logger.Info("logger ready", "id", id)
})

m.Load(map[string]any{
    "id":              "default",
    "driver":          "zap",
    "zap_development": true,
    "level":           "debug",
    "zap_encoding":    "console",
    "add_source":      true,
    "set_default":     true,
})
```

### Load 输入格式

| 格式 | 说明 |
|------|------|
| `map[string]any{"id": "...", ...}` | 单实例 |
| `map[string]any{"id1": {...}, "id2": {...}}` | 多实例 |

### Load 回调签名

- `func(id string, logger *slog.Logger)`
- `func(id string, logger *slog.Logger, cfg map[string]any)`
- `func(logger *slog.Logger)`

### 配置字段

#### 通用

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | `string` | 实例标识 |
| `driver` | `string` | 仅支持 `"zap"`（默认） |

#### driver=zap

| 字段 | 类型 | 说明 |
|------|------|------|
| `level` | `string` | `debug` / `info` / `warn` / `error` (默认 `info`) |
| `zap_development` | `bool` | 使用 development 模式 |
| `zap_encoding` | `string` | `json` / `console` |
| `zap_output_paths` | `[]string` | 输出路径，`stdout` / 文件路径 |
| `zap_error_output_paths` | `[]string` | 错误输出路径 |
| `zap_rotate_daily` | `bool` | 按天切割文件 |
| `zap_rotate_hourly` | `bool` | 按小时切割 |

#### slog 相关

| 字段 | 类型 | 说明 |
|------|------|------|
| `slog_level` | `string`/`int` | `trace` / `debug` / `info` / `warn` / `error` |
| `add_source` | `bool` | 输出调用位置 |
| `set_default` | `bool` | 执行 `slog.SetDefault(logger)` |

---

## fx 子模块 — Fx 集成

与 [gologger](https://github.com/kordar/gologger) 的 `RouteHandler` / `HandlerDecorator` / `LoggerEnricher` 协作，通过 Fx Value Group 实现松耦合的多模块日志装配。

### 模块路径

```
github.com/kordar/gologger-starter/fx/v2
```

包名：`gologgerstarter`

### 快速开始

```go
import (
    gologgerstarter "github.com/kordar/gologger-starter/fx/v2"
    "go.uber.org/fx"
)

func main() {
    fx.New(
        gologgerstarter.Module(gologgerstarter.ModuleConfig{
            Driver:         "zap",
            ZapEncoding:    "json",
            ZapOutputPaths: []string{"stdout"},
            AddSource:      true,
            SetDefault:     true,
            FxLogger:       true, // 替换 Fx 内部日志
        }),
    ).Run()
}
```

### ModuleConfig

```go
type ModuleConfig struct {
    Driver          string          // 仅支持 "zap"（默认）
    OutputDir       string          // 日志输出根目录（相对路径自动拼接）
    ZapLevel        string          // debug / info / warn / error
    ZapEncoding     string          // json / console
    ZapDevelopment  bool
    ZapOutputPaths  []string        // stdout / 文件路径
    ZapErrOutPaths  []string
    ZapRotateDaily  bool            // 按天切割
    ZapRotateHourly bool            // 按小时切割
    TouchOnStart    bool            // 启动时创建日志文件
    AddSource       bool            // slog 输出调用位置
    SlogLevel       slog.Leveler    // slog 最小级别
    SetDefault      bool            // slog.SetDefault
    FxLogger        bool            // 替换 Fx 内部日志为 slog
}
```

### 装配流程

```
gologger-starter/fx
    │
    ├── provideDefaultRouteHandler  →  Fx Group: "slog-route-handlers"
    │       └── gologger.RouteHandler{Route: "", Handler: zap handler}
    │
    ├── [其他模块注入]  →  Fx Group: "slog-handler-decorators"
    │       └── 如 o11y-starter 的 TraceHandlerMiddleware
    │
    ├── [其他模块注入]  →  Fx Group: "slog-logger-enrichers"
    │       └── 如 o11y-starter 的 StaticAttrsEnricher
    │
    └── assembleApplicationLogger()
            └── gologger.AssembleLogger(handlers, decorators, enrichers)
                    → *slog.Logger (name: "gologger")
```

### 与其他模块协作

gologger-starter 贡献**默认 RouteHandler**，其他模块通过 Fx Group 注入 decorator / enricher：

```go
// o11y-starter 注入 trace decorator 和 static attrs enricher
fx.Provide(
    fx.Annotate(
        func() func(slog.Handler) slog.Handler {
            return logger.TraceHandlerMiddleware(cfg)
        },
        fx.ResultTags(`group:"slog-handler-decorators"`),
    ),
    fx.Annotate(
        func() func(*slog.Logger) *slog.Logger {
            return logger.StaticAttrsEnricher(cfg)
        },
        fx.ResultTags(`group:"slog-logger-enrichers"`),
    ),
)
```

### gocfg-load-module 适配

`StarterModule` 将配置加载与 Fx 初始化衔接：

```go
gocfgmodulefx.StarterModule(gologgerstarter.StarterModule("logger"))
```

### 测试辅助

```go
func provideLogger(cfg ModuleConfig) (*slog.Logger, error)
```

直接构造 logger 用于单元测试，不依赖 Fx 容器。
