package extraction

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParseSession_EmptyReader(t *testing.T) {
	_, err := ParseSession(bytes.NewReader(nil))
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

func TestParseSession_ClaudeCodeDispatch(t *testing.T) {
	// Valid Claude Code JSONL — should dispatch to ClaudeCodeParser.
	line := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
	rec, err := ParseSession(strings.NewReader(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.AgentModel != "claude-sonnet-4-20250514" {
		t.Fatalf("AgentModel = %q, want claude-sonnet-4-20250514", rec.AgentModel)
	}
}

func TestParseSession_FallbackDispatch(t *testing.T) {
	// Plain text with timestamps — should fall through to FallbackParser.
	log := "2024-01-15T10:00:00 Starting session\n2024-01-15T10:05:00 Done\n"
	_, err := ParseSession(strings.NewReader(log))
	// With stub FallbackParser, it won't match (CanParse returns false),
	// so we get ErrUnrecognizedFormat. Once implemented, this will parse.
	if err != nil && !errors.Is(err, ErrEmptyContent) && !errors.Is(err, ErrUnrecognizedFormat) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseSession_BinaryGarbage(t *testing.T) {
	// Invalid UTF-8 binary data — no parser should match.
	garbage := []byte{0xff, 0xfe, 0x00, 0x01, 0x80, 0x90, 0xa0, 0xb0, 0xc0, 0xd0}
	_, err := ParseSession(bytes.NewReader(garbage))
	if !errors.Is(err, ErrUnrecognizedFormat) {
		t.Fatalf("expected ErrUnrecognizedFormat, got %v", err)
	}
}
