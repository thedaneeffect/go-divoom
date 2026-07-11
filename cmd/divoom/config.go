package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Config is persisted server/CLI configuration.
type Config struct {
	Transport  string `json:"transport"` // "serial" or "rfcomm"
	SerialPath string `json:"serialPath"`
	MAC        string `json:"mac"`
	Channel    uint8  `json:"channel"`
	ListenAddr string `json:"listenAddr"`
}

func defaultConfig() Config {
	return Config{Transport: "serial", Channel: 1, ListenAddr: ":8377"}
}

// configPath honors XDG_CONFIG_HOME on all platforms (os.UserConfigDir
// ignores it on macOS, which breaks hermetic tests and surprises CLI users).
func configPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve config dir: %w", err)
		}
	}
	return filepath.Join(base, "go-divoom", "config.json"), nil
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func saveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
