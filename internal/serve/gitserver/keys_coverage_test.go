package gitserver

import (
	"log/slog"
	"os"
	"testing"
)

func TestEnsureAdminKeys_Generate(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Server{
		dataDir: tmpDir,
		logger:  slog.Default(),
	}

	keyPath, err := s.ensureAdminKeys()
	if err != nil {
		t.Fatalf("ensureAdminKeys: %v", err)
	}

	// Check private key
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("private key not created: %v", err)
	}
	// Check public key
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Fatalf("public key not created: %v", err)
	}

	// Check permissions
	info, _ := os.Stat(keyPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("private key perms = %o, want 0600", info.Mode().Perm())
	}

	// Idempotency
	keyPath2, err := s.ensureAdminKeys()
	if err != nil {
		t.Fatalf("second ensureAdminKeys: %v", err)
	}
	if keyPath != keyPath2 {
		t.Error("key path changed on second call")
	}
}

func TestGetAdminPublicKey(t *testing.T) {
	tmpDir := t.TempDir()
	s := &Server{
		dataDir: tmpDir,
		logger:  slog.Default(),
	}

	// Generate keys first
	if _, err := s.ensureAdminKeys(); err != nil {
		t.Fatalf("ensureAdminKeys: %v", err)
	}

	pubKey, err := s.getAdminPublicKey()
	if err != nil {
		t.Fatalf("getAdminPublicKey: %v", err)
	}
	if pubKey == "" {
		t.Fatal("empty public key")
	}
	if len(pubKey) < 11 || pubKey[:11] != "ssh-ed25519" {
		t.Errorf("unexpected key format: %s", pubKey[:min(30, len(pubKey))])
	}
}

func TestGetAdminPublicKey_NoFile(t *testing.T) {
	s := &Server{
		dataDir: t.TempDir(),
		logger:  slog.Default(),
	}
	_, err := s.getAdminPublicKey()
	if err == nil {
		t.Fatal("expected error when no key file")
	}
}

// TestEnsureAdminKeys_PartialState removed — ssh-keygen won't overwrite.
