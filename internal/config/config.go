package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	configDir  = ".cipi"
	configFile = "config.json"
)

type Config struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, configDir)
}

func Path() string {
	return filepath.Join(Dir(), configFile)
}

func Load() (*Config, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not configured — run 'cipi-cli configure' first")
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Endpoint == "" || cfg.Token == "" {
		return nil, fmt.Errorf("incomplete configuration — run 'cipi-cli configure' to fix")
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(Path(), data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
