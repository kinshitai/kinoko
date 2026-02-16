package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigPath(t *testing.T) {
	p := DefaultConfigPath()
	if !strings.Contains(p, ".kinoko") {
		t.Fatalf("DefaultConfigPath() = %q, expected to contain .kinoko", p)
	}
	if !strings.HasSuffix(p, "config.yaml") {
		t.Fatalf("DefaultConfigPath() = %q, expected config.yaml suffix", p)
	}
}

func TestLoadLocalConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `client:
  server: "ssh://localhost:23231"
  api: "http://localhost:23232"
  cache_dir: "/tmp/cache"
  pull_interval: "5m"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadLocalConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Client.Server != "ssh://localhost:23231" {
		t.Fatalf("Server = %q", cfg.Client.Server)
	}
	if cfg.Client.API != "http://localhost:23232" {
		t.Fatalf("API = %q", cfg.Client.API)
	}
	if cfg.Client.CacheDir != "/tmp/cache" {
		t.Fatalf("CacheDir = %q", cfg.Client.CacheDir)
	}
	if cfg.Client.PullInterval != "5m" {
		t.Fatalf("PullInterval = %q", cfg.Client.PullInterval)
	}
}

func TestLoadLocalConfig_NotFound(t *testing.T) {
	_, err := LoadLocalConfig("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadLocalConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte("invalid: yaml: ["), 0644)

	_, err := LoadLocalConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSaveClientConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	section := ClientSection{
		Server:       "ssh://localhost:23231",
		API:          "http://localhost:23232",
		CacheDir:     "/tmp/cache",
		PullInterval: "10m",
	}

	if err := SaveClientConfig(path, section); err != nil {
		t.Fatal(err)
	}

	// Read back and verify.
	cfg, err := LoadLocalConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Client.Server != section.Server {
		t.Fatalf("Server mismatch: got %q", cfg.Client.Server)
	}
	if cfg.Client.PullInterval != section.PullInterval {
		t.Fatalf("PullInterval mismatch: got %q", cfg.Client.PullInterval)
	}
}

func TestSaveClientConfig_PreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write initial content with extra section.
	initial := `extra:
  key: value
client:
  server: old
`
	os.WriteFile(path, []byte(initial), 0644)

	// Save new client config.
	if err := SaveClientConfig(path, ClientSection{Server: "new"}); err != nil {
		t.Fatal(err)
	}

	// Verify extra section preserved.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "extra") {
		t.Fatal("expected extra section to be preserved")
	}
}
