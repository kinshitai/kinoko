package gitserver

import (
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// TestCreateUser_NoServer verifies CreateUser returns an error when
// no SSH connection is available (no running Soft Serve).
func TestCreateUser_NoServer(t *testing.T) {
	cfg := config.DefaultConfig()
	// Skip if soft binary is not available.
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("soft binary not available")
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// adminKeyPath is empty since Start() was never called → runSSHCommand fails.
	err = srv.CreateUser("test-user")
	if err == nil {
		t.Fatal("expected error without running server")
	}
}

// TestAddUserPubkey_NoServer verifies AddUserPubkey returns an error when
// no SSH connection is available.
func TestAddUserPubkey_NoServer(t *testing.T) {
	cfg := config.DefaultConfig()
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("soft binary not available")
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = srv.AddUserPubkey("test-user", "ssh-ed25519 AAAA")
	if err == nil {
		t.Fatal("expected error without running server")
	}
}

// TestAddCollab_NoServer verifies AddCollab returns an error when
// no SSH connection is available.
func TestAddCollab_NoServer(t *testing.T) {
	cfg := config.DefaultConfig()
	if _, err := CheckSoftBinary(); err != nil {
		t.Skip("soft binary not available")
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	err = srv.AddCollab("my-repo", "test-user", "read-write")
	if err == nil {
		t.Fatal("expected error without running server")
	}
}

// TestCombineKeys verifies the CombineKeys helper for additional keys.
func TestCombineKeys(t *testing.T) {
	admin := "ssh-ed25519 ADMIN-KEY"
	extra := []string{"ssh-ed25519 EXTRA1", "ssh-ed25519 EXTRA2"}
	combined := CombineKeys(admin, extra)
	if !strings.Contains(combined, "ADMIN-KEY") {
		t.Error("missing admin key")
	}
	if !strings.Contains(combined, "EXTRA1") {
		t.Error("missing extra key 1")
	}
	if !strings.Contains(combined, "EXTRA2") {
		t.Error("missing extra key 2")
	}
}
