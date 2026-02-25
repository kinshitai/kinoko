package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// runLLMWizard starts the interactive LLM setup wizard.
// It detects existing credentials and guides the user through provider selection.
func runLLMWizard(configPath string) error {
	// Check if stdin is a terminal (interactive mode)
	if !isTerminal() {
		return nil // Silently skip in non-interactive mode
	}

	fmt.Println()
	fmt.Println("🍄 Kinoko Setup - LLM Provider")
	fmt.Println()

	// Load current config to get existing LLM settings
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Try auto-detection first
	creds, autoErr := llm.ResolveCredentials(cfg.LLM)
	if autoErr == nil && creds != nil {
		fmt.Printf("✓ Detected: %s credentials\n", getProviderDisplayName(creds.Provider))
		fmt.Printf("  Provider: %s\n", creds.Provider)
		if creds.Model != "" {
			fmt.Printf("  Model: %s\n", creds.Model)
		}
		if creds.BaseURL != "" {
			fmt.Printf("  Endpoint: %s\n", creds.BaseURL)
		}
		fmt.Println()

		// Test the detected credentials
		if testErr := llm.ValidateCredentials(creds); testErr != nil {
			fmt.Printf("⚠️  Detected credentials failed validation: %v\n", testErr)
			fmt.Println("   Proceeding with manual setup...")
		} else {
			// Ask if user wants to use detected credentials
			fmt.Print("Use detected credentials? [Y/n]: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				response := strings.ToLower(strings.TrimSpace(scanner.Text()))
				if response == "" || response == "y" || response == "yes" {
					fmt.Println("✓ Using detected credentials")
					return nil // Already configured, no need to save
				}
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("failed to read input: %w", err)
			}
		}
		fmt.Println()
	}

	// No valid credentials detected, show provider menu
	fmt.Println("Which LLM provider would you like to use?")
	fmt.Println()
	fmt.Println("  1. Anthropic API key (recommended for API users)")
	fmt.Println("  2. Anthropic setup token (recommended for Claude Code subscribers)")
	fmt.Println("  3. Anthropic OAuth (reuse Claude Code credentials)")
	fmt.Println("  4. OpenAI API key")
	fmt.Println("  5. OpenAI ChatGPT subscription (reuse Codex credentials)")
	fmt.Println("  6. Custom provider (OpenAI-compatible endpoint)")
	fmt.Println("  7. Skip (configure later)")
	fmt.Println()
	fmt.Print("Choice [1-7, default=1]: ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		return fmt.Errorf("failed to read input")
	}

	choice := strings.TrimSpace(scanner.Text())
	if choice == "" {
		choice = "1" // Default to Anthropic API key
	}

	var newCreds *llm.Credentials
	var err2 error

	switch choice {
	case "1":
		newCreds, err2 = promptAnthropicAPIKey(scanner)
	case "2":
		newCreds, err2 = promptAnthropicSetupToken(scanner)
	case "3":
		newCreds, err2 = setupAnthropicOAuth()
	case "4":
		newCreds, err2 = promptOpenAIAPIKey(scanner)
	case "5":
		newCreds, err2 = setupCodexOAuth()
	case "6":
		newCreds, err2 = promptCustomProvider(scanner)
	case "7":
		fmt.Println("⏭️  Skipping LLM configuration.")
		fmt.Println("   Run 'kinoko init' again to configure, or set environment variables:")
		fmt.Println("   • ANTHROPIC_API_KEY for Anthropic")
		fmt.Println("   • OPENAI_API_KEY for OpenAI")
		return nil
	default:
		return fmt.Errorf("invalid choice: %s", choice)
	}

	if err2 != nil {
		return err2
	}

	// Save credentials to config file
	if err := saveLLMConfig(configPath, newCreds); err != nil {
		return fmt.Errorf("failed to save LLM config: %w", err)
	}

	fmt.Println("✓ LLM configuration saved")
	return nil
}

// isTerminal checks if stdin is connected to a terminal.
func isTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// getProviderDisplayName returns a user-friendly name for the provider.
func getProviderDisplayName(provider string) string {
	switch provider {
	case "anthropic":
		return "Anthropic"
	case "openai":
		return "OpenAI"
	case "claude-cli":
		return "Claude CLI"
	case "custom":
		return "Custom"
	default:
		// Simple capitalization for unknown providers
		if len(provider) == 0 {
			return provider
		}
		return strings.ToUpper(provider[:1]) + provider[1:]
	}
}

// promptAnthropicAPIKey prompts for an Anthropic API key and validates it.
func promptAnthropicAPIKey(scanner *bufio.Scanner) (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 Anthropic API Key Setup")
	fmt.Println("   Get your API key from: https://console.anthropic.com/settings/keys")
	fmt.Println()
	fmt.Print("Enter API key (sk-ant-api03-...): ")

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read API key: %w", err)
		}
		return nil, fmt.Errorf("failed to read API key")
	}

	apiKey := strings.TrimSpace(scanner.Text())
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	if !strings.HasPrefix(apiKey, "sk-ant-api03-") {
		fmt.Println("⚠️  Warning: API key doesn't match expected format (sk-ant-api03-...)")
	}

	creds := &llm.Credentials{
		Provider: "anthropic",
		APIKey:   apiKey,
		Model:    "claude-opus-4-0-20250514",
		BaseURL:  "",
	}

	// Test the credentials
	fmt.Print("🔍 Testing API key... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return nil, fmt.Errorf("API key validation failed: %w", err)
	}

	fmt.Println("✓ API key validated")
	return creds, nil
}

// promptAnthropicSetupToken prompts for an Anthropic setup token.
func promptAnthropicSetupToken(scanner *bufio.Scanner) (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 Anthropic Setup Token")
	fmt.Println("   1. Run 'claude setup-token' on any machine with Claude Code authenticated")
	fmt.Println("   2. Copy the generated token and paste it below")
	fmt.Println()
	fmt.Print("Enter setup token (sk-ant-oat01-...): ")

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read setup token: %w", err)
		}
		return nil, fmt.Errorf("failed to read setup token")
	}

	token := strings.TrimSpace(scanner.Text())
	if token == "" {
		return nil, fmt.Errorf("setup token cannot be empty")
	}

	if !strings.HasPrefix(token, "sk-ant-oat01-") {
		fmt.Println("⚠️  Warning: Token doesn't match expected format (sk-ant-oat01-...)")
	}

	creds := &llm.Credentials{
		Provider: "anthropic",
		APIKey:   token,
		Model:    "claude-opus-4-0-20250514",
		BaseURL:  "",
	}

	// Test the credentials
	fmt.Print("🔍 Testing setup token... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return nil, fmt.Errorf("setup token validation failed: %w", err)
	}

	fmt.Println("✓ Setup token validated")
	return creds, nil
}

// setupAnthropicOAuth attempts to reuse Claude Code OAuth credentials.
func setupAnthropicOAuth() (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 Claude Code OAuth Credentials")
	fmt.Print("🔍 Looking for existing credentials... ")

	creds, err := llm.ResolveCredentials(config.LLMConfig{}) // Empty config to skip config/env checks
	if err != nil || creds.Provider != "anthropic" || strings.HasPrefix(creds.APIKey, "sk-ant-api03-") {
		fmt.Println("❌ Not found")
		fmt.Println()
		fmt.Println("   Claude Code OAuth allows reusing credentials from the Claude desktop app.")
		fmt.Println()
		fmt.Println("   To set this up:")
		fmt.Println("   1. Install Claude Code from https://claude.ai/download")
		fmt.Println("   2. Sign in to your Anthropic account")
		fmt.Println("   3. Run this wizard again")
		fmt.Println()
		fmt.Println("   Or choose a different option above.")
		return nil, fmt.Errorf("no Claude Code OAuth credentials found")
	}

	fmt.Println("✓ Found")

	// Test the credentials
	fmt.Print("🔍 Testing OAuth credentials... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return nil, fmt.Errorf("OAuth credentials validation failed: %w", err)
	}

	fmt.Println("✓ OAuth credentials validated")
	return creds, nil
}

// promptOpenAIAPIKey prompts for an OpenAI API key and validates it.
func promptOpenAIAPIKey(scanner *bufio.Scanner) (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 OpenAI API Key Setup")
	fmt.Println("   Get your API key from: https://platform.openai.com/api-keys")
	fmt.Println()
	fmt.Print("Enter API key (sk-...): ")

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read API key: %w", err)
		}
		return nil, fmt.Errorf("failed to read API key")
	}

	apiKey := strings.TrimSpace(scanner.Text())
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	if !strings.HasPrefix(apiKey, "sk-") {
		fmt.Println("⚠️  Warning: API key doesn't match expected format (sk-...)")
	}

	creds := &llm.Credentials{
		Provider: "openai",
		APIKey:   apiKey,
		Model:    "gpt-5.2",
		BaseURL:  "",
	}

	// Test the credentials
	fmt.Print("🔍 Testing API key... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return nil, fmt.Errorf("API key validation failed: %w", err)
	}

	fmt.Println("✓ API key validated")
	return creds, nil
}

// setupCodexOAuth attempts to reuse Codex OAuth credentials.
func setupCodexOAuth() (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 Codex OAuth Credentials")
	fmt.Print("🔍 Looking for existing credentials... ")

	// Try to load Codex credentials directly
	creds, err := llm.ResolveCredentials(config.LLMConfig{}) // Empty config, will check OAuth files
	if err != nil || creds.Provider != "openai" || strings.HasPrefix(creds.APIKey, "sk-") {
		fmt.Println("❌ Not found")
		fmt.Println()
		fmt.Println("   Codex OAuth allows reusing credentials from the OpenAI Codex CLI.")
		fmt.Println()
		fmt.Println("   To set this up:")
		fmt.Println("   1. Install Codex from the OpenAI platform")
		fmt.Println("   2. Run 'codex login' to authenticate")
		fmt.Println("   3. Run this wizard again")
		fmt.Println()
		fmt.Println("   Or choose a different option above.")
		return nil, fmt.Errorf("no Codex OAuth credentials found")
	}

	fmt.Println("✓ Found")

	// Test the credentials
	fmt.Print("🔍 Testing OAuth credentials... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return nil, fmt.Errorf("OAuth credentials validation failed: %w", err)
	}

	fmt.Println("✓ OAuth credentials validated")
	return creds, nil
}

// promptCustomProvider prompts for custom provider details.
func promptCustomProvider(scanner *bufio.Scanner) (*llm.Credentials, error) {
	fmt.Println()
	fmt.Println("📝 Custom Provider Setup")
	fmt.Println("   Configure an OpenAI-compatible endpoint (e.g., local Ollama, corporate proxy)")
	fmt.Println()

	fmt.Print("Enter endpoint URL: ")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read endpoint URL: %w", err)
		}
		return nil, fmt.Errorf("failed to read endpoint URL")
	}

	baseURL := strings.TrimSpace(scanner.Text())
	if baseURL == "" {
		return nil, fmt.Errorf("endpoint URL cannot be empty")
	}

	fmt.Print("Enter API key (optional, press Enter to skip): ")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read API key: %w", err)
		}
		return nil, fmt.Errorf("failed to read API key")
	}

	apiKey := strings.TrimSpace(scanner.Text())

	fmt.Print("Enter model name (e.g., gpt-4, llama3:latest): ")
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read model name: %w", err)
		}
		return nil, fmt.Errorf("failed to read model name")
	}

	llmModel := strings.TrimSpace(scanner.Text())
	if llmModel == "" {
		llmModel = "gpt-4" // Default model
	}

	creds := &llm.Credentials{
		Provider: "custom",
		APIKey:   apiKey,
		Model:    llmModel,
		BaseURL:  baseURL,
	}

	// Test the credentials
	fmt.Print("🔍 Testing custom endpoint... ")
	if err := llm.ValidateCredentials(creds); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		fmt.Println("   Note: Some endpoints may not support the test API call.")
		fmt.Print("Continue anyway? [y/N]: ")

		if scanner.Scan() {
			response := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if response == "y" || response == "yes" {
				fmt.Println("⚠️  Proceeding without validation")
				return creds, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}
		return nil, fmt.Errorf("endpoint validation failed: %w", err)
	}

	fmt.Println("✓ Endpoint validated")
	return creds, nil
}

// saveLLMConfig saves the LLM credentials to the config file.
func saveLLMConfig(configPath string, creds *llm.Credentials) error {
	// Load existing config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load existing config: %w", err)
	}

	// Update LLM section based on credential type
	cfg.LLM.Provider = creds.Provider
	cfg.LLM.Model = creds.Model
	cfg.LLM.BaseURL = creds.BaseURL

	// Determine how to store the credential
	if strings.HasPrefix(creds.APIKey, "sk-ant-oat01-") {
		// This is a setup token
		cfg.LLM.SetupToken = creds.APIKey
		cfg.LLM.APIKey = "" // Clear API key field
	} else {
		// This is an API key or OAuth token
		cfg.LLM.APIKey = creds.APIKey
		cfg.LLM.SetupToken = "" // Clear setup token field
	}

	// Save updated config
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}
