package extraction

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClaudeCodeParser_HappyPath(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"system","timestamp":"2025-01-15T10:00:00Z","message":{"content":[]}}`,
		`{"type":"user","timestamp":"2025-01-15T10:00:01Z","message":{"content":[{"type":"text","text":"fix the bug"}]}}`,
		`{"type":"assistant","timestamp":"2025-01-15T10:00:05Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"I will fix it"}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"assistant","timestamp":"2025-01-15T10:01:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","name":"Bash"}],"usage":{"input_tokens":200,"output_tokens":30}}}`,
		`{"type":"user","timestamp":"2025-01-15T10:01:05Z","message":{"content":[{"type":"tool_result","is_error":false}]}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// system is not counted. user=2, assistant=2 → 4 messages.
	if rec.MessageCount != 4 {
		t.Fatalf("MessageCount = %d, want 4", rec.MessageCount)
	}
	if rec.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", rec.ToolCallCount)
	}
	if !rec.HasSuccessfulExec {
		t.Fatal("expected HasSuccessfulExec=true")
	}
	if rec.AgentModel != "claude-sonnet-4-20250514" {
		t.Fatalf("AgentModel = %q, want claude-sonnet-4-20250514", rec.AgentModel)
	}
	if rec.TokensUsed != 380 { // 100+50+200+30
		t.Fatalf("TokensUsed = %d, want 380", rec.TokensUsed)
	}

	wantStart := time.Date(2025, 1, 15, 10, 0, 1, 0, time.UTC)
	if !rec.StartedAt.Equal(wantStart) {
		t.Fatalf("StartedAt = %v, want %v", rec.StartedAt, wantStart)
	}
	wantEnd := time.Date(2025, 1, 15, 10, 1, 5, 0, time.UTC)
	if !rec.EndedAt.Equal(wantEnd) {
		t.Fatalf("EndedAt = %v, want %v", rec.EndedAt, wantEnd)
	}
}

func TestClaudeCodeParser_ToolResultError(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","name":"Bash"}],"usage":{"input_tokens":10,"output_tokens":5}}}`,
		`{"type":"user","timestamp":"2025-01-15T10:00:05Z","message":{"content":[{"type":"tool_result","is_error":true}]}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", rec.ErrorCount)
	}
	if rec.ErrorRate != 1.0 {
		t.Fatalf("ErrorRate = %f, want 1.0", rec.ErrorRate)
	}
}

func TestClaudeCodeParser_MultipleToolCalls(t *testing.T) {
	jsonl := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Bash"},{"type":"tool_use","name":"Write"}],"usage":{"input_tokens":50,"output_tokens":20}}}`

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount != 3 {
		t.Fatalf("ToolCallCount = %d, want 3", rec.ToolCallCount)
	}
}

func TestClaudeCodeParser_TokenCounting(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"a"}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
		`{"type":"assistant","timestamp":"2025-01-15T10:01:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"b"}],"usage":{"input_tokens":200,"output_tokens":75}}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.TokensUsed != 425 { // 100+50+200+75
		t.Fatalf("TokensUsed = %d, want 425", rec.TokensUsed)
	}
}

func TestClaudeCodeParser_NoAssistantMessages(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"system","timestamp":"2025-01-15T10:00:00Z","message":{"content":[]}}`,
		`{"type":"queue-operation","timestamp":"2025-01-15T10:00:01Z","message":{}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	_, err := p.Parse(strings.NewReader(jsonl))
	if !errors.Is(err, ErrMalformedFormat) {
		t.Fatalf("expected ErrMalformedFormat, got %v", err)
	}
}

func TestClaudeCodeParser_SingleMessage(t *testing.T) {
	jsonl := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":2}}}`

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes != 0 {
		t.Fatalf("DurationMinutes = %f, want 0", rec.DurationMinutes)
	}
}

func TestClaudeCodeParser_ProgressAndSnapshotIgnored(t *testing.T) {
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":2}}}`,
		`{"type":"progress","timestamp":"2025-01-15T10:00:10Z","message":{}}`,
		`{"type":"file-history-snapshot","timestamp":"2025-01-15T10:00:20Z","message":{}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.MessageCount != 1 {
		t.Fatalf("MessageCount = %d, want 1 (progress/snapshot should not count)", rec.MessageCount)
	}
}

func TestClaudeCodeParser_CanParse_Positive(t *testing.T) {
	header := []byte(`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{}}` + "\n")
	p := &ClaudeCodeParser{}
	if !p.CanParse(header) {
		t.Fatal("expected CanParse=true for valid Claude Code JSONL")
	}
}

func TestClaudeCodeParser_CanParse_PlainText(t *testing.T) {
	header := []byte("2024-01-15T10:00:00 Starting session\n")
	p := &ClaudeCodeParser{}
	if p.CanParse(header) {
		t.Fatal("expected CanParse=false for plain text")
	}
}

func TestClaudeCodeParser_CanParse_JSONWithoutType(t *testing.T) {
	header := []byte(`{"foo":"bar","baz":123}` + "\n")
	p := &ClaudeCodeParser{}
	if p.CanParse(header) {
		t.Fatal("expected CanParse=false for JSON without known type")
	}
}
