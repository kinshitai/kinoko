package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test that defaults are set correctly
	if config.Server.Host != "127.0.0.1" {
		t.Errorf("expected default host '127.0.0.1', got '%s'", config.Server.Host)
	}

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

	// Test extraction defaults
	if !config.Extraction.AutoExtract {
		t.Error("expected default auto_extract to be true")
	}

	if config.Extraction.MinConfidence != 0.5 {
		t.Errorf("expected default min_confidence 0.5, got %f", config.Extraction.MinConfidence)
	}

	// Test hooks defaults
	if !config.Hooks.CredentialScan {
		t.Error("expected default credential_scan to be true")
	}

	// Test defaults section
	if config.Defaults.Confidence != 0.7 {
		t.Errorf("expected default confidence 0.7, got %f", config.Defaults.Confidence)
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
			name: "empty host",
			config: &Config{
				Server: ServerConfig{Host: "", Port: 8080, DataDir: "/tmp"},
				Storage: StorageConfig{Driver: "sqlite", DSN: "/tmp/test.db"},
				Libraries: []LibraryConfig{
					{Name: "test", Path: "/tmp", Priority: 1},
				},
			},
			expectError: true,
		},
		{
			name: "invalid port - too low",
			config: &Config{
				Server: ServerConfig{Host: "127.0.0.1", Port: 0, DataDir: "/tmp"},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 99999, DataDir: "/tmp"},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: ""},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server: ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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

func TestTildeExpansion(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "mycelium-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	// Create config with tilde paths
	configContent := `storage:
  driver: sqlite
  dsn: ~/mycelium.db

server:
  host: 127.0.0.1
  port: 8080
  dataDir: ~/data

libraries:
  - name: test
    path: ~/skills
    priority: 100
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config and test tilde expansion
	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Check that tildes were expanded
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	expectedDSN := filepath.Join(homeDir, "mycelium.db")
	expectedDataDir := filepath.Join(homeDir, "data")
	expectedLibPath := filepath.Join(homeDir, "skills")

	if config.Storage.DSN != expectedDSN {
		t.Errorf("expected DSN '%s', got '%s'", expectedDSN, config.Storage.DSN)
	}

	if config.Server.DataDir != expectedDataDir {
		t.Errorf("expected DataDir '%s', got '%s'", expectedDataDir, config.Server.DataDir)
	}

	if config.Libraries[0].Path != expectedLibPath {
		t.Errorf("expected library path '%s', got '%s'", expectedLibPath, config.Libraries[0].Path)
	}
}