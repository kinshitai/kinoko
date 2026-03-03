package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// newCLICommand builds the *exec.Cmd for the Claude CLI.
// Overridden in tests to inject a fake binary.
var newCLICommand = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "claude", args...)
}

// claudeCLIResponse is the JSON output from `claude -p --output-format json`.
type claudeCLIResponse struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// ClaudeCLIClient invokes the Claude CLI for completions.
// It requires no API key — the CLI handles auth via `claude login`
// or the ANTHROPIC_API_KEY environment variable.
type ClaudeCLIClient struct {
	model string
}

// NewClaudeCLIClient creates a ClaudeCLIClient.
// model is optional; pass "" to use the CLI default.
func NewClaudeCLIClient(model string) *ClaudeCLIClient {
	return &ClaudeCLIClient{model: model}
}

// Complete implements LLMClient.
func (c *ClaudeCLIClient) Complete(ctx context.Context, prompt string) (string, error) {
	res, err := c.run(ctx, prompt)
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// CompleteWithTimeout implements LLMClientV2.
func (c *ClaudeCLIClient) CompleteWithTimeout(ctx context.Context, prompt string, timeout time.Duration) (*LLMCompleteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.run(ctx, prompt)
}

// run executes the Claude CLI and parses its JSON output.
func (c *ClaudeCLIClient) run(ctx context.Context, prompt string) (*LLMCompleteResult, error) {
	args := []string{"-p", "--output-format", "json", "--allowedTools", "", "--max-turns", "1"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}

	cmd := newCLICommand(ctx, args...)
	cmd.Stdin = strings.NewReader(prompt)

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude-cli: timed out")
		}
		return nil, fmt.Errorf("claude-cli exec: %w", err)
	}

	var resp claudeCLIResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("claude-cli: invalid JSON response: %w", err)
	}

	if resp.IsError {
		if strings.Contains(resp.Result, "Not logged in") {
			return nil, fmt.Errorf("claude CLI not authenticated. Run 'claude login' or set ANTHROPIC_API_KEY instead")
		}
		return nil, fmt.Errorf("claude-cli: %s", resp.Result)
	}

	return &LLMCompleteResult{Content: resp.Result}, nil
}
