package config

import (
	"path/filepath"
	"testing"
)

func TestLoadOrCreateDefaultConfigUsesSecureFileAndDefaults(t *testing.T) {
	homeDir := t.TempDir()
	cfg, path, err := LoadOrCreate("", homeDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(homeDir, "config.toml") {
		t.Fatalf("unexpected path %q", path)
	}
	if cfg.Server.Bind != "127.0.0.1:11435" {
		t.Fatalf("unexpected bind %q", cfg.Server.Bind)
	}
	if len(cfg.Providers) != 3 {
		t.Fatalf("expected default provider placeholders, got %d", len(cfg.Providers))
	}
}
