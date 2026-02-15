package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Storage   StorageConfig   `yaml:"storage"`
	Libraries []LibraryConfig `yaml:"libraries"`
}

// ServerConfig contains server-related configuration
type ServerConfig struct {
	Port    int    `yaml:"port"`
	DataDir string `yaml:"dataDir"`
}

// StorageConfig contains storage backend configuration
type StorageConfig struct {
	Driver string `yaml:"driver"` // sqlite, postgres
	DSN    string `yaml:"dsn"`
}

// LibraryConfig represents a skill library configuration
type LibraryConfig struct {
	Name     string `yaml:"name"`
	Path     string `yaml:"path,omitempty"`
	URL      string `yaml:"url,omitempty"`
	Priority int    `yaml:"priority"`
}

// DefaultConfig returns a config with sane defaults
func DefaultConfig() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	dataDir := filepath.Join(home, ".mycelium", "data")
	dbPath := filepath.Join(home, ".mycelium", "mycelium.db")
	skillsPath := filepath.Join(home, ".mycelium", "skills")

	return &Config{
		Server: ServerConfig{
			Port:    23231,
			DataDir: dataDir,
		},
		Storage: StorageConfig{
			Driver: "sqlite",
			DSN:    dbPath,
		},
		Libraries: []LibraryConfig{
			{
				Name:     "local",
				Path:     skillsPath,
				Priority: 100,
			},
		},
	}
}

// Load loads configuration from the specified file path
// If the file doesn't exist, returns the default configuration
func Load(configPath string) (*Config, error) {
	// If no config path provided, use default location
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		configPath = filepath.Join(home, ".mycelium", "config.yaml")
	}

	// If config file doesn't exist, return defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	// Parse YAML
	config := DefaultConfig() // Start with defaults
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Save saves the configuration to the specified file path
func (c *Config) Save(configPath string) error {
	// If no config path provided, use default location
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}
		configPath = filepath.Join(home, ".mycelium", "config.yaml")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate server config
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("server port must be between 1 and 65535, got %d", c.Server.Port)
	}

	if c.Server.DataDir == "" {
		return fmt.Errorf("server data directory cannot be empty")
	}

	// Validate storage config
	if c.Storage.Driver != "sqlite" && c.Storage.Driver != "postgres" {
		return fmt.Errorf("storage driver must be 'sqlite' or 'postgres', got '%s'", c.Storage.Driver)
	}

	if c.Storage.DSN == "" {
		return fmt.Errorf("storage DSN cannot be empty")
	}

	// Validate libraries
	for i, lib := range c.Libraries {
		if lib.Name == "" {
			return fmt.Errorf("library[%d] name cannot be empty", i)
		}

		if lib.Path == "" && lib.URL == "" {
			return fmt.Errorf("library[%d] must have either path or URL", i)
		}

		if lib.Path != "" && lib.URL != "" {
			return fmt.Errorf("library[%d] cannot have both path and URL", i)
		}

		if lib.Priority < 0 {
			return fmt.Errorf("library[%d] priority cannot be negative", i)
		}
	}

	return nil
}