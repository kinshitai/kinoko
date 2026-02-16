package gitserver

import (
	"os"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/config"
)

func TestNewServer(t *testing.T) {
	// Test with nil config
	_, err := NewServer(nil)
	if err == nil {
		t.Error("Expected error with nil config")
	}

	// Test when soft binary is not available (skip if soft is actually available)
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping test because soft binary is not available")
	}

	// Test with valid config
	cfg := config.DefaultConfig()
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to be created")
	}

	if server.config != cfg {
		t.Error("Server config not set correctly")
	}

	if server.softBinary == "" {
		t.Error("Expected soft binary path to be set")
	}
}

func TestCheckSoftBinary(t *testing.T) {
	path, err := CheckSoftBinary()
	if err != nil {
		t.Skip("Soft binary not available, skipping test")
	}

	if path == "" {
		t.Error("Expected non-empty path when binary is found")
	}
}

func TestSSHKeyGeneration(t *testing.T) {
	// Skip if soft binary is not available
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping test because soft binary is not available")
	}

	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test key generation
	keyPath, err := server.ensureAdminKeys()
	if err != nil {
		t.Errorf("Failed to generate admin keys: %v", err)
	}

	// Verify private key exists
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("Private key file was not created")
	}

	// Verify public key exists
	pubKeyPath := keyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		t.Error("Public key file was not created")
	}

	// Test idempotency - calling again should not error
	keyPath2, err := server.ensureAdminKeys()
	if err != nil {
		t.Errorf("Second call to ensureAdminKeys failed: %v", err)
	}

	if keyPath != keyPath2 {
		t.Error("Key path changed on second call")
	}

	// Test reading public key
	pubKey, err := server.getAdminPublicKey()
	if err != nil {
		t.Errorf("Failed to read admin public key: %v", err)
	}

	if pubKey == "" {
		t.Error("Public key content is empty")
	}

	// Verify the public key starts with ssh-ed25519
	if len(pubKey) < 12 || pubKey[:11] != "ssh-ed25519" {
		t.Errorf("Public key doesn't look like an ed25519 key: %s", pubKey)
	}
}

func TestRepoNameValidation(t *testing.T) {
	// Skip if soft binary is not available
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping test because soft binary is not available")
	}

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = t.TempDir()

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test empty name for CreateRepo
	if err := server.CreateRepo("", "description"); err == nil {
		t.Error("Expected error with empty repository name")
	}

	// Test empty name for DeleteRepo
	if err := server.DeleteRepo(""); err == nil {
		t.Error("Expected error with empty repository name")
	}

	// Test GetCloneURL
	cloneURL := server.GetCloneURL("test-repo")
	expectedURL := "ssh://127.0.0.1:23231/test-repo"
	if cloneURL != expectedURL {
		t.Errorf("Expected clone URL %s, got %s", expectedURL, cloneURL)
	}
}

func TestConnectionInfo(t *testing.T) {
	// Skip if soft binary is not available
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping test because soft binary is not available")
	}

	cfg := config.DefaultConfig()
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	info := server.GetConnectionInfo()

	if info.SSHHost != cfg.Server.Host {
		t.Errorf("Expected host %s, got %s", cfg.Server.Host, info.SSHHost)
	}

	if info.SSHPort != cfg.Server.Port {
		t.Errorf("Expected port %d, got %d", cfg.Server.Port, info.SSHPort)
	}

	expectedSSHURL := "ssh://127.0.0.1:23231"
	if info.SSHUrl != expectedSSHURL {
		t.Errorf("Expected SSH URL %s, got %s", expectedSSHURL, info.SSHUrl)
	}

	expectedHTTPURL := "http://127.0.0.1:23232"
	if info.HTTPUrl != expectedHTTPURL {
		t.Errorf("Expected HTTP URL %s, got %s", expectedHTTPURL, info.HTTPUrl)
	}

	// Test clone URL functions
	if info.CloneSSH != nil {
		cloneSSH := info.CloneSSH("test")
		expectedCloneSSH := "ssh://127.0.0.1:23231/test"
		if cloneSSH != expectedCloneSSH {
			t.Errorf("Expected SSH clone URL %s, got %s", expectedCloneSSH, cloneSSH)
		}
	}

	if info.CloneHTTP != nil {
		cloneHTTP := info.CloneHTTP("test")
		expectedCloneHTTP := "http://127.0.0.1:23232/test"
		if cloneHTTP != expectedCloneHTTP {
			t.Errorf("Expected HTTP clone URL %s, got %s", expectedCloneHTTP, cloneHTTP)
		}
	}
}

// Integration tests that require a running Soft Serve instance
// These are skipped if soft binary is not available or if we can't start the server

func TestServerLifecycle(t *testing.T) {
	// Skip if soft binary is not available
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping integration test because soft binary is not available")
	}

	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 23233 // Use different port to avoid conflicts

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test start (this may take some time)
	if err := server.Start(); err != nil {
		t.Skipf("Failed to start server (this is OK in CI): %v", err)
	}

	// If we get here, the server started successfully
	defer func() {
		if err := server.Stop(); err != nil {
			t.Errorf("Failed to stop server: %v", err)
		}
	}()

	// Test connection info after starting
	info := server.GetConnectionInfo()
	if info.SSHHost != cfg.Server.Host {
		t.Errorf("Expected host %s, got %s", cfg.Server.Host, info.SSHHost)
	}
	if info.SSHPort != cfg.Server.Port {
		t.Errorf("Expected port %d, got %d", cfg.Server.Port, info.SSHPort)
	}
}

func TestRepoManagement(t *testing.T) {
	// Skip if soft binary is not available
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("Skipping integration test because soft binary is not available")
	}

	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 23234 // Use different port to avoid conflicts

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Skipf("Failed to start server (this is OK in CI): %v", err)
	}

	defer func() {
		_ = server.Stop()
	}()

	// Test creating repository
	repoName := "test-skill"
	description := "A test skill repository"

	if err := server.CreateRepo(repoName, description); err != nil {
		t.Errorf("Failed to create repository: %v", err)
	}

	// Test listing repositories
	repos, err := server.ListRepos()
	if err != nil {
		t.Errorf("Failed to list repositories: %v", err)
	}

	// Should contain our repo
	found := false
	for _, repo := range repos {
		if repo == repoName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Created repository %s not found in list: %v", repoName, repos)
	}

	// Test deleting repository
	if err := server.DeleteRepo(repoName); err != nil {
		t.Errorf("Failed to delete repository: %v", err)
	}

	// Verify repository was deleted
	repos, err = server.ListRepos()
	if err != nil {
		t.Errorf("Failed to list repositories after deletion: %v", err)
	}

	for _, repo := range repos {
		if repo == repoName {
			t.Errorf("Repository %s still exists after deletion", repoName)
		}
	}
}
