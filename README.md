# gologger-starter

基于配置加载 `log/slog` 的启动器。目前仅支持 `driver=nazalog`，通过 [gologger_nazalog](https://github.com/kordar/gologger_nazalog) 将日志输出到上游的 [nazalog](https://github.com/q191201771/naza/tree/master/pkg/nazalog)。

> 注意：模块路径是 `github.com/kordar/gologger-starter`，包名是 `gologgerstarter`。

## 安装

```bash
go get github.com/kordar/gologger-starter
```

## 快速开始

```go
package main

import (
	"log/slog"

	gologgerstarter "github.com/kordar/gologger-starter"
	"github.com/q191201771/naza/pkg/nazalog"
)

func main() {
	gologgerstarter.HookBackendOutFn = func(level nazalog.Level, line []byte) {
		// 这里可以接入你自己的日志采集/测试断言等逻辑
	}

	m := gologgerstarter.NewLoggerModule("logger", func(id string, logger *slog.Logger) {
		// 拿到构造好的 slog.Logger
		logger.Info("logger ready", "id", id)
	})

	m.Load(map[string]any{
		"id":                 "default",
		"driver":             "nazalog",
		"filename":           "",
		"is_to_stdout":       true,
		"level":              int(nazalog.LevelInfo),
		"slog_level":         "info",
		"add_source":         true,
		"set_default_logger": true,
	})
}
```

## 配置说明

`LoggerModule.Load(value any)` 支持两种输入：

- 单实例：`map[string]any{"id": "...", ...}`
- 多实例：`map[string]any{"id1": map[string]any{...}, "id2": map[string]any{...}}`

### 通用字段

- `id`: string，实例标识
- `driver`: string，目前仅支持 `"nazalog"`

### driver=nazalog 字段

这些字段会映射到 `nazalog.Option`：

- `level`: int，对应 `nazalog.Level`
- `is_to_stdout`: bool
- `filename`: string
- `is_rotate_daily`: bool
- `is_rotate_hourly`: bool
- `timestamp_flag`: bool
- `timestamp_with_ms_flag`: bool
- `level_flag`: bool
- `short_file_flag`: bool
- `assert_behavior`: int，对应 `nazalog.AssertBehavior`

这些字段会映射到 `slog.HandlerOptions`：

- `slog_level`: string/int，支持 `"trace"|"debug"|"info"|"warn"|"error"` 或直接传 `slog.Level`/数字
- `add_source`: bool

其它：

- `set_default_logger`: bool，为 `true` 时会执行 `slog.SetDefault(logger)`
- `HookBackendOutFn`: 包级变量（不是 cfg 字段），若非 nil，会设置到 `nazalog.Option.HookBackendOutFn`

## Load 回调签名

`NewLoggerModule(name, load)` 的 `load` 支持以下签名之一：

- `func(id string, logger *slog.Logger)`
- `func(id string, logger *slog.Logger, cfg map[string]any)`
- `func(logger *slog.Logger)`
