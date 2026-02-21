package extraction

import (
	"testing"
)

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

func TestParseSessionFromLog_Basic(t *testing.T) {
	log := `2024-01-15T10:00:00 Starting session
model=gpt-4o
2024-01-15T10:05:00 tool_call: exec something
error: something failed
2024-01-15T10:10:00 Done
`
	session := ParseSessionFromLog([]byte(log), "test-lib")
	if session.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if session.LibraryID != "test-lib" {
		t.Fatalf("LibraryID = %q", session.LibraryID)
	}
	if session.ToolCallCount != 1 {
		t.Fatalf("ToolCallCount = %d, want 1", session.ToolCallCount)
	}
	if session.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", session.ErrorCount)
	}
	if !session.HasSuccessfulExec {
		t.Fatal("expected HasSuccessfulExec=true")
	}
	if session.AgentModel != "gpt-4o" {
		t.Fatalf("AgentModel = %q, want gpt-4o", session.AgentModel)
	}
	if session.DurationMinutes < 9 || session.DurationMinutes > 11 {
		t.Fatalf("DurationMinutes = %f, want ~10", session.DurationMinutes)
	}
	if session.TokensUsed == 0 {
		t.Fatal("expected non-zero TokensUsed")
	}
}

func TestParseSessionFromLog_NoTimestamps(t *testing.T) {
	log := `Just some text without timestamps`
	session := ParseSessionFromLog([]byte(log), "lib")
	// Should fall back to default timestamps.
	if session.DurationMinutes < 9 {
		t.Fatalf("expected ~10min default duration, got %f", session.DurationMinutes)
	}
}

func TestParseSessionFromLog_Empty(t *testing.T) {
	session := ParseSessionFromLog([]byte(""), "lib")
	if session.ID == "" {
		t.Fatal("expected non-empty ID even for empty log")
	}
}
