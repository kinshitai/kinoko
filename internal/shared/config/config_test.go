package config

import (
	"os"
	"os/user"
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
				Server:  ServerConfig{Host: "", Port: 8080, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 0, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 99999, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: ""},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
				Server:  ServerConfig{Host: "127.0.0.1", Port: 8080, DataDir: "/tmp"},
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
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
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
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
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
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")

	// Create config with tilde paths
	configContent := `storage:
  driver: sqlite
  dsn: ~/kinoko.db

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

	expectedDSN := filepath.Join(homeDir, "kinoko.db")
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

func TestExpandPathEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func() string // Function to compute expected result at test time
	}{
		{
			name:  "tilde alone",
			input: "~",
			expected: func() string {
				if homeDir, err := os.UserHomeDir(); err == nil {
					return homeDir
				}
				return "~" // fallback
			},
		},
		{
			name:  "current user home path",
			input: "~/path",
			expected: func() string {
				if homeDir, err := os.UserHomeDir(); err == nil {
					return filepath.Join(homeDir, "path")
				}
				return "~/path" // fallback
			},
		},
		{
			name:  "absolute path no expansion",
			input: "/absolute/path",
			expected: func() string {
				return "/absolute/path"
			},
		},
		{
			name:  "relative path no expansion",
			input: "relative/path",
			expected: func() string {
				return "relative/path"
			},
		},
		{
			name:  "tilde in middle no expansion",
			input: "/some/path/~/subdir",
			expected: func() string {
				return "/some/path/~/subdir"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPath(tt.input)
			expected := tt.expected()
			if result != expected {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, expected)
			}
		})
	}
}

func TestExpandPathUserLookup(t *testing.T) {
	// Test with current user (this should work on any system)
	currentUserHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get current user home directory, skipping user lookup tests")
	}

	// Get current username from user.Current()
	currentUser, err := user.Current()
	if err != nil {
		t.Skip("Cannot get current user info, skipping user lookup tests")
	}

	username := currentUser.Username

	// Test expanding ~username/path with current user
	userPath := "~" + username + "/test/path"
	result := expandPath(userPath)
	expected := filepath.Join(currentUserHome, "test", "path")

	// This might work or might not depending on system setup
	// If it doesn't work (returns original), that's also acceptable
	if result != userPath && result != expected {
		t.Errorf("expandPath(%q) = %q, expected either %q (unchanged) or %q (expanded)",
			userPath, result, userPath, expected)
	}
}

func TestExpandPathNonexistentUser(t *testing.T) {
	// Test with a user that definitely doesn't exist
	nonexistentPath := "~nonexistentuser12345/path"
	result := expandPath(nonexistentPath)

	// Should return the path unchanged since user doesn't exist
	if result != nonexistentPath {
		t.Errorf("expandPath(%q) = %q, expected %q (unchanged)", nonexistentPath, result, nonexistentPath)
	}
}

func TestConfigPartialMerging(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name           string
		configContent  string
		validateResult func(*testing.T, *Config)
	}{
		{
			name: "only server section",
			configContent: `server:
  host: 192.168.1.100
  port: 9999
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Server section should be overridden
				if cfg.Server.Host != "192.168.1.100" {
					t.Errorf("expected host '192.168.1.100', got '%s'", cfg.Server.Host)
				}
				if cfg.Server.Port != 9999 {
					t.Errorf("expected port 9999, got %d", cfg.Server.Port)
				}

				// Other sections should use defaults
				if cfg.Storage.Driver != "sqlite" {
					t.Errorf("expected default storage driver 'sqlite', got '%s'", cfg.Storage.Driver)
				}
				if len(cfg.Libraries) == 0 {
					t.Error("expected default libraries to be present")
				}
				if cfg.Extraction.MinConfidence != 0.5 {
					t.Errorf("expected default min_confidence 0.5, got %f", cfg.Extraction.MinConfidence)
				}
			},
		},
		{
			name: "only storage section",
			configContent: `storage:
  driver: postgres
  dsn: "postgres://user:pass@localhost/kinoko"
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Storage section should be overridden
				if cfg.Storage.Driver != "postgres" {
					t.Errorf("expected driver 'postgres', got '%s'", cfg.Storage.Driver)
				}
				if cfg.Storage.DSN != "postgres://user:pass@localhost/kinoko" {
					t.Errorf("expected postgres DSN, got '%s'", cfg.Storage.DSN)
				}

				// Other sections should use defaults
				if cfg.Server.Host != "127.0.0.1" {
					t.Errorf("expected default host '127.0.0.1', got '%s'", cfg.Server.Host)
				}
				if cfg.Server.Port != 23231 {
					t.Errorf("expected default port 23231, got %d", cfg.Server.Port)
				}
			},
		},
		{
			name: "only libraries section",
			configContent: `libraries:
  - name: custom
    url: "https://github.com/example/skills"
    priority: 200
  - name: other  
    path: "/opt/skills"
    priority: 150
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Libraries should be completely replaced (not merged with defaults)
				if len(cfg.Libraries) != 2 {
					t.Errorf("expected 2 libraries, got %d", len(cfg.Libraries))
				}
				if cfg.Libraries[0].Name != "custom" {
					t.Errorf("expected first library name 'custom', got '%s'", cfg.Libraries[0].Name)
				}
				if cfg.Libraries[1].Name != "other" {
					t.Errorf("expected second library name 'other', got '%s'", cfg.Libraries[1].Name)
				}

				// Other sections should use defaults
				if cfg.Server.Port != 23231 {
					t.Errorf("expected default port 23231, got %d", cfg.Server.Port)
				}
				if cfg.Storage.Driver != "sqlite" {
					t.Errorf("expected default storage driver 'sqlite', got '%s'", cfg.Storage.Driver)
				}
			},
		},
		{
			name: "partial extraction config",
			configContent: `extraction:
  min_confidence: 0.8
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Specified extraction fields should be overridden
				if cfg.Extraction.MinConfidence != 0.8 {
					t.Errorf("expected min_confidence 0.8, got %f", cfg.Extraction.MinConfidence)
				}

				// Other extraction fields should use defaults
				if !cfg.Extraction.AutoExtract {
					t.Error("expected default auto_extract to be true")
				}
				if !cfg.Extraction.RequireValidation {
					t.Error("expected default require_validation to be true")
				}
			},
		},
		{
			name: "empty config file",
			configContent: `# Just a comment
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Should be identical to defaults
				defaultCfg := DefaultConfig()
				if cfg.Server.Host != defaultCfg.Server.Host {
					t.Errorf("expected default host '%s', got '%s'", defaultCfg.Server.Host, cfg.Server.Host)
				}
				if cfg.Server.Port != defaultCfg.Server.Port {
					t.Errorf("expected default port %d, got %d", defaultCfg.Server.Port, cfg.Server.Port)
				}
				if cfg.Storage.Driver != defaultCfg.Storage.Driver {
					t.Errorf("expected default driver '%s', got '%s'", defaultCfg.Storage.Driver, cfg.Storage.Driver)
				}
			},
		},
		{
			name: "mixed partial config",
			configContent: `server:
  port: 8888

storage:
  driver: postgres
  dsn: "postgres://localhost/test"
  
defaults:
  confidence: 0.9
`,
			validateResult: func(t *testing.T, cfg *Config) {
				// Specified fields should be overridden
				if cfg.Server.Port != 8888 {
					t.Errorf("expected port 8888, got %d", cfg.Server.Port)
				}
				if cfg.Storage.Driver != "postgres" {
					t.Errorf("expected driver 'postgres', got '%s'", cfg.Storage.Driver)
				}
				if cfg.Defaults.Confidence != 0.9 {
					t.Errorf("expected defaults confidence 0.9, got %f", cfg.Defaults.Confidence)
				}

				// Unspecified fields should use defaults
				if cfg.Server.Host != "127.0.0.1" {
					t.Errorf("expected default host '127.0.0.1', got '%s'", cfg.Server.Host)
				}
				if len(cfg.Libraries) == 0 {
					t.Error("expected default libraries to be present")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(tempDir, tt.name+".yaml")

			if err := os.WriteFile(configPath, []byte(tt.configContent), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			config, err := Load(configPath)
			if err != nil {
				t.Fatalf("failed to load config: %v", err)
			}

			tt.validateResult(t, config)
		})
	}
}

func TestExpandPathsSSHKeyPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `client:
  ssh_key_path: "~/.kinoko/id_ed25519"

server:
  host: 127.0.0.1
  port: 8080
  dataDir: /tmp

storage:
  driver: sqlite
  dsn: /tmp/test.db

libraries:
  - name: test
    path: /tmp/skills
    priority: 100
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	expected := filepath.Join(homeDir, ".kinoko", "id_ed25519")
	if config.Client.SSHKeyPath != expected {
		t.Errorf("expected SSHKeyPath %q, got %q", expected, config.Client.SSHKeyPath)
	}
}

func TestGetSSHKeyPath_Default(t *testing.T) {
	c := &ClientConfig{}
	got := c.GetSSHKeyPath()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	expected := filepath.Join(home, ".kinoko", "id_ed25519")
	if got != expected {
		t.Errorf("expected default %q, got %q", expected, got)
	}
}

func TestGetSSHKeyPath_Custom(t *testing.T) {
	c := &ClientConfig{SSHKeyPath: "/custom/path/mykey"}
	got := c.GetSSHKeyPath()
	if got != "/custom/path/mykey" {
		t.Errorf("expected /custom/path/mykey, got %q", got)
	}
}

func TestRegistrationTokenLoading(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `server:
  host: 127.0.0.1
  port: 8080
  dataDir: /tmp
  registrationToken: "my-secret-token-123"

storage:
  driver: sqlite
  dsn: /tmp/test.db

libraries:
  - name: test
    path: /tmp/skills
    priority: 100
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if config.Server.RegistrationToken != "my-secret-token-123" {
		t.Errorf("expected RegistrationToken %q, got %q", "my-secret-token-123", config.Server.RegistrationToken)
	}
}

func TestRegistrationTokenEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kinoko-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.yaml")
	configContent := `server:
  host: 127.0.0.1
  port: 8080
  dataDir: /tmp

storage:
  driver: sqlite
  dsn: /tmp/test.db

libraries:
  - name: test
    path: /tmp/skills
    priority: 100
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if config.Server.RegistrationToken != "" {
		t.Errorf("expected empty RegistrationToken, got %q", config.Server.RegistrationToken)
	}
}
