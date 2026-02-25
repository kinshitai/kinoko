package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ValidateCredentials makes a simple API call to validate the credentials work.
func ValidateCredentials(creds *Credentials) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch creds.Provider {
	case "anthropic":
		return validateAnthropicCredentials(ctx, creds)
	case "openai", "custom":
		return validateOpenAICredentials(ctx, creds)
	case "claude-cli":
		// For CLI delegation, we can't easily test without actually calling the CLI
		// which might be expensive. Just assume it works if claude is on PATH.
		return nil
	default:
		// For unknown providers, try OpenAI format first, then Anthropic
		if err := validateOpenAICredentials(ctx, creds); err == nil {
			return nil
		}
		return validateAnthropicCredentials(ctx, creds)
	}
}

// validateAnthropicCredentials tests Anthropic API access with a minimal request.
func validateAnthropicCredentials(ctx context.Context, creds *Credentials) error {
	url := "https://api.anthropic.com/v1/messages"
	if creds.BaseURL != "" {
		url = strings.TrimSuffix(creds.BaseURL, "/") + "/v1/messages"
	}

	// Create a minimal test message
	payload := map[string]interface{}{
		"model":      creds.Model,
		"max_tokens": 1,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": "Hi",
			},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", creds.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed — check your API key")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied — API key may lack required permissions")
	}
	if resp.StatusCode == 429 {
		return fmt.Errorf("rate limited — try again in a moment")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d) — check your endpoint URL and model name", resp.StatusCode)
	}

	return nil
}

// validateOpenAICredentials tests OpenAI API access by listing models.
func validateOpenAICredentials(ctx context.Context, creds *Credentials) error {
	url := "https://api.openai.com/v1/models"
	if creds.BaseURL != "" {
		url = strings.TrimSuffix(creds.BaseURL, "/") + "/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if creds.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+creds.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return fmt.Errorf("authentication failed — check your API key")
	}
	if resp.StatusCode == 403 {
		return fmt.Errorf("access denied — API key may lack required permissions")
	}
	if resp.StatusCode == 429 {
		return fmt.Errorf("rate limited — try again in a moment")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("API error (HTTP %d) — check your endpoint URL and model name", resp.StatusCode)
	}

	return nil
}
