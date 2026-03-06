package extraction

import (
	"bufio"
	"encoding/json"
	"io"
	"time"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// ClaudeCodeParser parses Claude Code native JSONL session logs.
type ClaudeCodeParser struct{}

// claudeCodeContent represents a single content block in a Claude Code message.
type claudeCodeContent struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

// claudeCodeUsage holds token counts for a Claude Code message.
type claudeCodeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// claudeCodeLine is a lightweight envelope for a single JSONL line.
type claudeCodeLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		Model   string              `json:"model"`
		Content []claudeCodeContent `json:"content"`
		Usage   claudeCodeUsage     `json:"usage"`
	} `json:"message"`
}

// knownClaudeCodeTypes are the top-level type values emitted by Claude Code.
var knownClaudeCodeTypes = map[string]bool{
	"assistant":             true,
	"user":                  true,
	"system":                true,
	"queue-operation":       true,
	"file-history-snapshot": true,
	"progress":              true,
}

// CanParse returns true if the header looks like Claude Code JSONL.
//
// NOTE: If the first JSONL line exceeds headerSize (4KB), the truncated
// JSON will fail to unmarshal and CanParse returns false. This is unlikely
// but possible with very large system prompts. The session would fall
// through to FallbackParser or ErrUnrecognizedFormat.
func (p *ClaudeCodeParser) CanParse(header []byte) bool {
	// Find end of first line.
	end := len(header)
	for i, b := range header {
		if b == '\n' {
			end = i
			break
		}
	}
	firstLine := header[:end]
	if len(firstLine) == 0 {
		return false
	}

	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(firstLine, &probe); err != nil {
		return false
	}
	return knownClaudeCodeTypes[probe.Type]
}

// Parse extracts session metadata from Claude Code JSONL.
func (p *ClaudeCodeParser) Parse(r io.Reader) (*model.SessionRecord, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	var (
		timestamps    []time.Time
		messageCount  int
		toolCallCount int
		errorCount    int
		hasExec       bool
		agentModel    string
		totalTokens   int
	)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry claudeCodeLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		// Only "assistant" and "user" are counted as messages.
		// New types added by Claude Code are safely ignored — the parser
		// extracts what it knows and skips the rest.
		switch entry.Type {
		case "assistant", "user":
			messageCount++
			if !entry.Timestamp.IsZero() {
				timestamps = append(timestamps, entry.Timestamp)
			}
		}

		if entry.Type == "assistant" {
			if agentModel == "" && entry.Message.Model != "" {
				agentModel = entry.Message.Model
			}
			totalTokens += entry.Message.Usage.InputTokens + entry.Message.Usage.OutputTokens

			for _, c := range entry.Message.Content {
				if c.Type == "tool_use" {
					toolCallCount++
					if c.Name == "Bash" || c.Name == "exec" || c.Name == "shell" {
						hasExec = true
					}
				}
			}
		}

		// Count errors in all message types (tool_result can appear in user messages).
		for _, c := range entry.Message.Content {
			if c.Type == "tool_result" && c.IsError {
				errorCount++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if messageCount == 0 {
		return nil, ErrMalformedFormat
	}

	rec := &model.SessionRecord{
		MessageCount:      messageCount,
		ToolCallCount:     toolCallCount,
		ErrorCount:        errorCount,
		HasSuccessfulExec: hasExec,
		AgentModel:        agentModel,
		TokensUsed:        totalTokens,
	}

	if len(timestamps) >= 2 {
		rec.StartedAt = timestamps[0]
		rec.EndedAt = timestamps[len(timestamps)-1]
		rec.DurationMinutes = rec.EndedAt.Sub(rec.StartedAt).Minutes()
		if rec.DurationMinutes < 0 {
			rec.DurationMinutes = 0
		}
	} else if len(timestamps) == 1 {
		rec.StartedAt = timestamps[0]
		rec.EndedAt = timestamps[0]
		rec.DurationMinutes = 0
	}

	if rec.ToolCallCount > 0 {
		rec.ErrorRate = float64(rec.ErrorCount) / float64(rec.ToolCallCount)
	}

	return rec, nil
}
