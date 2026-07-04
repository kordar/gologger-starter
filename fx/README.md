# gologger-starter/fx

`gologger-starter/fx` 是一个基于 `go.uber.org/fx` 的单实例日志 starter。

它的目标很直接：

- 一个 `[gologger]` 配置段完成一个 `zap -> slog` logger 的全部能力
- 不在 starter 内部区分 `access`、`error`、`audit` 这类多实例
- 给未来接入其他日志组件预留清晰边界，不把“日志分类”耦合进 starter 设计

当前实现基于 `gologger_zap`，日志驱动仅支持 `zap`。

## 快速接入

### 1. 注册 starter

```go
import (
	gocfgmodulefx "github.com/kordar/gocfg-load-module/fx/v2"
	gologgerstarterfx "github.com/kordar/gologger-starter/fx/v2"
)

gocfgmodulefx.Register(gologgerstarterfx.StarterModule("gologger"))
```

### 2. 在 GoAdmin 中使用

[adminserver.go](file:///home/kordar/projects/goadmin/starter/adminserver.go#L14-L23) 和 [cmdserver.go](file:///home/kordar/projects/goadmin/starter/cmdserver.go#L34-L43) 已默认接入该 starter。

### 3. 在业务模块中注入 logger

该 starter 向 Fx 提供一个命名实例：

```text
name:"gologger"
```

注入示例：

```go
type MyService struct {
	fx.In

	Logger *slog.Logger `name:"gologger"`
}
```

如果你开启了 `set_default = true`，业务代码也可以直接使用 `slog.Default()`。

## 配置方式

只使用一个配置段：

```ini
[gologger]
fx_logger = true
driver = "zap"
level = "info"
zap_encoding = "json"
set_default = true
```

不再支持把 `[gologger.access]`、`[gologger.error]` 作为独立 logger 使用。

如果配置里仍然出现这类子段，当前版本会忽略它们，并保留根段 `[gologger]` 的单实例语义。

## 配置项说明

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `fx_logger` | bool | `false` | 是否将 `slog.Default()` 适配为 Fx 内部日志输出 |
| `driver` | string | `"zap"` | 当前仅支持 `"zap"` |
| `output_dir` | string | 空 | 为相对输出路径提供统一根目录 |
| `level` | string | `"info"` | zap 日志级别，支持 `debug` / `info` / `warn` / `error` |
| `zap_encoding` | string | zap 默认值 | 输出格式，支持 `console` / `json` |
| `zap_development` | bool | `false` | 是否使用 zap 开发模式配置 |
| `zap_output_paths` | []string | zap 默认值 | 主输出路径，支持 `stdout` / `stderr` / 文件路径 |
| `zap_error_output_paths` | []string | zap 默认值 | zap 内部错误输出路径 |
| `zap_rotate_daily` | bool | `false` | 是否按天切分文件 |
| `zap_rotate_hourly` | bool | `false` | 是否按小时切分文件 |
| `touch_on_start` | bool | `false` | 启动时是否预创建日志文件 |
| `add_source` | bool | `false` | 是否为 `slog` 输出调用位置信息 |
| `slog_level` | string | 空 | `slog.HandlerOptions.Level`，支持 `trace` / `debug` / `info` / `warn` / `error` |
| `set_default` | bool | `false` | 是否将该实例设置为 `slog.Default()` |

## 输出路径规则

- `stdout` 和 `stderr` 原样保留
- 绝对路径原样保留
- 相对路径在配置了 `output_dir` 后，会自动拼接到该目录下
- 开启切分后，真实文件名会自动追加时间后缀

示例：

```ini
[gologger]
output_dir = "./logs"
zap_output_paths = ["app.log"]
zap_error_output_paths = ["error.log"]
zap_rotate_daily = true
```

对应的实际文件：

```text
./logs/app.log.20060102
./logs/error.log.20060102
```

如果启用 `zap_rotate_hourly = true`，则后缀改为 `.2006010215`。

## 推荐配置

### 开发环境

```ini
[gologger]
fx_logger = true
driver = "zap"
level = "debug"
zap_encoding = "console"
zap_development = true
set_default = true
slog_level = "debug"
add_source = true
```

### 生产环境

```ini
[gologger]
fx_logger = true
driver = "zap"
level = "info"
zap_encoding = "json"
set_default = true
output_dir = "/var/log/goadmin"
zap_output_paths = ["app.log"]
zap_error_output_paths = ["error.log"]
zap_rotate_daily = true
touch_on_start = true
```

## `set_default` 与 `fx_logger` 的区别

- `set_default = true`：决定当前 logger 是否成为 `slog.Default()`
- `fx_logger = true`：决定 Fx 内部事件是否通过 `slog.Default()` 输出

推荐组合：

- 业务想统一使用一个全局 logger：开启 `set_default = true`
- 希望 Fx 启动/停止日志也进入同一套日志：再额外开启 `fx_logger = true`

## 常见问题

### 1. 为什么文件没有立刻出现？

这是正常行为。

- 默认情况下，文件 sink 是惰性创建的
- 只有真正发生写入时，目标文件才会出现
- 如果开启了切分，真实文件名会带上日期或小时后缀

例如：

```ini
[gologger]
zap_output_paths = ["./logs/app.log"]
zap_rotate_daily = true
```

实际文件类似：

```text
./logs/app.log.20260702
```

如果希望应用启动时就先创建文件，可以开启：

```ini
touch_on_start = true
```

### 2. `output_dir` 和 `zap_output_paths` 怎么搭配？

推荐把目录和文件名拆开：

```ini
output_dir = "./logs"
zap_output_paths = ["app.log"]
zap_error_output_paths = ["error.log"]
```

这样目录迁移更方便，也更符合单实例配置模型。

### 3. 旧的 `[gologger.access]` 配置怎么办？

当前版本不会再把它当成单独 logger 使用。

建议把它合并回根段：

```ini
[gologger]
output_dir = "./logs"
zap_output_paths = ["app.log"]
zap_error_output_paths = ["error.log"]
zap_rotate_daily = true
```

如果历史配置里仍然带子段，starter 会忽略这些子段，避免继续扩散多实例语义。

## 实现说明

- 模块实现了 `gocfgmodulefx.GoCfgModule`，可直接接入 `gocfg-load-module/fx`
- starter 内部只创建一个 logger，并以 `name:"gologger"` 注册到 Fx 容器
- 模块内部通过 `fx.Invoke` 强制触发 logger 构造，避免 provider 惰性执行导致副作用不生效
- `set_default` 在 provider 阶段直接调用 `slog.SetDefault`
- `touch_on_start` 会在 logger 初始化时预创建目标输出文件
- 轮转文件由内置 `timeRotatingWriteSyncer` 管理，支持按天和按小时两种粒度
