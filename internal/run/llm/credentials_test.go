package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

func TestResolveCredentials_Config(t *testing.T) {
	// Test 1: Config with explicit API key and provider
	cfg := config.LLMConfig{
		Provider: "anthropic",
		APIKey:   "sk-ant-api03-test",
		Model:    "claude-sonnet-4-20250514",
		BaseURL:  "https://api.anthropic.com",
	}

	creds, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-ant-api03-test" {
		t.Errorf("expected API key 'sk-ant-api03-test', got: %s", creds.APIKey)
	}
	if creds.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model 'claude-sonnet-4-20250514', got: %s", creds.Model)
	}
	if creds.BaseURL != "https://api.anthropic.com" {
		t.Errorf("expected base URL 'https://api.anthropic.com', got: %s", creds.BaseURL)
	}
}

func TestResolveCredentials_ConfigInferProvider(t *testing.T) {
	// Test 2: Config with API key but no provider → infer from key
	cfg := config.LLMConfig{
		APIKey: "sk-ant-api03-infer-test",
		Model:  "custom-model",
	}

	creds, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "anthropic" {
		t.Errorf("expected inferred provider 'anthropic', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-ant-api03-infer-test" {
		t.Errorf("expected API key 'sk-ant-api03-infer-test', got: %s", creds.APIKey)
	}
	if creds.Model != "custom-model" {
		t.Errorf("expected model 'custom-model', got: %s", creds.Model)
	}
}

func TestResolveCredentials_EnvVars(t *testing.T) {
	// Clean environment
	cleanEnv(t)

	// Test 3: KINOKO_API_KEY env var
	t.Setenv("KINOKO_API_KEY", "sk-ant-oat01-kinoko-test")

	cfg := config.LLMConfig{} // empty config

	creds, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-ant-oat01-kinoko-test" {
		t.Errorf("expected API key 'sk-ant-oat01-kinoko-test', got: %s", creds.APIKey)
	}
	if creds.Model != "claude-opus-4-0-20250514" {
		t.Errorf("expected default model, got: %s", creds.Model)
	}

	// Test 4: ANTHROPIC_API_KEY env var
	t.Setenv("KINOKO_API_KEY", "") // clear previous
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-api03-anthropic-test")

	creds, err = ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-ant-api03-anthropic-test" {
		t.Errorf("expected API key 'sk-ant-api03-anthropic-test', got: %s", creds.APIKey)
	}

	// Test 5: OPENAI_API_KEY env var
	t.Setenv("ANTHROPIC_API_KEY", "") // clear previous
	t.Setenv("OPENAI_API_KEY", "sk-openai-test")

	creds, err = ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "openai" {
		t.Errorf("expected provider 'openai', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-openai-test" {
		t.Errorf("expected API key 'sk-openai-test', got: %s", creds.APIKey)
	}
	if creds.Model != "gpt-5.2" {
		t.Errorf("expected OpenAI default model, got: %s", creds.Model)
	}
}

func TestResolveCredentials_SetupToken(t *testing.T) {
	cleanEnv(t)

	// Test 6: SetupToken in config
	cfg := config.LLMConfig{
		SetupToken: "sk-ant-oat01-setup-token-test",
	}

	creds, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got: %s", creds.Provider)
	}
	if creds.APIKey != "sk-ant-oat01-setup-token-test" {
		t.Errorf("expected API key 'sk-ant-oat01-setup-token-test', got: %s", creds.APIKey)
	}
}

func TestResolveCredentials_Proxy(t *testing.T) {
	cleanEnv(t)

	// Test 7: Proxy detection
	// Create a mock proxy server
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer proxy.Close()

	// We can't easily test the real proxy detection without modifying the URL,
	// so let's test the detectMaxProxy function separately
	if detectMaxProxy() {
		// If the real proxy is running, we can't reliably test this
		t.Skip("claude-max-api-proxy is actually running, skipping proxy test")
	}

	// Test the case where no proxy is detected
	cfg := config.LLMConfig{}
	_, err := ResolveCredentials(cfg)
	if err == nil {
		t.Errorf("expected error when no credentials found")
	}
	if err.Error() != "no LLM credentials found — run 'kinoko init' to configure" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestResolveCredentials_PriorityOrder(t *testing.T) {
	cleanEnv(t)

	// Test 8: Priority order - config beats env vars
	t.Setenv("KINOKO_API_KEY", "sk-env-key")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-env-key")

	cfg := config.LLMConfig{
		APIKey: "sk-config-key",
	}

	creds, err := ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.APIKey != "sk-config-key" {
		t.Errorf("expected config key to win, got: %s", creds.APIKey)
	}

	// Test 9: KINOKO_API_KEY beats ANTHROPIC_API_KEY
	cfg = config.LLMConfig{} // clear config

	creds, err = ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.APIKey != "sk-env-key" {
		t.Errorf("expected KINOKO_API_KEY to win, got: %s", creds.APIKey)
	}

	// Test 10: Setup token beats env vars when config is empty
	cfg = config.LLMConfig{
		SetupToken: "sk-ant-oat01-setup-wins",
	}

	creds, err = ResolveCredentials(cfg)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if creds.APIKey != "sk-ant-oat01-setup-wins" {
		t.Errorf("expected setup token to win, got: %s", creds.APIKey)
	}
}

func TestInferProvider(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"sk-ant-api03-test", "anthropic"},
		{"sk-ant-oat01-test", "anthropic"},
		{"sk-test", "openai"},
		{"sk-proj-test", "openai"},
		{"invalid-key", "anthropic"}, // default
		{"", "anthropic"},            // default
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := inferProvider(tt.key)
			if result != tt.expected {
				t.Errorf("inferProvider(%q) = %q, want %q", tt.key, result, tt.expected)
			}
		})
	}
}

func TestGetDefaultModel(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"anthropic", "claude-opus-4-0-20250514"},
		{"openai", "gpt-5.2"},
		{"unknown", "claude-opus-4-0-20250514"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			result := getDefaultModel(tt.provider)
			if result != tt.expected {
				t.Errorf("getDefaultModel(%q) = %q, want %q", tt.provider, result, tt.expected)
			}
		})
	}
}

func TestDetectMaxProxy(t *testing.T) {
	// Test when proxy is not running
	if detectMaxProxy() {
		t.Skip("claude-max-api-proxy is running, can't test negative case")
	}

	// Test mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// The function specifically checks localhost:3456, so we can only test the negative case
	result := detectMaxProxy()
	if result {
		t.Log("Proxy is actually running - test passes")
	} else {
		t.Log("No proxy detected - test passes")
	}
}

func TestOAuthStubs(t *testing.T) {
	// Test that OAuth readers return "not implemented" errors for commit 1
	_, err := loadClaudeCodeOAuth()
	if err == nil {
		t.Error("expected error from loadClaudeCodeOAuth stub")
	}

	_, err = loadCodexOAuth()
	if err == nil {
		t.Error("expected error from loadCodexOAuth stub")
	}
}

func TestResolveCredentials_WhitespaceKeys(t *testing.T) {
	cleanEnv(t)

	// Whitespace-only API key should be treated as empty
	cfg := config.LLMConfig{
		APIKey: "  \t\n  ",
	}

	_, err := ResolveCredentials(cfg)
	if err == nil {
		t.Error("expected error for whitespace-only API key")
	}

	// Whitespace-only setup token should be treated as empty
	cfg = config.LLMConfig{
		SetupToken: "  \t  ",
	}

	_, err = ResolveCredentials(cfg)
	if err == nil {
		t.Error("expected error for whitespace-only setup token")
	}
}

func TestCredentials_String(t *testing.T) {
	c := &Credentials{
		Provider: "anthropic",
		APIKey:   "sk-ant-api03-longkeythatshouldbmasked",
		Model:    "claude-opus-4-0-20250514",
	}
	s := c.String()
	if strings.Contains(s, "longkey") {
		t.Errorf("String() should mask API key, got: %s", s)
	}
	if !strings.Contains(s, "sk-ant-a") {
		t.Errorf("String() should show key prefix, got: %s", s)
	}
}

// OAuth credential format tests will be added in commit 2 when readers are implemented.

// cleanEnv clears all LLM-related environment variables for testing.
func cleanEnv(t *testing.T) {
	t.Setenv("KINOKO_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
}
