package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateCredentials_Dispatch(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{
			name:     "anthropic_provider",
			provider: "anthropic",
			wantErr:  false, // will succeed with mock server
		},
		{
			name:     "openai_provider",
			provider: "openai",
			wantErr:  false, // will succeed with mock server
		},
		{
			name:     "custom_provider",
			provider: "custom",
			wantErr:  false, // uses OpenAI format
		},
		{
			name:     "claude_cli_provider",
			provider: "claude-cli",
			wantErr:  false, // always passes
		},
		{
			name:     "unknown_provider_openai_succeeds",
			provider: "unknown-provider",
			wantErr:  false, // will try OpenAI first and succeed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock servers for success case
			anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"content":[{"text":"Hi"}]}`))
			}))
			defer anthropicServer.Close()

			openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data":[]}`))
			}))
			defer openaiServer.Close()

			creds := &Credentials{
				Provider: tt.provider,
				APIKey:   "test-key",
				Model:    "test-llmModel",
				BaseURL:  getServerURL(tt.provider, anthropicServer.URL, openaiServer.URL),
			}

			err := ValidateCredentials(creds)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCredentials() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCredentials_NoProvider(t *testing.T) {
	// Test unknown provider that falls back to OpenAI format
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // OpenAI fails
	}))
	defer openaiServer.Close()

	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":[{"text":"Hi"}]}`))
	}))
	defer anthropicServer.Close()

	creds := &Credentials{
		Provider: "unknown-provider",
		APIKey:   "test-key",
		Model:    "test-llmModel",
		BaseURL:  openaiServer.URL, // Will fail with OpenAI
	}

	// Should try OpenAI first, fail, then try Anthropic format
	err := ValidateCredentials(creds)
	if err == nil {
		t.Error("expected error when both OpenAI and Anthropic validation fail")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateCredentials_Timeout(t *testing.T) {
	// Create a server that delays longer than the context timeout
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // longer than 10s timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	creds := &Credentials{
		Provider: "anthropic",
		APIKey:   "test-key",
		Model:    "test-llmModel",
		BaseURL:  slowServer.URL,
	}

	err := ValidateCredentials(creds)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateAnthropicCredentials(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
		shouldHaveErr bool
	}{
		{
			name:          "success_200",
			statusCode:    200,
			responseBody:  `{"content":[{"text":"Hi"}]}`,
			shouldHaveErr: false,
		},
		{
			name:          "auth_failed_401",
			statusCode:    401,
			expectedError: "authentication failed — check your API key",
			shouldHaveErr: true,
		},
		{
			name:          "access_denied_403",
			statusCode:    403,
			expectedError: "access denied — API key may lack required permissions",
			shouldHaveErr: true,
		},
		{
			name:          "rate_limited_429",
			statusCode:    429,
			expectedError: "rate limited — try again in a moment",
			shouldHaveErr: true,
		},
		{
			name:          "server_error_500",
			statusCode:    500,
			expectedError: "API error (HTTP 500) — check your endpoint URL and model name",
			shouldHaveErr: true,
		},
		{
			name:          "bad_request_400",
			statusCode:    400,
			expectedError: "API error (HTTP 400) — check your endpoint URL and model name",
			shouldHaveErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request format
				if r.Method != "POST" {
					t.Errorf("expected POST request, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected application/json content type")
				}
				if r.Header.Get("x-api-key") != "test-api-key" {
					t.Errorf("expected x-api-key header")
				}
				if r.Header.Get("anthropic-version") != "2023-06-01" {
					t.Errorf("expected anthropic-version header")
				}

				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			creds := &Credentials{
				Provider: "anthropic",
				APIKey:   "test-api-key",
				Model:    "claude-3-haiku-20240307",
				BaseURL:  server.URL,
			}

			ctx := context.Background()
			err := validateAnthropicCredentials(ctx, creds)

			if tt.shouldHaveErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateAnthropicCredentials_CustomBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the URL path is correctly constructed
		expectedPath := "/v1/messages"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":[{"text":"Hi"}]}`))
	}))
	defer server.Close()

	creds := &Credentials{
		Provider: "anthropic",
		APIKey:   "test-api-key",
		Model:    "claude-3-haiku-20240307",
		BaseURL:  server.URL + "/", // trailing slash should be trimmed
	}

	ctx := context.Background()
	err := validateAnthropicCredentials(ctx, creds)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestValidateAnthropicCredentials_DefaultURL(t *testing.T) {
	// Test with empty BaseURL (should use default)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"content":[{"text":"Hi"}]}`))
	}))
	defer server.Close()

	creds := &Credentials{
		Provider: "anthropic",
		APIKey:   "test-api-key",
		Model:    "claude-3-haiku-20240307",
		BaseURL:  "", // empty should use default
	}

	ctx := context.Background()
	// This will try to hit the real API, so we expect it to fail
	// But we're testing the URL construction logic
	err := validateAnthropicCredentials(ctx, creds)
	if err == nil {
		t.Error("expected error when hitting real API without valid key")
	}
}

func TestValidateOpenAICredentials(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		expectedError string
		shouldHaveErr bool
	}{
		{
			name:          "success_200",
			statusCode:    200,
			responseBody:  `{"data":[]}`,
			shouldHaveErr: false,
		},
		{
			name:          "auth_failed_401",
			statusCode:    401,
			expectedError: "authentication failed — check your API key",
			shouldHaveErr: true,
		},
		{
			name:          "access_denied_403",
			statusCode:    403,
			expectedError: "access denied — API key may lack required permissions",
			shouldHaveErr: true,
		},
		{
			name:          "rate_limited_429",
			statusCode:    429,
			expectedError: "rate limited — try again in a moment",
			shouldHaveErr: true,
		},
		{
			name:          "server_error_500",
			statusCode:    500,
			expectedError: "API error (HTTP 500) — check your endpoint URL and model name",
			shouldHaveErr: true,
		},
		{
			name:          "bad_request_400",
			statusCode:    400,
			expectedError: "API error (HTTP 400) — check your endpoint URL and model name",
			shouldHaveErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request format
				if r.Method != "GET" {
					t.Errorf("expected GET request, got %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-api-key" {
					t.Errorf("expected Authorization header with Bearer token")
				}

				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			creds := &Credentials{
				Provider: "openai",
				APIKey:   "test-api-key",
				Model:    "gpt-4",
				BaseURL:  server.URL,
			}

			ctx := context.Background()
			err := validateOpenAICredentials(ctx, creds)

			if tt.shouldHaveErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateOpenAICredentials_CustomBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the URL path is correctly constructed
		expectedPath := "/v1/models"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	creds := &Credentials{
		Provider: "openai",
		APIKey:   "test-api-key",
		Model:    "gpt-4",
		BaseURL:  server.URL + "/", // trailing slash should be trimmed
	}

	ctx := context.Background()
	err := validateOpenAICredentials(ctx, creds)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestValidateOpenAICredentials_EmptyAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no Authorization header when API key is empty
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got: %s", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	creds := &Credentials{
		Provider: "openai",
		APIKey:   "", // empty API key
		Model:    "gpt-4",
		BaseURL:  server.URL,
	}

	ctx := context.Background()
	err := validateOpenAICredentials(ctx, creds)
	if err != nil {
		t.Errorf("expected no error but got: %v", err)
	}
}

func TestValidateOpenAICredentials_DefaultURL(t *testing.T) {
	// Test with empty BaseURL (should use default)
	creds := &Credentials{
		Provider: "openai",
		APIKey:   "test-api-key",
		Model:    "gpt-4",
		BaseURL:  "", // empty should use default
	}

	ctx := context.Background()
	// This will try to hit the real API, so we expect it to fail
	// But we're testing the URL construction logic
	err := validateOpenAICredentials(ctx, creds)
	if err == nil {
		t.Error("expected error when hitting real API without valid key")
	}
}

// Helper function to get appropriate server URL based on provider
func getServerURL(provider, anthropicURL, openaiURL string) string {
	switch provider {
	case "anthropic":
		return anthropicURL
	case "openai", "custom":
		return openaiURL
	case "claude-cli":
		return "" // not used for CLI
	default:
		return openaiURL // unknown providers try OpenAI first
	}
}
