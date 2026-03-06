package extraction

import (
	"errors"
	"strings"
	"testing"
)

func TestFallbackParser_Basic(t *testing.T) {
	log := `2024-01-15T10:00:00 Starting session
model=gpt-4o
2024-01-15T10:05:00 tool_call: exec something
error: something failed
2024-01-15T10:10:00 Done
`
	p := &FallbackParser{}
	rec, err := p.Parse(strings.NewReader(log))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", rec.ToolCallCount)
	}
	if rec.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", rec.ErrorCount)
	}
	if !rec.HasSuccessfulExec {
		t.Fatal("expected HasSuccessfulExec=true")
	}
	if rec.AgentModel != "gpt-4o" {
		t.Fatalf("AgentModel = %q, want gpt-4o", rec.AgentModel)
	}
	if rec.DurationMinutes < 9 || rec.DurationMinutes > 11 {
		t.Fatalf("DurationMinutes = %f, want ~10", rec.DurationMinutes)
	}
	if rec.TokensUsed == 0 {
		t.Fatal("expected non-zero TokensUsed")
	}
}

func TestFallbackParser_NoTimestamps(t *testing.T) {
	log := `Just some text without timestamps`
	p := &FallbackParser{}
	rec, err := p.Parse(strings.NewReader(log))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No timestamps → zero duration (not fake 10-minute default).
	if rec.DurationMinutes != 0 {
		t.Fatalf("DurationMinutes = %f, want 0", rec.DurationMinutes)
	}
}

func TestFallbackParser_Empty(t *testing.T) {
	p := &FallbackParser{}
	_, err := p.Parse(strings.NewReader(""))
	if !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}
}

func TestFallbackParser_CanParse_UTF8(t *testing.T) {
	header := []byte("2024-01-15T10:00:00 Starting session\n")
	p := &FallbackParser{}
	if !p.CanParse(header) {
		t.Fatal("expected CanParse=true for valid UTF-8 with timestamp")
	}
}

func TestFallbackParser_CanParse_Binary(t *testing.T) {
	header := []byte{0xff, 0xfe, 0x00, 0x01, 0x80, 0x90}
	p := &FallbackParser{}
	if p.CanParse(header) {
		t.Fatal("expected CanParse=false for invalid UTF-8")
	}
}

func TestFallbackParser_CanParse_NoTimestamp(t *testing.T) {
	header := []byte("Just some plain text without any timestamps\n")
	p := &FallbackParser{}
	if p.CanParse(header) {
		t.Fatal("expected CanParse=false for valid UTF-8 without timestamp")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input []byte
		want  int
	}{
		{nil, 0},
		{[]byte(""), 0},
		{[]byte("hello world!"), 3}, // 12 / 4
		{[]byte("abcd"), 1},         // 4 / 4
		{make([]byte, 1000), 250},   // 1000 / 4
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got != tt.want {
			t.Errorf("EstimateTokens(%d bytes) = %d, want %d", len(tt.input), got, tt.want)
		}
	}
}
