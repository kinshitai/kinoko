package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
)

// AnthropicClient is a client for the Anthropic Messages API.
type AnthropicClient struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewAnthropicClient creates an Anthropic LLM client.
// baseURL is optional; pass "" to use the default Anthropic endpoint.
func NewAnthropicClient(apiKey, model, baseURL string, opts ...Option) *AnthropicClient {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	c := &AnthropicClient{
		apiKey:     apiKey,
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
	// Reuse Option type — only WithHTTPClient applies; others are silently ignored
	// via an adapter since Option targets OpenAIClient. We accept *http.Client directly.
	for _, opt := range opts {
		// Apply option via a temporary OpenAI wrapper to extract the http client.
		tmp := &OpenAIClient{}
		opt(tmp)
		if tmp.httpClient != nil {
			c.httpClient = tmp.httpClient
		}
	}
	return c
}

// anthropicRequest is the request body for Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the response from Anthropic Messages API.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete implements LLMClient.
func (c *AnthropicClient) Complete(ctx context.Context, prompt string) (string, error) {
	result, err := c.doRequest(ctx, prompt, 4096)
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

// CompleteWithTimeout implements LLMClientV2.
func (c *AnthropicClient) CompleteWithTimeout(ctx context.Context, prompt string, timeout time.Duration) (*LLMCompleteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.doRequest(ctx, prompt, 4096)
}

func (c *AnthropicClient) doRequest(ctx context.Context, prompt string, maxTokens int) (*LLMCompleteResult, error) {
	reqBody := anthropicRequest{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages: []anthropicMessage{
			{Role: "user", Content: prompt},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		return nil, &LLMError{StatusCode: resp.StatusCode, Message: string(body[:n])}
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", result.Error.Message)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	// Find the first text block.
	for _, block := range result.Content {
		if block.Type == "text" {
			return &LLMCompleteResult{
				Content:   block.Text,
				TokensIn:  result.Usage.InputTokens,
				TokensOut: result.Usage.OutputTokens,
			}, nil
		}
	}

	return nil, fmt.Errorf("no text content in response")
}
