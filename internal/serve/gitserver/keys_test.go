package gitserver

import (
	"testing"
)

func TestCombineKeys_AdminOnly(t *testing.T) {
	got := CombineKeys("ssh-ed25519 AAAA admin@host", nil)
	want := "ssh-ed25519 AAAA admin@host"
	if got != want {
		t.Errorf("CombineKeys admin only = %q, want %q", got, want)
	}
}

func TestCombineKeys_AdminPlusClient(t *testing.T) {
	got := CombineKeys("ssh-ed25519 AAAA admin@host", []string{"ssh-ed25519 BBBB client@host"})
	want := "ssh-ed25519 AAAA admin@host\nssh-ed25519 BBBB client@host"
	if got != want {
		t.Errorf("CombineKeys admin+client = %q, want %q", got, want)
	}
}

func TestCombineKeys_MultipleAdditional(t *testing.T) {
	got := CombineKeys("ssh-ed25519 AAAA admin@host", []string{
		"ssh-ed25519 BBBB client1@host",
		"ssh-ed25519 CCCC client2@host",
	})
	want := "ssh-ed25519 AAAA admin@host\nssh-ed25519 BBBB client1@host\nssh-ed25519 CCCC client2@host"
	if got != want {
		t.Errorf("CombineKeys multiple = %q, want %q", got, want)
	}
}

func TestCombineKeys_EmptyAdditional(t *testing.T) {
	got := CombineKeys("ssh-ed25519 AAAA admin@host", []string{})
	want := "ssh-ed25519 AAAA admin@host"
	if got != want {
		t.Errorf("CombineKeys empty additional = %q, want %q", got, want)
	}
}

func TestCombineKeys_WhitespaceInKeys(t *testing.T) {
	got := CombineKeys("  ssh-ed25519 AAAA admin@host\n", []string{
		"  ssh-ed25519 BBBB client@host  \n",
	})
	want := "ssh-ed25519 AAAA admin@host\nssh-ed25519 BBBB client@host"
	if got != want {
		t.Errorf("CombineKeys whitespace = %q, want %q", got, want)
	}
}

func TestCombineKeys_EmptyAdminKey(t *testing.T) {
	// Empty admin key with additional keys — the admin is trimmed to "",
	// so result starts with \n which is messy. Documents current behavior.
	got := CombineKeys("", []string{"ssh-ed25519 BBBB client@host"})
	want := "\nssh-ed25519 BBBB client@host"
	if got != want {
		t.Errorf("CombineKeys empty admin = %q, want %q", got, want)
	}
}

func TestCombineKeys_AllEmptyAdditional(t *testing.T) {
	// Additional keys that are all whitespace should be skipped.
	got := CombineKeys("ssh-ed25519 AAAA admin@host", []string{"", "  ", "\n"})
	want := "ssh-ed25519 AAAA admin@host"
	if got != want {
		t.Errorf("CombineKeys all-empty additional = %q, want %q", got, want)
	}
}

func TestCombineKeys_NilAdditional(t *testing.T) {
	// Explicit nil (vs empty slice) — same as no additional keys.
	got := CombineKeys("ssh-ed25519 AAAA admin@host", nil)
	want := "ssh-ed25519 AAAA admin@host"
	if got != want {
		t.Errorf("CombineKeys nil additional = %q, want %q", got, want)
	}
}
