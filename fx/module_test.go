package gologgerstarter

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildModuleConfigSingleInstanceDefaults(t *testing.T) {
	cfg := buildModuleConfig(map[string]any{
		"zap_encoding": "json",
	})

	if cfg.FxLogger {
		t.Fatal("expected fx logger disabled by default")
	}
	if cfg.Driver != "zap" {
		t.Fatalf("expected default driver zap, got %q", cfg.Driver)
	}
	if cfg.ZapEncoding != "json" {
		t.Fatalf("expected encoding json, got %q", cfg.ZapEncoding)
	}
}

func TestBuildModuleConfigSingleSectionOnly(t *testing.T) {
	cfg := buildModuleConfig(map[string]any{
		"fx_logger":    true,
		"zap_encoding": "console",
		"output_dir":   "logs",
		"access": map[string]any{
			"zap_output_paths": []string{"access.log"},
			"zap_rotate_daily": true,
		},
	})

	if !cfg.FxLogger {
		t.Fatal("expected fx logger enabled")
	}
	if cfg.ZapEncoding != "console" {
		t.Fatalf("expected root encoding console, got %q", cfg.ZapEncoding)
	}
	if len(cfg.ZapOutputPaths) != 0 {
		t.Fatalf("expected nested output paths ignored, got %#v", cfg.ZapOutputPaths)
	}
}

func TestBuildModuleConfigParsesJSONStylePathArray(t *testing.T) {
	cfg := buildModuleConfig(map[string]any{
		"output_dir":             "./logs",
		"zap_output_paths":       `["app.log"]`,
		"zap_error_output_paths": `["error.log"]`,
	})

	if len(cfg.ZapOutputPaths) != 1 || cfg.ZapOutputPaths[0] != "app.log" {
		t.Fatalf("expected output paths [app.log], got %#v", cfg.ZapOutputPaths)
	}
	if len(cfg.ZapErrOutPaths) != 1 || cfg.ZapErrOutPaths[0] != "error.log" {
		t.Fatalf("expected error output paths [error.log], got %#v", cfg.ZapErrOutPaths)
	}
}

func TestNormalizeModuleConfigDoesNotRewritePaths(t *testing.T) {
	cfg := normalizeModuleConfig(ModuleConfig{
		OutputDir:      "logs",
		ZapOutputPaths: []string{"app.log", "stdout"},
		ZapErrOutPaths: []string{"error.log"},
	})

	if got := cfg.OutputDir; got != "logs" {
		t.Fatalf("expected cleaned output dir logs, got %q", got)
	}
	if got, want := cfg.ZapOutputPaths[0], "app.log"; got != want {
		t.Fatalf("expected output path %q, got %q", want, got)
	}
	if got := cfg.ZapOutputPaths[1]; got != "stdout" {
		t.Fatalf("expected stdout passthrough, got %q", got)
	}
	if got, want := cfg.ZapErrOutPaths[0], "error.log"; got != want {
		t.Fatalf("expected error path %q, got %q", want, got)
	}
}

func TestResolvedModulePathsResolvesOnce(t *testing.T) {
	cfg := ModuleConfig{
		OutputDir:      "logs",
		ZapOutputPaths: []string{"app.log"},
		ZapErrOutPaths: []string{"error.log"},
	}

	outputPaths, errOutputPaths := resolvedModulePaths(cfg)
	if got, want := outputPaths[0], filepath.Join("logs", "app.log"); got != want {
		t.Fatalf("expected output path %q, got %q", want, got)
	}
	if got, want := errOutputPaths[0], filepath.Join("logs", "error.log"); got != want {
		t.Fatalf("expected error path %q, got %q", want, got)
	}

	cfg = normalizeModuleConfig(cfg)
	outputPaths, errOutputPaths = resolvedModulePaths(cfg)
	if got, want := outputPaths[0], filepath.Join("logs", "app.log"); got != want {
		t.Fatalf("expected output path %q after normalize, got %q", want, got)
	}
	if got, want := errOutputPaths[0], filepath.Join("logs", "error.log"); got != want {
		t.Fatalf("expected error path %q after normalize, got %q", want, got)
	}
}

func TestProvideLoggerTouchOnStartCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := ModuleConfig{
		OutputDir:      dir,
		ZapEncoding:    "json",
		ZapOutputPaths: []string{"app.log"},
		ZapErrOutPaths: []string{"error.log"},
		ZapRotateDaily: true,
		TouchOnStart:   true,
	}

	logger, err := provideLogger(cfg)
	if err != nil {
		t.Fatalf("provideLogger returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}

	suffix := time.Now().Format("20060102")
	for _, name := range []string{"app.log", "error.log"} {
		path := filepath.Join(dir, name+"."+suffix)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected file %q to exist: %v", path, err)
		}
		if info.IsDir() {
			t.Fatalf("expected %q to be a file", path)
		}
	}
}
