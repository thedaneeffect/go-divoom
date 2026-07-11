package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transport != "serial" || cfg.Channel != 1 || cfg.ListenAddr != ":8377" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	cfg.SerialPath = "/dev/cu.Pixoo-Max"
	cfg.MAC = "AA:BB:CC:DD:EE:FF"
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Errorf("round trip mismatch:\ngot  %+v\nwant %+v", got, cfg)
	}
}

func TestConfigPathCreated(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := saveConfig(Config{Transport: "serial"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go-divoom", "config.json")); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
