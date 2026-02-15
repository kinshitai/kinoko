package gitserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mycelium-dev/mycelium/internal/config"
)

func TestNewServer(t *testing.T) {
	// Test with nil config
	_, err := NewServer(nil)
	if err == nil {
		t.Error("Expected error with nil config")
	}

	// Test with valid config
	cfg := config.DefaultConfig()
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if server == nil {
		t.Error("Expected server to be created")
	}

	if server.config != cfg {
		t.Error("Server config not set correctly")
	}

	// Cleanup
	server.Stop()
}

func TestServerLifecycle(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test start
	if err := server.Start(); err != nil {
		t.Errorf("Failed to start server: %v", err)
	}

	// Test connection info
	info := server.GetConnectionInfo()
	if info.SSHHost != cfg.Server.Host {
		t.Errorf("Expected host %s, got %s", cfg.Server.Host, info.SSHHost)
	}
	if info.SSHPort != cfg.Server.Port {
		t.Errorf("Expected port %d, got %d", cfg.Server.Port, info.SSHPort)
	}

	// Test stop
	if err := server.Stop(); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}
}

func TestRepoManagement(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test creating repository
	repoName := "test-skill"
	description := "A test skill repository"
	
	if err := server.CreateRepo(repoName, description); err != nil {
		t.Errorf("Failed to create repository: %v", err)
	}

	// Verify repository directory was created
	expectedPath := filepath.Join(tmpDir, "repos", repoName+".git")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Repository directory was not created at %s", expectedPath)
	}

	// Test listing repositories
	repos, err := server.ListRepos()
	if err != nil {
		t.Errorf("Failed to list repositories: %v", err)
	}

	if len(repos) != 1 {
		t.Errorf("Expected 1 repository, got %d", len(repos))
	}

	if repos[0] != repoName {
		t.Errorf("Expected repository %s, got %s", repoName, repos[0])
	}

	// Test deleting repository
	if err := server.DeleteRepo(repoName); err != nil {
		t.Errorf("Failed to delete repository: %v", err)
	}

	// Verify repository was deleted
	if _, err := os.Stat(expectedPath); !os.IsNotExist(err) {
		t.Error("Repository directory still exists after deletion")
	}

	// Test listing after deletion
	repos, err = server.ListRepos()
	if err != nil {
		t.Errorf("Failed to list repositories after deletion: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories after deletion, got %d", len(repos))
	}
}

func TestCreateRepoValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = t.TempDir()

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test empty name
	if err := server.CreateRepo("", "description"); err == nil {
		t.Error("Expected error with empty repository name")
	}
}

func TestDeleteRepoValidation(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = t.TempDir()

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test empty name
	if err := server.DeleteRepo(""); err == nil {
		t.Error("Expected error with empty repository name")
	}
}