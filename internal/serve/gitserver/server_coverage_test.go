package gitserver

import (
	"log/slog"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// Tests for filesystem-only functions that don't need SSH.

func TestGetCloneURL_Formats(t *testing.T) {
	tests := []struct {
		host string
		port int
		repo string
		want string
	}{
		{"127.0.0.1", 23231, "test-repo", "ssh://127.0.0.1:23231/test-repo"},
		{"example.com", 2222, "lib/skill", "ssh://example.com:2222/lib/skill"},
		{"localhost", 22, "", "ssh://localhost:22/"},
	}
	for _, tt := range tests {
		cfg := config.DefaultConfig()
		cfg.Server.Host = tt.host
		cfg.Server.Port = tt.port
		s := &Server{config: cfg}
		got := s.GetCloneURL(tt.repo)
		if got != tt.want {
			t.Errorf("GetCloneURL(%q) = %q, want %q", tt.repo, got, tt.want)
		}
	}
}

func TestGetConnectionInfo_Fields(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Host = "10.0.0.1"
	cfg.Server.Port = 3000
	s := &Server{config: cfg}

	info := s.GetConnectionInfo()

	if info.SSHHost != "10.0.0.1" {
		t.Errorf("SSHHost = %q, want %q", info.SSHHost, "10.0.0.1")
	}
	if info.SSHPort != 3000 {
		t.Errorf("SSHPort = %d, want %d", info.SSHPort, 3000)
	}
	if info.SSHUrl != "ssh://10.0.0.1:3000" {
		t.Errorf("SSHUrl = %q, want %q", info.SSHUrl, "ssh://10.0.0.1:3000")
	}
	if info.HTTPUrl != "http://10.0.0.1:3001" {
		t.Errorf("HTTPUrl = %q, want %q", info.HTTPUrl, "http://10.0.0.1:3001")
	}

	// Test clone functions
	if got := info.CloneSSH("my-repo"); got != "ssh://10.0.0.1:3000/my-repo" {
		t.Errorf("CloneSSH = %q", got)
	}
	if got := info.CloneHTTP("my-repo"); got != "http://10.0.0.1:3001/my-repo" {
		t.Errorf("CloneHTTP = %q", got)
	}
}

func TestCreateRepo_EmptyName(t *testing.T) {
	s := &Server{}
	if err := s.CreateRepo("", "desc"); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestDeleteRepo_EmptyName(t *testing.T) {
	s := &Server{}
	if err := s.DeleteRepo(""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestRunSSHCommand_NoAdminKey(t *testing.T) {
	s := &Server{adminKeyPath: ""}
	_, err := s.runSSHCommand("repo", "list")
	if err == nil {
		t.Fatal("expected error when admin key path not set")
	}
}

func TestSetSessionHooks(t *testing.T) {
	s := &Server{
		logger: slog.Default(),
	}
	s.SetSessionHooks(nil, nil)
	if s.onSessionStart != nil || s.onSessionEnd != nil {
		t.Fatal("expected nil hooks")
	}
}

func TestNewServer_NilConfig(t *testing.T) {
	_, err := NewServer(nil)
	if err == nil {
		t.Fatal("expected error with nil config")
	}
}

func TestStop_NotRunning(t *testing.T) {
	s := &Server{
		logger: slog.Default(),
	}
	// Stop without Start should be fine (cmd is nil)
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() on non-running server: %v", err)
	}
}
