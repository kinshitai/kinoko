package llm

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// Credentials holds resolved LLM access information.
type Credentials struct {
	Provider string // "anthropic" | "openai" | "custom" | "claude-cli"
	APIKey   string // API key or OAuth token (both go in auth header)
	Model    string // model identifier
	BaseURL  string // custom endpoint (empty = provider default)
}

// String returns a safe representation of credentials with masked API key.
func (c *Credentials) String() string {
	masked := c.APIKey
	if len(masked) > 8 {
		masked = masked[:8] + "..." + masked[len(masked)-4:]
	} else if masked != "" {
		masked = "***"
	}
	return fmt.Sprintf("Credentials{Provider:%s, Model:%s, Key:%s}", c.Provider, c.Model, masked)
}

// ResolveCredentials finds LLM credentials using the priority chain from RFC-004.
// Order: config → setup token → env vars → Claude Code OAuth → Codex OAuth → proxy → CLI
func ResolveCredentials(cfg config.LLMConfig) (*Credentials, error) {
	// 1. cfg.APIKey non-empty → use it, infer provider from key prefix if cfg.Provider empty
	if strings.TrimSpace(cfg.APIKey) != "" {
		provider := cfg.Provider
		if provider == "" {
			provider = inferProvider(cfg.APIKey)
		}
		return &Credentials{
			Provider: provider,
			APIKey:   strings.TrimSpace(cfg.APIKey),
			Model:    cfg.Model,
			BaseURL:  cfg.BaseURL,
		}, nil
	}

	// 2. cfg.SetupToken non-empty → anthropic (setup tokens are Anthropic OAuth tokens)
	if strings.TrimSpace(cfg.SetupToken) != "" {
		return &Credentials{
			Provider: "anthropic",
			APIKey:   strings.TrimSpace(cfg.SetupToken),
			Model:    getDefaultModel("anthropic"),
			BaseURL:  "",
		}, nil
	}

	// 3. KINOKO_API_KEY env var → infer provider
	if key := strings.TrimSpace(os.Getenv("KINOKO_API_KEY")); key != "" {
		provider := inferProvider(key)
		return &Credentials{
			Provider: provider,
			APIKey:   key,
			Model:    getDefaultModel(provider),
			BaseURL:  "",
		}, nil
	}

	// 4. ANTHROPIC_API_KEY env var → anthropic
	if key := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); key != "" {
		return &Credentials{
			Provider: "anthropic",
			APIKey:   key,
			Model:    getDefaultModel("anthropic"),
			BaseURL:  "",
		}, nil
	}

	// 5. OPENAI_API_KEY env var → openai
	if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
		return &Credentials{
			Provider: "openai",
			APIKey:   key,
			Model:    getDefaultModel("openai"),
			BaseURL:  "",
		}, nil
	}

	// 6. Claude Code OAuth: try ~/.claude/.credentials.json (Linux/Win)
	if creds, err := loadClaudeCodeOAuth(); err == nil {
		return creds, nil
	}

	// 7. Codex OAuth: try ~/.codex/auth.json
	if creds, err := loadCodexOAuth(); err == nil {
		return creds, nil
	}

	// 8. Proxy detection: HTTP GET localhost:3456/health (timeout 2s)
	if detectMaxProxy() {
		return &Credentials{
			Provider: "anthropic",
			APIKey:   "", // proxy doesn't need API key
			Model:    "claude-opus-4",
			BaseURL:  "http://localhost:3456/v1",
		}, nil
	}

	// 9. Claude CLI on PATH → return special "claude-cli" credentials
	if _, err := exec.LookPath("claude"); err == nil {
		return &Credentials{
			Provider: "claude-cli",
			APIKey:   "", // not used for CLI delegation
			Model:    "opus",
			BaseURL:  "",
		}, nil
	}

	// 10. Error: no credentials found
	return nil, fmt.Errorf("no LLM credentials found — run 'kinoko init' to configure")
}

// inferProvider determines the provider from API key prefix.
func inferProvider(key string) string {
	if strings.HasPrefix(key, "sk-ant-api03-") || strings.HasPrefix(key, "sk-ant-oat01-") {
		return "anthropic"
	}
	if strings.HasPrefix(key, "sk-") {
		return "openai"
	}
	return "anthropic" // default
}

// getDefaultModel returns the default model for a provider.
func getDefaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-opus-4-0-20250514"
	case "openai":
		return "gpt-5.2"
	default:
		return "claude-opus-4-0-20250514"
	}
}

// detectMaxProxy checks if claude-max-api-proxy is running at localhost:3456.
func detectMaxProxy() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:3456/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// loadClaudeCodeOAuth reads Claude Code OAuth credentials.
// Linux/Windows: ~/.claude/.credentials.json
// Stub for now — full implementation in commit 2.
func loadClaudeCodeOAuth() (*Credentials, error) {
	return nil, fmt.Errorf("Claude Code OAuth reader not implemented yet")
}

// loadCodexOAuth reads Codex OAuth credentials from ~/.codex/auth.json.
// Stub for now — full implementation in commit 2.
func loadCodexOAuth() (*Credentials, error) {
	return nil, fmt.Errorf("Codex OAuth reader not implemented yet")
}
