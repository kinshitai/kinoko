// Package serverclient provides HTTP client implementations for kinoko run
// to communicate with kinoko serve.
package serverclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// defaultTimeout is the default HTTP client timeout.
	defaultTimeout = 30 * time.Second
	// maxResponseBytes is the maximum response body size (1 MB).
	maxResponseBytes = 1 << 20
)

// Client is the base HTTP client for communicating with kinoko serve.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// APIError represents an error response from the server.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("server error %d: %s", e.StatusCode, e.Message)
}

// New creates a new Client pointed at the given base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// doJSON performs an HTTP request with JSON encoding/decoding.
// If body is non-nil it is JSON-encoded as the request body.
// If response is non-nil the response body is JSON-decoded into it.
func (c *Client) doJSON(ctx context.Context, method, path string, body, response any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		msg := string(respBody)
		// Try to extract error field from JSON.
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			msg = errResp.Error
		}
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}

	if response != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(response); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
