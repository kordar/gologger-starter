package gologgerstarter

import (
	"log/slog"
	"strings"

	"github.com/kordar/gologger_nazalog"
	"github.com/q191201771/naza/pkg/nazalog"
	"github.com/spf13/cast"
)

var HookBackendOutFn func(level nazalog.Level, line []byte)

type LoggerModule struct {
	name string
	load any
}

func NewLoggerModule(name string, load any) *LoggerModule {
	return &LoggerModule{name: name, load: load}
}

func (m LoggerModule) Name() string {
	return m.name
}

func (m LoggerModule) _load(id string, cfg map[string]any) {
	if id == "" {
		slog.Warn("the attribute id cannot be empty", "module", m.Name())
		return
	}
	if m.load == nil {
		slog.Warn("load callback cannot be nil", "module", m.Name(), "id", id)
		return
	}

	driver := cast.ToString(cfg["driver"])
	if driver == "nazalog" {
		l, err := nazalog.New(func(o *nazalog.Option) {
			if cfg["level"] != nil {
				o.Level = nazalog.Level(cast.ToInt(cfg["level"]))
			}
			if cfg["is_to_stdout"] != nil {
				o.IsToStdout = cast.ToBool(cfg["is_to_stdout"])
			}
			if cfg["filename"] != nil {
				o.Filename = cast.ToString(cfg["filename"])
			}
			if cfg["is_rotate_daily"] != nil {
				o.IsRotateDaily = cast.ToBool(cfg["is_rotate_daily"])
			}
			if cfg["is_rotate_hourly"] != nil {
				o.IsRotateHourly = cast.ToBool(cfg["is_rotate_hourly"])
			}
			if cfg["timestamp_flag"] != nil {
				o.TimestampFlag = cast.ToBool(cfg["timestamp_flag"])
			}
			if cfg["timestamp_with_ms_flag"] != nil {
				o.TimestampWithMsFlag = cast.ToBool(cfg["timestamp_with_ms_flag"])
			}
			if cfg["level_flag"] != nil {
				o.LevelFlag = cast.ToBool(cfg["level_flag"])
			}
			if cfg["short_file_flag"] != nil {
				o.ShortFileFlag = cast.ToBool(cfg["short_file_flag"])
			}
			if cfg["assert_behavior"] != nil {
				o.AssertBehavior = nazalog.AssertBehavior(cast.ToInt(cfg["assert_behavior"]))
			}
			if HookBackendOutFn != nil {
				o.HookBackendOutFn = HookBackendOutFn
			}
		})
		if err != nil {
			slog.Error("init nazalog failed", "module", m.Name(), "id", id, "err", err)
			return
		}

		opts := &slog.HandlerOptions{}
		if cfg["add_source"] != nil {
			opts.AddSource = cast.ToBool(cfg["add_source"])
		}
		if cfg["slog_level"] != nil {
			opts.Level = parseSlogLeveler(cfg["slog_level"])
		}

		sl := gologger_nazalog.NewSlogLogger(l, opts)
		if cast.ToBool(cfg["set_default_logger"]) {
			slog.SetDefault(sl)
		}

		switch load := m.load.(type) {
		case func(id string, logger *slog.Logger):
			load(id, sl)
		case func(id string, logger *slog.Logger, cfg map[string]any):
			load(id, sl, cfg)
		case func(logger *slog.Logger):
			load(sl)
		default:
			slog.Warn("unsupported load callback signature", "module", m.Name(), "id", id)
		}
		slog.Info("loading module successfully", "module", m.Name(), "id", id, "driver", driver)
		return
	}

	

	slog.Warn("unsupported driver", "module", m.Name(), "id", id, "driver", driver)
}

func (m LoggerModule) Load(value any) {
	if value == nil {
		return
	}

	items := cast.ToStringMap(value)
	if items["id"] != nil {
		id := cast.ToString(items["id"])
		m._load(id, items)
		return
	}

	for key, item := range items {
		m._load(key, cast.ToStringMap(item))
	}
}

func (m LoggerModule) Close() {
}

func parseSlogLeveler(v any) slog.Leveler {
	if v == nil {
		return nil
	}
	switch vv := v.(type) {
	case slog.Leveler:
		return vv
	case slog.Level:
		return vv
	case int:
		return slog.Level(vv)
	case int64:
		return slog.Level(vv)
	case float64:
		return slog.Level(int(vv))
	case string:
		s := strings.TrimSpace(strings.ToLower(vv))
		switch s {
		case "debug":
			return slog.LevelDebug
		case "info":
			return slog.LevelInfo
		case "warn", "warning":
			return slog.LevelWarn
		case "error":
			return slog.LevelError
		case "trace":
			return slog.LevelDebug - 4
		default:
			return slog.LevelInfo
		}
	default:
		return slog.LevelInfo
	}
}
