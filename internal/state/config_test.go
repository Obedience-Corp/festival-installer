package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(context.Background(), dir)
	if err != nil {
		t.Fatalf("want nil error for missing file, got %v", err)
	}
	if cfg.Marketplaces.Default != "official-obey" {
		t.Fatalf("want default marketplace, got %q", cfg.Marketplaces.Default)
	}
}

func TestLoadConfig_Malformed(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "this is not = toml [[[")
	_, err := LoadConfig(context.Background(), dir)
	var e *errpkg.Error
	if !errors.As(err, &e) || e.Code != "E_CONFIG_PARSE" {
		t.Fatalf("want E_CONFIG_PARSE, got %v", err)
	}
}

func TestLoadConfig_Valid(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `
[telemetry]
enabled = true

[marketplaces]
default = "acme"
`)
	cfg, err := LoadConfig(context.Background(), dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Fatal("want telemetry enabled")
	}
	if cfg.Marketplaces.Default != "acme" {
		t.Fatalf("want acme, got %q", cfg.Marketplaces.Default)
	}
}

func TestLoadConfig_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "[telemetry]\nenabled = false\n")
	cfg, err := LoadConfig(context.Background(), dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Marketplaces.Default != "official-obey" {
		t.Fatalf("want default fallback, got %q", cfg.Marketplaces.Default)
	}
}
