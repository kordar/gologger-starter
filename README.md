# gologger-starter

基于配置加载 `log/slog` 的启动器。目前仅支持 `driver=zap`，通过 [gologger_zap](https://github.com/kordar/gologger_zap) 将日志输出到 [zap](https://github.com/uber-go/zap)。

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
)

func main() {
	m := gologgerstarter.NewLoggerModule("logger", func(id string, logger *slog.Logger) {
		// 拿到构造好的 slog.Logger
		logger.Info("logger ready", "id", id)
	})

	m.Load(map[string]any{
		"id":                 "default",
		"driver":             "zap",
		"zap_development":    true,
		"zap_level":          "debug",
		"zap_encoding":       "console",
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
- `driver`: string，目前仅支持 `"zap"`（也可不填，默认 zap）

### driver=zap 字段

这些字段会映射到 `zap.Config`：

- `zap_development`: bool，使用 development/production 默认配置
- `zap_level`: string，例如 `"debug"|"info"|"warn"|"error"`
- `zap_encoding`: string，例如 `"json"|"console"`
- `zap_output_paths`: []string，例如 `[]string{"stdout"}` 或 `[]string{"./app.log"}`
- `zap_error_output_paths`: []string
- `zap_rotate_daily`: bool，按天切割文件（对文件路径生效，stdout/stderr 无效）
- `zap_rotate_hourly`: bool，按小时切割文件（优先级高于按天）

这些字段会映射到 `slog.HandlerOptions`：

- `slog_level`: string/int，支持 `"trace"|"debug"|"info"|"warn"|"error"` 或直接传 `slog.Level`/数字
- `add_source`: bool

其它：

- `set_default_logger`: bool，为 `true` 时会执行 `slog.SetDefault(logger)`

## Load 回调签名

`NewLoggerModule(name, load)` 的 `load` 支持以下签名之一：

- `func(id string, logger *slog.Logger)`
- `func(id string, logger *slog.Logger, cfg map[string]any)`
- `func(logger *slog.Logger)`
