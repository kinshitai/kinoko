package client

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LocalConfig is the client-side config stored at ~/.kinoko/config.yaml.
type LocalConfig struct {
	Client ClientSection `yaml:"client"`
}

// ClientSection holds client connection settings.
type ClientSection struct {
	Server       string `yaml:"server"`        // SSH URL for git
	API          string `yaml:"api"`            // HTTP API URL
	CacheDir     string `yaml:"cache_dir"`      // Local cache dir
	PullInterval string `yaml:"pull_interval"`  // e.g. "5m"
}

// DefaultConfigPath returns ~/.kinoko/config.yaml.
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kinoko", "config.yaml")
}

// LoadLocalConfig reads client config from the given path.
func LoadLocalConfig(path string) (*LocalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg LocalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// SaveClientConfig writes or updates the client section in config.yaml.
// It preserves existing server-side config if present.
func SaveClientConfig(path string, client ClientSection) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Try to load existing config to preserve other sections
	var raw map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &raw)
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	// Set client section
	raw["client"] = map[string]any{
		"server":        client.Server,
		"api":           client.API,
		"cache_dir":     client.CacheDir,
		"pull_interval": client.PullInterval,
	}

	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
