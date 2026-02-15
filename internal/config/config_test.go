package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test that defaults are set correctly
	if config.Server.Port != 23231 {
		t.Errorf("expected default port 23231, got %d", config.Server.Port)
	}

	if config.Server.DataDir == "" {
		t.Error("expected default data dir to be set")
	}

	if config.Storage.Driver != "sqlite" {
		t.Errorf("expected default storage driver 'sqlite', got '%s'", config.Storage.Driver)
	}

	if config.Storage.DSN == "" {
		t.Error("expected default DSN to be set")
	}

	if len(config.Libraries) == 0 {
		t.Error("expected at least one default library")
	}

	if config.Libraries[0].Name != "local" {
		t.Errorf("expected first library name 'local', got '%s'", config.Libraries[0].Name)
	}

	if config.Libraries[0].Priority != 100 {
		t.Errorf("expected first library priority 100, got %d", config.Libraries[0].Priority)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name:        "valid config",
			config:      DefaultConfig(),
			expectError: false,
		},
		{
			name: "invalid port - too low",
			config: &Config{
				Server: ServerConfig{Port: 0, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "invalid port - too high",
			config: &Config{
				Server: ServerConfig{Port: 99999, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "empty data dir",
			config: &Config{
				Server: ServerConfig{Port: 8080, DataDir: ""},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "invalid storage driver",
			config: &Config{
				Server: ServerConfig{Port: 8080, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "redis", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "empty library name",
			config: &Config{
				Server: ServerConfig{Port: 8080, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "library with both path and URL",
			config: &Config{
				Server: ServerConfig{Port: 8080, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", URL: "https://example.com", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "library with neither path nor URL",
			config: &Config{
				Server: ServerConfig{Port: 8080, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Priority: 1},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected validation error, got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no validation error, got: %v", err)
			}
		})
	}
}

func TestConfigLoadAndSave(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "mycelium-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	// Test loading non-existent config returns defaults
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load non-existent config: %v", err)
	}

	if config.Server.Port != 23231 {
		t.Error("loaded config should have default values")
	}

	// Test saving config
	config.Server.Port = 9999
	config.Libraries = append(config.Libraries, LibraryConfig{
		Name:     "test",
		URL:      "https://example.com",
		Priority: 50,
	})

	if err := config.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Test loading saved config
	loadedConfig, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loadedConfig.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loadedConfig.Server.Port)
	}

	if len(loadedConfig.Libraries) != 2 {
		t.Errorf("expected 2 libraries, got %d", len(loadedConfig.Libraries))
	}

	if loadedConfig.Libraries[1].Name != "test" {
		t.Errorf("expected second library name 'test', got '%s'", loadedConfig.Libraries[1].Name)
	}
}

func TestConfigLoadInvalidYAML(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "mycelium-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "invalid.yaml")

	// Write invalid YAML
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644); err != nil {
		t.Fatalf("failed to write invalid YAML: %v", err)
	}

	// Test loading invalid config
	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error when loading invalid YAML, got none")
	}
}