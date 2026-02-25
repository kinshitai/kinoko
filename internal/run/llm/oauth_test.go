package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestClaudeCodeOAuth tests Claude Code OAuth credential loading
func TestClaudeCodeOAuth(t *testing.T) {
	// Create temporary directory for test credentials
	tmpDir := t.TempDir()

	// Test 1: Valid credentials with expiry
	t.Run("valid_with_expiry", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude1")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		// Create valid credentials (expires in future)
		creds := map[string]interface{}{
			"accessToken":  "sk-ant-oat01-test-access-token",
			"refreshToken": "sk-ant-refresh-test-token",
			"expiresAt":    time.Now().Unix() + 3600, // expires in 1 hour
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		result, err := loadClaudeCodeOAuthWithHome(claudeDir)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if result.Provider != "anthropic" {
			t.Errorf("expected provider 'anthropic', got: %s", result.Provider)
		}
		if result.APIKey != "sk-ant-oat01-test-access-token" {
			t.Errorf("expected access token, got: %s", result.APIKey)
		}
		if result.Model != "claude-opus-4-0-20250514" {
			t.Errorf("expected default model, got: %s", result.Model)
		}
	})

	// Test 2: Expired token
	t.Run("expired_token", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude2")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		// Create expired credentials
		creds := map[string]interface{}{
			"accessToken":  "sk-ant-oat01-expired-token",
			"refreshToken": "sk-ant-refresh-expired-token", 
			"expiresAt":    time.Now().Unix() - 3600, // expired 1 hour ago
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		_, err = loadClaudeCodeOAuthWithHome(claudeDir)
		if err == nil {
			t.Error("expected error for expired token")
		}
		if err.Error() != "OAuth token expired. Run 'claude' to refresh your Claude Code session" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	// Test 3: No expiry (expiresAt = 0) - should be treated as valid
	t.Run("no_expiry", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude3")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"accessToken":  "sk-ant-oat01-no-expiry-token",
			"refreshToken": "sk-ant-refresh-no-expiry-token",
			"expiresAt":    0,
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		result, err := loadClaudeCodeOAuthWithHome(claudeDir)
		if err != nil {
			t.Fatalf("expected no error for no expiry, got: %v", err)
		}

		if result.APIKey != "sk-ant-oat01-no-expiry-token" {
			t.Errorf("expected access token, got: %s", result.APIKey)
		}
	})

	// Test 4: Missing expiry field - should be treated as valid
	t.Run("missing_expiry", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude4")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"accessToken":  "sk-ant-oat01-missing-expiry-token",
			"refreshToken": "sk-ant-refresh-missing-expiry-token",
			// No expiresAt field
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		result, err := loadClaudeCodeOAuthWithHome(claudeDir)
		if err != nil {
			t.Fatalf("expected no error for missing expiry, got: %v", err)
		}

		if result.APIKey != "sk-ant-oat01-missing-expiry-token" {
			t.Errorf("expected access token, got: %s", result.APIKey)
		}
	})

	// Test 5: Missing file
	t.Run("missing_file", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude_missing")

		_, err := loadClaudeCodeOAuthWithHome(claudeDir)
		if err == nil {
			t.Error("expected error for missing credentials file")
		}
		if !os.IsNotExist(err) && !filepath.IsAbs(err.Error()) {
			// Should contain the file path in error
			expectedPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
			if !filepath.IsAbs(expectedPath) || !filepath.IsAbs(err.Error()) {
				t.Logf("Error: %v", err)
			}
		}
	})

	// Test 6: Malformed JSON
	t.Run("malformed_json", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude_malformed")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		// Write invalid JSON
		err = os.WriteFile(credPath, []byte(`{"accessToken": invalid json`), 0644)
		if err != nil {
			t.Fatalf("failed to write malformed credentials: %v", err)
		}

		_, err = loadClaudeCodeOAuthWithHome(claudeDir)
		if err == nil {
			t.Error("expected error for malformed JSON")
		}
	})

	// Test 7: Missing access token
	t.Run("missing_access_token", func(t *testing.T) {
		claudeDir := filepath.Join(tmpDir, "claude_no_token")
		err := os.MkdirAll(claudeDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"refreshToken": "sk-ant-refresh-only-token",
			"expiresAt":    time.Now().Unix() + 3600,
		}

		credPath := filepath.Join(claudeDir, ".claude", ".credentials.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		_, err = loadClaudeCodeOAuthWithHome(claudeDir)
		if err == nil {
			t.Error("expected error for missing access token")
		}
	})
}

// TestCodexOAuth tests Codex OAuth credential loading
func TestCodexOAuth(t *testing.T) {
	// Create temporary directory for test credentials
	tmpDir := t.TempDir()

	// Test 1: Valid credentials with expiry
	t.Run("valid_with_expiry", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex1")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		// Create valid credentials (expires in future)
		futureTime := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		creds := map[string]interface{}{
			"token":      "gth_test_token_valid",
			"expires_at": futureTime,
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		result, err := loadCodexOAuthWithHome(codexDir)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if result.Provider != "openai" {
			t.Errorf("expected provider 'openai', got: %s", result.Provider)
		}
		if result.APIKey != "gth_test_token_valid" {
			t.Errorf("expected token, got: %s", result.APIKey)
		}
		if result.Model != "gpt-5.2" {
			t.Errorf("expected default OpenAI model, got: %s", result.Model)
		}
	})

	// Test 2: Expired token
	t.Run("expired_token", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex2")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		// Create expired credentials
		pastTime := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
		creds := map[string]interface{}{
			"token":      "gth_test_token_expired",
			"expires_at": pastTime,
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		_, err = loadCodexOAuthWithHome(codexDir)
		if err == nil {
			t.Error("expected error for expired token")
		}
		if err.Error() != "OAuth token expired. Run 'codex login' to refresh your Codex session" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	// Test 3: No expiry field - should be treated as valid
	t.Run("no_expiry", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex3")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"token": "gth_test_token_no_expiry",
			// No expires_at field
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		result, err := loadCodexOAuthWithHome(codexDir)
		if err != nil {
			t.Fatalf("expected no error for no expiry, got: %v", err)
		}

		if result.APIKey != "gth_test_token_no_expiry" {
			t.Errorf("expected token, got: %s", result.APIKey)
		}
	})

	// Test 4: Missing file
	t.Run("missing_file", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex_missing")

		_, err := loadCodexOAuthWithHome(codexDir)
		if err == nil {
			t.Error("expected error for missing credentials file")
		}
	})

	// Test 5: Malformed JSON
	t.Run("malformed_json", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex_malformed")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		// Write invalid JSON
		err = os.WriteFile(credPath, []byte(`{"token": invalid json`), 0644)
		if err != nil {
			t.Fatalf("failed to write malformed credentials: %v", err)
		}

		_, err = loadCodexOAuthWithHome(codexDir)
		if err == nil {
			t.Error("expected error for malformed JSON")
		}
	})

	// Test 6: Missing token
	t.Run("missing_token", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex_no_token")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			// No token field
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		_, err = loadCodexOAuthWithHome(codexDir)
		if err == nil {
			t.Error("expected error for missing token")
		}
	})

	// Test 7: Malformed expiry time
	t.Run("malformed_expiry", func(t *testing.T) {
		codexDir := filepath.Join(tmpDir, "codex_bad_expiry")
		err := os.MkdirAll(codexDir, 0755)
		if err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}

		creds := map[string]interface{}{
			"token":      "gth_test_token_bad_expiry",
			"expires_at": "not-a-valid-time-format",
		}

		credPath := filepath.Join(codexDir, ".codex", "auth.json")
		err = os.MkdirAll(filepath.Dir(credPath), 0755)
		if err != nil {
			t.Fatalf("failed to create credentials directory: %v", err)
		}

		credData, _ := json.Marshal(creds)
		err = os.WriteFile(credPath, credData, 0644)
		if err != nil {
			t.Fatalf("failed to write test credentials: %v", err)
		}

		_, err = loadCodexOAuthWithHome(codexDir)
		if err == nil {
			t.Error("expected error for malformed expiry time")
		}
	})
}

// TestOAuthIntegration tests OAuth functions in the resolution chain
func TestOAuthIntegration(t *testing.T) {
	// Test that the stubs no longer return "not implemented" errors
	_, err := loadClaudeCodeOAuth()
	if err != nil && err.Error() == "Claude Code OAuth reader not implemented yet" {
		t.Error("loadClaudeCodeOAuth should not return stub error after implementation")
	}

	_, err = loadCodexOAuth() 
	if err != nil && err.Error() == "Codex OAuth reader not implemented yet" {
		t.Error("loadCodexOAuth should not return stub error after implementation")
	}

	// Both functions should return errors for missing files, not implementation errors
	_, claudeErr := loadClaudeCodeOAuth()
	_, codexErr := loadCodexOAuth()
	
	if claudeErr != nil && claudeErr.Error() == "Claude Code OAuth reader not implemented yet" {
		t.Error("Claude Code OAuth reader should be implemented")
	}
	
	if codexErr != nil && codexErr.Error() == "Codex OAuth reader not implemented yet" {
		t.Error("Codex OAuth reader should be implemented")  
	}
}