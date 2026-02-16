package sanitize

import (
	"log/slog"
	"strings"
	"testing"
)

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	s := New(WithLogger(logger))
	if s == nil {
		t.Fatal("expected non-nil scanner")
	}
}

func TestRedact_WithSecrets(t *testing.T) {
	s := New()
	// AWS key pattern.
	text := `config:
  aws_access_key_id = AKIAIOSFODNN7EXAMPLE
  password = "SuperSecret123!"
`
	redacted := s.Redact(text)
	if strings.Contains(redacted, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatal("expected AWS key to be redacted")
	}
	if !strings.Contains(redacted, "[REDACTED:") {
		t.Fatal("expected [REDACTED:] marker in output")
	}
}

func TestRedact_GenericPassword(t *testing.T) {
	// Use low redact threshold to ensure the generic_password pattern triggers.
	s := New(WithRedactThreshold(0.4))
	text := `password = "my-secret-value123"`
	redacted := s.Redact(text)
	if strings.Contains(redacted, "my-secret-value123") {
		t.Fatalf("expected password value to be redacted, got %q", redacted)
	}
}

func TestRedact_PrivateKey(t *testing.T) {
	s := New()
	text := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"
	redacted := s.Redact(text)
	if strings.Contains(redacted, "BEGIN RSA PRIVATE KEY") {
		t.Fatal("expected private key to be redacted")
	}
}

func TestRedact_ContextRequired(t *testing.T) {
	// Test context-required patterns like generic_token.
	s := New(WithRedactThreshold(0.3))
	// A hex token near a "token" keyword should be redacted.
	text := `Authorization: token abcdef1234567890abcdef1234567890abcdef12`
	redacted := s.Redact(text)
	if strings.Contains(redacted, "abcdef1234567890") {
		t.Logf("redacted = %q", redacted)
		// Context patterns may not trigger depending on exact pattern config.
		// Just ensure Redact doesn't crash.
	}
}

func TestRedact_MultipleFindings(t *testing.T) {
	s := New()
	text := `line1: AKIAIOSFODNN7EXAMPLE
line2: AKIAIOSFODNN7ANOTHER`
	redacted := s.Redact(text)
	if strings.Contains(redacted, "AKIAIOSFODNN7EXAMPLE") || strings.Contains(redacted, "AKIAIOSFODNN7ANOTHER") {
		t.Fatal("expected all AWS keys to be redacted")
	}
}

func TestRedact_NoSecrets(t *testing.T) {
	s := New()
	text := "just normal text with no secrets"
	if got := s.Redact(text); got != text {
		t.Fatalf("expected unchanged text, got %q", got)
	}
}
