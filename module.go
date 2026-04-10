package gologgerstarter

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kordar/gologger_zap"
	"github.com/spf13/cast"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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
	if driver == "" {
		driver = "zap"
	}
	if driver != "zap" {
		slog.Warn("unsupported driver", "module", m.Name(), "id", id, "driver", driver)
		return
	}

	zl, err := buildZapLogger(cfg)
	if err != nil {
		slog.Error("init zap failed", "module", m.Name(), "id", id, "err", err)
		return
	}

	opts := &slog.HandlerOptions{}
	if cfg["add_source"] != nil {
		opts.AddSource = cast.ToBool(cfg["add_source"])
	}
	if cfg["slog_level"] != nil {
		opts.Level = parseSlogLeveler(cfg["slog_level"])
	}

	sl := gologger_zap.NewSlogLogger(zl, opts)
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

func buildZapLogger(cfg map[string]any) (*zap.Logger, error) {
	dev := cast.ToBool(cfg["zap_development"])
	levelStr := cast.ToString(cfg["zap_level"])
	encoding := cast.ToString(cfg["zap_encoding"])
	rotateDaily := cast.ToBool(cfg["zap_rotate_daily"])
	rotateHourly := cast.ToBool(cfg["zap_rotate_hourly"])

	var zcfg zap.Config
	if dev {
		zcfg = zap.NewDevelopmentConfig()
	} else {
		zcfg = zap.NewProductionConfig()
	}

	if encoding != "" {
		zcfg.Encoding = encoding
	}

	if levelStr != "" {
		var lvl zapcore.Level
		if err := lvl.UnmarshalText([]byte(levelStr)); err != nil {
			return nil, err
		}
		zcfg.Level.SetLevel(lvl)
	}

	if cfg["zap_output_paths"] != nil {
		zcfg.OutputPaths = cast.ToStringSlice(cfg["zap_output_paths"])
	}
	if cfg["zap_error_output_paths"] != nil {
		zcfg.ErrorOutputPaths = cast.ToStringSlice(cfg["zap_error_output_paths"])
	}

	if !rotateDaily && !rotateHourly {
		return zcfg.Build()
	}

	ws, err := buildZapWriteSyncerWithRotation(zcfg.OutputPaths, rotateDaily, rotateHourly)
	if err != nil {
		return nil, err
	}

	var enc zapcore.Encoder
	switch strings.ToLower(zcfg.Encoding) {
	case "console":
		enc = zapcore.NewConsoleEncoder(zcfg.EncoderConfig)
	default:
		enc = zapcore.NewJSONEncoder(zcfg.EncoderConfig)
	}

	core := zapcore.NewCore(enc, ws, zcfg.Level)
	opts := make([]zap.Option, 0, 8)
	if !zcfg.DisableCaller {
		opts = append(opts, zap.AddCaller())
	}
	if !zcfg.DisableStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	if len(zcfg.InitialFields) > 0 {
		fields := make([]zap.Field, 0, len(zcfg.InitialFields))
		for k, v := range zcfg.InitialFields {
			fields = append(fields, zap.Any(k, v))
		}
		opts = append(opts, zap.Fields(fields...))
	}
	if zcfg.Development {
		opts = append(opts, zap.Development())
	}

	return zap.New(core, opts...), nil
}

func buildZapWriteSyncerWithRotation(outputPaths []string, rotateDaily bool, rotateHourly bool) (zapcore.WriteSyncer, error) {
	period := rotatePeriodDaily
	if rotateHourly {
		period = rotatePeriodHourly
	}

	writers := make([]zapcore.WriteSyncer, 0, len(outputPaths))
	for _, p := range outputPaths {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "", "stdout":
			writers = append(writers, zapcore.AddSync(os.Stdout))
		case "stderr":
			writers = append(writers, zapcore.AddSync(os.Stderr))
		default:
			writers = append(writers, newTimeRotatingWriteSyncer(p, period))
		}
	}

	if len(writers) == 0 {
		return zapcore.AddSync(os.Stdout), nil
	}
	if len(writers) == 1 {
		return writers[0], nil
	}
	return zapcore.NewMultiWriteSyncer(writers...), nil
}

type rotatePeriod uint8

const (
	rotatePeriodDaily rotatePeriod = iota + 1
	rotatePeriodHourly
)

type timeRotatingWriteSyncer struct {
	mu       sync.Mutex
	basePath string
	period   rotatePeriod

	currentSuffix string
	f             *os.File
}

func newTimeRotatingWriteSyncer(basePath string, period rotatePeriod) *timeRotatingWriteSyncer {
	return &timeRotatingWriteSyncer{
		basePath: basePath,
		period:   period,
	}
}

func (w *timeRotatingWriteSyncer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeededLocked(time.Now()); err != nil {
		return 0, err
	}
	return w.f.Write(p)
}

func (w *timeRotatingWriteSyncer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	return w.f.Sync()
}

func (w *timeRotatingWriteSyncer) rotateIfNeededLocked(now time.Time) error {
	layout := "20060102"
	if w.period == rotatePeriodHourly {
		layout = "2006010215"
	}
	suffix := now.Format(layout)
	if w.f != nil && suffix == w.currentSuffix {
		return nil
	}

	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}

	path := w.basePath + "." + suffix
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	w.f = f
	w.currentSuffix = suffix
	return nil
}
