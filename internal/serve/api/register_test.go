package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegister_Success(t *testing.T) {
	var calledPubkey, calledUsername string
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, pubkey, username string) error {
			calledPubkey = pubkey
			calledUsername = username
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test-key",
		Name:      "my-host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RegisterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "registered" {
		t.Errorf("expected status 'registered', got %q", resp.Status)
	}
	if resp.Username != "my-host" {
		t.Errorf("expected username 'my-host', got %q", resp.Username)
	}
	if calledPubkey != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI test-key" {
		t.Errorf("unexpected pubkey passed to registerFn: %q", calledPubkey)
	}
	if calledUsername != "my-host" {
		t.Errorf("unexpected username passed to registerFn: %q", calledUsername)
	}
}

func TestRegister_InvalidBody(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	tests := []struct {
		name string
		body string
	}{
		{"not json", "not json at all"},
		{"empty pubkey", `{"public_key":"","name":"host"}`},
		{"bad pubkey prefix", `{"public_key":"not-ssh-key","name":"host"}`},
		{"pubkey type only", `{"public_key":"ssh-ed25519","name":"host"}`},
		{"pubkey base64 too short", `{"public_key":"ssh-ed25519 AAAA","name":"host"}`},
		{"empty name", `{"public_key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST","name":""}`},
		{"invalid name chars", `{"public_key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST","name":"bad name!"}`},
		{"name starts with hyphen", `{"public_key":"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST","name":"-bad"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader([]byte(tt.body)))
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRegister_TokenRequired(t *testing.T) {
	srv := New(Config{
		Port:              0,
		RegistrationToken: "secret-token",
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})

	// No token → 401
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}

	// Wrong token → 401
	body, _ = json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})
	req = httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}
}

func TestRegister_TokenValid(t *testing.T) {
	srv := New(Config{
		Port:              0,
		RegistrationToken: "secret-token",
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_NotConfigured(t *testing.T) {
	srv := New(Config{Port: 0}) // No RegisterFn

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 without registerFn, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegister_FnError(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return fmt.Errorf("soft serve exploded")
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on registerFn error, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegister_UnicodeName verifies that names with unicode characters
// are rejected by the server's reValidName regex.
func TestRegister_UnicodeName(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	names := []string{
		"хост",         // Cyrillic
		"名前",           // CJK
		"host-émoji",   // accented char
		"host🍄",        // emoji
		"host\x00name", // null byte
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(RegisterRequest{
				PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
				Name:      name,
			})
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for unicode name %q, got %d: %s", name, w.Code, w.Body.String())
			}
		})
	}
}

// TestRegister_NameTooLong verifies the 64-char limit on names.
func TestRegister_NameTooLong(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	tests := []struct {
		name   string
		input  string
		expect int
	}{
		{"exactly 64 chars", "a" + repeat('b', 63), http.StatusOK},
		{"65 chars", "a" + repeat('b', 64), http.StatusBadRequest},
		{"128 chars", "a" + repeat('b', 127), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(RegisterRequest{
				PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
				Name:      tt.input,
			})
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			if w.Code != tt.expect {
				t.Errorf("expected %d for %d-char name, got %d: %s", tt.expect, len(tt.input), w.Code, w.Body.String())
			}
		})
	}
}

func repeat(b byte, n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = b
	}
	return string(buf)
}

// TestRegister_OversizedBody verifies that the MaxBytesReader limit
// (1 MB) causes a 400 for very large request bodies.
func TestRegister_OversizedBody(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	// 2 MB of data — exceeds the 1 MB limit.
	hugeBody := bytes.Repeat([]byte("A"), 2<<20)
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(hugeBody))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegister_PubkeyOnlyType verifies that "ssh-ed25519" alone
// (no base64 data) is now rejected by the stricter validation.
func TestRegister_PubkeyOnlyType(t *testing.T) {
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for key-type-only pubkey, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegister_AuthEdgeCases tests malformed Authorization headers.
func TestRegister_AuthEdgeCases(t *testing.T) {
	srv := New(Config{
		Port:              0,
		RegistrationToken: "secret",
		RegisterFn: func(_ context.Context, _, _ string) error {
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "host",
	})

	tests := []struct {
		name string
		auth string
	}{
		{"empty header", ""},
		{"just Bearer", "Bearer"},
		{"Basic auth", "Basic dXNlcjpwYXNz"},
		{"Bearer with trailing space", "Bearer "},
		{"token without Bearer prefix", "secret"},
		{"partial token", "Bearer secre"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			w := httptest.NewRecorder()
			srv.httpServer.Handler.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for auth %q, got %d", tt.auth, w.Code)
			}
		})
	}
}

// TestRegister_PubkeyWhitespaceTrimming verifies that leading/trailing
// whitespace in the public key is trimmed before validation.
func TestRegister_PubkeyWhitespaceTrimming(t *testing.T) {
	var calledPubkey string
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, pubkey, _ string) error {
			calledPubkey = pubkey
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "  ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest  \n",
		Name:      "host",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if calledPubkey != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAItest" {
		// After TrimSpace, the padded pubkey should be clean.
		t.Errorf("expected trimmed pubkey, got %q", calledPubkey)
	}
}

// TestRegister_NameWhitespaceTrimming verifies that name whitespace is trimmed.
func TestRegister_NameWhitespaceTrimming(t *testing.T) {
	var calledUsername string
	srv := New(Config{
		Port: 0,
		RegisterFn: func(_ context.Context, _, username string) error {
			calledUsername = username
			return nil
		},
	})

	body, _ := json.Marshal(RegisterRequest{
		PublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST",
		Name:      "  my-host  ",
	})
	req := httptest.NewRequest("POST", "/api/v1/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if calledUsername != "my-host" {
		t.Errorf("expected trimmed username 'my-host', got %q", calledUsername)
	}
}
