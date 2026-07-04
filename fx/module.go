package gologgerstarter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gocfgmodulefx "github.com/kordar/gocfg-load-module/fx/v2"
	"github.com/kordar/gologger_zap"
	"github.com/spf13/cast"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ModuleConfig 描述单个 gologger 实例的全部配置。
// 当前 starter 只管理一个 logger，不再区分 access/error 等多实例。
type ModuleConfig struct {
	Driver          string
	OutputDir       string
	ZapLevel        string
	ZapEncoding     string
	ZapDevelopment  bool
	ZapOutputPaths  []string
	ZapErrOutPaths  []string
	ZapRotateDaily  bool
	ZapRotateHourly bool
	TouchOnStart    bool
	AddSource       bool
	SlogLevel       slog.Leveler
	SetDefault      bool
	FxLogger        bool // 是否将 slog 适配为 fxevent.Logger，替换 Fx 默认日志
}

type cfgModule struct {
	name  string
	index int
}

const (
	moduleKeyFxLog  = "fx_logger"
	moduleLoggerTag = `name:"gologger"`
)

var _ gocfgmodulefx.GoCfgModule = cfgModule{}
var _ gocfgmodulefx.GoCfgIndex = cfgModule{}

type Option func(*cfgModule)

// WithIndex 设置优先级
func WithIndex(index int) Option {
	return func(s *cfgModule) {
		s.index = index
	}
}

// StarterModule 返回可注册到 gocfg-load-module/fx 的日志模块适配器。
func StarterModule(name string, opts ...Option) gocfgmodulefx.GoCfgModule {
	c := &cfgModule{name: name}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (m cfgModule) Index() int {
	return m.index
}

func (m cfgModule) Name() string {
	return m.name
}

func (m cfgModule) Load(data any) []fx.Option {
	slog.Info("Module Load Complete", "module", "gologger-starter(fx)")
	return Module(buildModuleConfig(data))
}

// Module 返回 fx.Option 切片，按配置初始化并注册单个日志实例。
// 日志实例以 `name:"gologger"` 的命名标签注册到 fx 容器。
// 若配置了 set_default，则在 provider 中直接调用 slog.SetDefault。
// 若配置了 fx_logger，则将 fx.WithLogger 作为顶层 option 返回，替换 Fx 内部日志。
func Module(config ModuleConfig) []fx.Option {
	config = normalizeModuleConfig(config)
	logger, err := provideLogger(config)

	result := []fx.Option{
		fx.Module("gologger-starter",
			fx.Supply(config),
			fx.Provide(
				fx.Annotate(
					func() (*slog.Logger, error) { return logger, err },
					fx.ResultTags(moduleLoggerTag),
				),
			),
		),
	}

	if config.FxLogger {
		result = append(result, fx.WithLogger(func() fxevent.Logger { return &slogFxLogger{logger: logger} }))
	}

	return result
}

func buildModuleConfig(data any) ModuleConfig {
	section := cloneStringAnyMap(cast.ToStringMap(data))
	if len(section) == 0 {
		return ModuleConfig{}
	}

	for key, value := range section {
		if _, ok := value.(map[string]any); ok {
			slog.Warn("gologger nested config is ignored in single logger mode", "section", key)
			delete(section, key)
		}
	}

	cfg := ModuleConfig{
		Driver:          cast.ToString(section["driver"]),
		OutputDir:       strings.TrimSpace(cast.ToString(section["output_dir"])),
		ZapLevel:        cast.ToString(section["level"]),
		ZapEncoding:     cast.ToString(section["zap_encoding"]),
		ZapDevelopment:  cast.ToBool(section["zap_development"]),
		ZapOutputPaths:  parseStringSliceValue(section["zap_output_paths"]),
		ZapErrOutPaths:  parseStringSliceValue(section["zap_error_output_paths"]),
		ZapRotateDaily:  cast.ToBool(section["zap_rotate_daily"]),
		ZapRotateHourly: cast.ToBool(section["zap_rotate_hourly"]),
		TouchOnStart:    cast.ToBool(section["touch_on_start"]),
		AddSource:       cast.ToBool(section["add_source"]),
		SlogLevel:       parseSlogLeveler(section["slog_level"]),
		SetDefault:      cast.ToBool(section["set_default"]),
		FxLogger:        cast.ToBool(section[moduleKeyFxLog]),
	}
	return normalizeModuleConfig(cfg)
}

func provideLogger(cfg ModuleConfig) (*slog.Logger, error) {
	cfg = normalizeModuleConfig(cfg)
	driver := cfg.Driver
	if driver != "zap" {
		return nil, fmt.Errorf("gologger: unsupported driver %q", driver)
	}

	if cfg.TouchOnStart {
		if err := ensureLoggerOutputPaths(cfg); err != nil {
			return nil, fmt.Errorf("gologger: prepare outputs: %w", err)
		}
	}

	zl, err := buildZapLogger(cfg)
	if err != nil {
		return nil, fmt.Errorf("gologger: build zap: %w", err)
	}

	opts := &slog.HandlerOptions{
		AddSource: cfg.AddSource,
	}
	if cfg.SlogLevel != nil {
		opts.Level = cfg.SlogLevel
	}

	sl := gologger_zap.NewSlogLogger(zl, opts)
	if cfg.SetDefault {
		slog.SetDefault(sl)
		slog.Info("slog default logger updated", "logger", "gologger")
	}
	slog.Info("gologger initialized")
	return sl, nil
}

func normalizeModuleConfig(cfg ModuleConfig) ModuleConfig {
	if cfg.Driver == "" {
		cfg.Driver = "zap"
	}
	if cfg.ZapLevel == "" {
		cfg.ZapLevel = "info"
	}
	if cfg.OutputDir != "" {
		cfg.OutputDir = filepath.Clean(cfg.OutputDir)
	}
	return cfg
}

func resolvedModulePaths(cfg ModuleConfig) (outputPaths []string, errOutputPaths []string) {
	cfg = normalizeModuleConfig(cfg)
	return resolveOutputPaths(cfg.OutputDir, cfg.ZapOutputPaths), resolveOutputPaths(cfg.OutputDir, cfg.ZapErrOutPaths)
}

func resolveOutputPaths(outputDir string, paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	resolved := make([]string, 0, len(paths))
	for _, p := range paths {
		trimmed := strings.TrimSpace(p)
		switch strings.ToLower(trimmed) {
		case "", "stdout", "stderr":
			resolved = append(resolved, trimmed)
		default:
			if outputDir != "" && !filepath.IsAbs(trimmed) {
				resolved = append(resolved, filepath.Join(outputDir, trimmed))
				continue
			}
			resolved = append(resolved, trimmed)
		}
	}
	return resolved
}

func ensureLoggerOutputPaths(cfg ModuleConfig) error {
	period := resolveRotatePeriod(cfg.ZapRotateDaily, cfg.ZapRotateHourly)
	outputPaths, errOutputPaths := resolvedModulePaths(cfg)
	paths := append([]string{}, outputPaths...)
	paths = append(paths, errOutputPaths...)
	for _, p := range paths {
		if err := ensureOutputPath(p, period, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func ensureOutputPath(path string, period rotatePeriod, now time.Time) error {
	trimmed := strings.TrimSpace(path)
	switch strings.ToLower(trimmed) {
	case "", "stdout", "stderr":
		return nil
	}

	target := logPathForTime(trimmed, period, now)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(target), err)
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %q: %w", target, err)
	}
	return f.Close()
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func parseStringSliceValue(v any) []string {
	switch vv := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), vv...)
	case []any:
		items := make([]string, 0, len(vv))
		for _, item := range vv {
			s := strings.TrimSpace(cast.ToString(item))
			if s != "" {
				items = append(items, s)
			}
		}
		return items
	case string:
		return parseStringSliceLiteral(vv)
	default:
		return cast.ToStringSlice(v)
	}
}

func parseStringSliceLiteral(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var items []string
		if err := json.Unmarshal([]byte(raw), &items); err == nil {
			return items
		}

		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		if inner == "" {
			return nil
		}

		parts := strings.Split(inner, ",")
		items = make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, `"'`)
			if part != "" {
				items = append(items, part)
			}
		}
		return items
	}

	return []string{raw}
}

// ----------------------------------------------------------------
// 以下工具函数保持不变（zap 构建、日志轮转等）
// ----------------------------------------------------------------

func parseSlogLeveler(v any) slog.Leveler {
	if v == nil {
		return nil
	}
	switch vv := v.(type) {
	case slog.Leveler:
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

func buildZapLogger(cfg ModuleConfig) (*zap.Logger, error) {
	cfg = normalizeModuleConfig(cfg)
	outputPaths, errOutputPaths := resolvedModulePaths(cfg)
	var zcfg zap.Config
	if cfg.ZapDevelopment {
		zcfg = zap.NewDevelopmentConfig()
	} else {
		zcfg = zap.NewProductionConfig()
	}

	if cfg.ZapEncoding != "" {
		zcfg.Encoding = cfg.ZapEncoding
	}

	if cfg.ZapLevel != "" {
		var lvl zapcore.Level
		if err := lvl.UnmarshalText([]byte(cfg.ZapLevel)); err != nil {
			return nil, err
		}
		zcfg.Level.SetLevel(lvl)
	}

	if len(outputPaths) > 0 {
		zcfg.OutputPaths = outputPaths
	}
	if len(errOutputPaths) > 0 {
		zcfg.ErrorOutputPaths = errOutputPaths
	}

	period := resolveRotatePeriod(cfg.ZapRotateDaily, cfg.ZapRotateHourly)
	if period == rotatePeriodNone {
		return zcfg.Build()
	}

	ws, err := buildZapWriteSyncer(zcfg.OutputPaths, period)
	if err != nil {
		return nil, err
	}
	errWS, err := buildZapWriteSyncer(zcfg.ErrorOutputPaths, period)
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
	opts = append(opts, zap.ErrorOutput(errWS))

	return zap.New(core, opts...), nil
}

func buildZapWriteSyncer(outputPaths []string, period rotatePeriod) (zapcore.WriteSyncer, error) {
	writers := make([]zapcore.WriteSyncer, 0, len(outputPaths))
	for _, p := range outputPaths {
		ws, err := openWriteSyncer(p, period)
		if err != nil {
			return nil, err
		}
		writers = append(writers, ws)
	}

	if len(writers) == 0 {
		return zapcore.AddSync(os.Stdout), nil
	}
	if len(writers) == 1 {
		return writers[0], nil
	}
	return zapcore.NewMultiWriteSyncer(writers...), nil
}

func openWriteSyncer(path string, period rotatePeriod) (zapcore.WriteSyncer, error) {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "", "stdout":
		return zapcore.AddSync(os.Stdout), nil
	case "stderr":
		return zapcore.AddSync(os.Stderr), nil
	default:
		if period != rotatePeriodNone {
			return newTimeRotatingWriteSyncer(path, period), nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", path, err)
		}
		return zapcore.AddSync(f), nil
	}
}

type rotatePeriod uint8

const (
	rotatePeriodNone rotatePeriod = iota
	rotatePeriodDaily
	rotatePeriodHourly
)

func resolveRotatePeriod(rotateDaily bool, rotateHourly bool) rotatePeriod {
	if rotateHourly {
		return rotatePeriodHourly
	}
	if rotateDaily {
		return rotatePeriodDaily
	}
	return rotatePeriodNone
}

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
	suffix := rotationSuffix(now, w.period)
	if w.f != nil && suffix == w.currentSuffix {
		return nil
	}

	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}

	path := logPathForTime(w.basePath, w.period, now)
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

func logPathForTime(basePath string, period rotatePeriod, now time.Time) string {
	if period == rotatePeriodNone {
		return basePath
	}
	return basePath + "." + rotationSuffix(now, period)
}

func rotationSuffix(now time.Time, period rotatePeriod) string {
	layout := "20060102"
	if period == rotatePeriodHourly {
		layout = "2006010215"
	}
	return now.Format(layout)
}

// ----------------------------------------------------------------
// Fx Logger 适配
// ----------------------------------------------------------------

// slogFxLogger 将 log/slog 适配为 fxevent.Logger，替代 Fx 默认日志。
type slogFxLogger struct {
	logger *slog.Logger
}

func (l *slogFxLogger) LogEvent(event fxevent.Event) {
	logger := l.logger

	switch e := event.(type) {
	case *fxevent.OnStartExecuting:
		logger.Info("fx: start executing hook",
			slog.String("caller", e.CallerName),
			slog.String("function", e.FunctionName),
		)
	case *fxevent.OnStartExecuted:
		if e.Err != nil {
			logger.Error("fx: start hook failed",
				slog.String("caller", e.CallerName),
				slog.String("function", e.FunctionName),
				slog.Any("err", e.Err),
				slog.String("runtime", e.Runtime.String()),
			)
		} else {
			logger.Info("fx: start hook executed",
				slog.String("caller", e.CallerName),
				slog.String("function", e.FunctionName),
				slog.String("runtime", e.Runtime.String()),
			)
		}
	case *fxevent.OnStopExecuting:
		logger.Info("fx: stop executing hook",
			slog.String("caller", e.CallerName),
			slog.String("function", e.FunctionName),
		)
	case *fxevent.OnStopExecuted:
		if e.Err != nil {
			logger.Error("fx: stop hook failed",
				slog.String("caller", e.CallerName),
				slog.String("function", e.FunctionName),
				slog.Any("err", e.Err),
				slog.String("runtime", e.Runtime.String()),
			)
		} else {
			logger.Info("fx: stop hook executed",
				slog.String("caller", e.CallerName),
				slog.String("function", e.FunctionName),
				slog.String("runtime", e.Runtime.String()),
			)
		}
	case *fxevent.Supplied:
		if e.Err != nil {
			logger.Error("fx: supply failed",
				slog.String("type", e.TypeName),
				slog.Any("err", e.Err),
			)
		} else {
			logger.Debug("fx: supplied",
				slog.String("type", e.TypeName),
			)
		}
	case *fxevent.Provided:
		for _, rtype := range e.OutputTypeNames {
			logger.Debug("fx: provided",
				slog.String("constructor", e.ConstructorName),
				slog.String("type", rtype),
			)
		}
		if e.Err != nil {
			logger.Error("fx: provide failed",
				slog.String("constructor", e.ConstructorName),
				slog.Any("err", e.Err),
			)
		}
	case *fxevent.Replaced:
		for _, rtype := range e.OutputTypeNames {
			logger.Debug("fx: replaced",
				slog.String("type", rtype),
			)
		}
	case *fxevent.Decorated:
		for _, rtype := range e.OutputTypeNames {
			logger.Debug("fx: decorated",
				slog.String("decorator", e.DecoratorName),
				slog.String("type", rtype),
			)
		}
	case *fxevent.Invoking:
		logger.Debug("fx: invoking",
			slog.String("function", e.FunctionName),
			slog.String("module", e.ModuleName),
		)
	case *fxevent.Invoked:
		if e.Err != nil {
			logger.Error("fx: invoke failed",
				slog.String("function", e.FunctionName),
				slog.String("module", e.ModuleName),
				slog.Any("err", e.Err),
			)
		}
	case *fxevent.Stopping:
		logger.Info("fx: stopping",
			slog.String("signal", strings.ToUpper(e.Signal.String())),
		)
	case *fxevent.Stopped:
		if e.Err != nil {
			logger.Error("fx: stopped with error",
				slog.Any("err", e.Err),
			)
		} else {
			logger.Info("fx: stopped")
		}
	case *fxevent.Started:
		if e.Err != nil {
			logger.Error("fx: start failed",
				slog.Any("err", e.Err),
			)
		} else {
			logger.Info("fx: started")
		}
	case *fxevent.LoggerInitialized:
		if e.Err != nil {
			logger.Error("fx: logger init failed",
				slog.Any("err", e.Err),
			)
		} else {
			logger.Debug("fx: logger initialized",
				slog.String("function", e.ConstructorName),
			)
		}
	case *fxevent.Run:
		if e.Err != nil {
			logger.Error("fx: run failed",
				slog.String("name", e.Name),
				slog.String("kind", e.Kind),
				slog.Any("err", e.Err),
			)
		} else {
			logger.Debug("fx: run",
				slog.String("name", e.Name),
				slog.String("kind", e.Kind),
				slog.String("module", e.ModuleName),
			)
		}
	}
}
