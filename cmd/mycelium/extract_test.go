package main

import (
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// Tests for parseSessionFromLog edge cases — R9 area.
// Must exist BEFORE moving to internal/extraction/logparser.go.

func TestParseSessionFromLog_Empty(t *testing.T) {
	s := extraction.ParseSessionFromLog([]byte(""), "lib-1")
	if s.LibraryID != "lib-1" {
		t.Errorf("LibraryID = %q", s.LibraryID)
	}
	if s.ID == "" {
		t.Error("ID should be generated")
	}
	// With no timestamps, should use defaults
	if s.DurationMinutes < 0 {
		t.Errorf("negative duration: %f", s.DurationMinutes)
	}
}

func TestParseSessionFromLog_NoTimestamps(t *testing.T) {
	content := []byte("Some log without timestamps\ntool_call: exec ls\nerror: oops")
	s := extraction.ParseSessionFromLog(content, "lib-2")
	if s.ToolCallCount != 1 {
		t.Errorf("ToolCallCount = %d, want 1", s.ToolCallCount)
	}
	if s.ErrorCount < 1 {
		t.Errorf("ErrorCount = %d, want >= 1", s.ErrorCount)
	}
	// Should use 10min default
	if s.DurationMinutes < 9 || s.DurationMinutes > 11 {
		t.Errorf("DurationMinutes = %f, want ~10", s.DurationMinutes)
	}
}

func TestParseSessionFromLog_MultipleFormats(t *testing.T) {
	content := []byte("2025-06-01 09:00:00 start\n2025-06-01T09:30:00 end")
	s := extraction.ParseSessionFromLog(content, "lib-3")
	if s.DurationMinutes < 29 || s.DurationMinutes > 31 {
		t.Errorf("DurationMinutes = %f, want ~30", s.DurationMinutes)
	}
}

func TestParseSessionFromLog_OnlyOneTimestamp(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 something happened")
	s := extraction.ParseSessionFromLog(content, "lib-4")
	// With only one timestamp, should fallback to defaults (10min)
	if s.DurationMinutes < 9 || s.DurationMinutes > 11 {
		t.Errorf("DurationMinutes = %f, want ~10 (fallback)", s.DurationMinutes)
	}
}

func TestParseSessionFromLog_ToolCallPatterns(t *testing.T) {
	content := []byte(`tool_call: something
function_call: more
<tool_use>action</tool_use>
"type": "function"
<invoke name="test">`)
	s := extraction.ParseSessionFromLog(content, "lib-5")
	if s.ToolCallCount < 4 {
		t.Errorf("ToolCallCount = %d, want >= 4", s.ToolCallCount)
	}
}

func TestParseSessionFromLog_ErrorPatterns(t *testing.T) {
	content := []byte(`normal line
error: something broke
ERROR: critical
traceback (most recent call last):
panic: runtime error
FAILED test
exit status 1`)
	s := extraction.ParseSessionFromLog(content, "lib-6")
	if s.ErrorCount < 5 {
		t.Errorf("ErrorCount = %d, want >= 5", s.ErrorCount)
	}
}

func TestParseSessionFromLog_ExecDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"tool_call exec", "tool_call: exec ls", true},
		{"shell_exec", "shell_exec: pwd", true},
		{"command_output", "command_output: success", true},
		{`"name":"exec"`, `"name": "exec"`, true},
		{"no exec", "tool_call: read_file foo.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := extraction.ParseSessionFromLog([]byte(tt.content), "lib")
			if s.HasSuccessfulExec != tt.want {
				t.Errorf("HasSuccessfulExec = %v, want %v", s.HasSuccessfulExec, tt.want)
			}
		})
	}
}

func TestParseSessionFromLog_ModelDetection(t *testing.T) {
	content := []byte("Starting session model=gpt-4-turbo\nDoing stuff")
	s := extraction.ParseSessionFromLog(content, "lib")
	if s.AgentModel != "gpt-4-turbo" {
		t.Errorf("AgentModel = %q, want gpt-4-turbo", s.AgentModel)
	}
}

func TestParseSessionFromLog_TokenEstimate(t *testing.T) {
	content := make([]byte, 400) // 400 bytes ÷ 4 = 100 tokens
	for i := range content {
		content[i] = 'a'
	}
	s := extraction.ParseSessionFromLog(content, "lib")
	if s.TokensUsed != 100 {
		t.Errorf("TokensUsed = %d, want 100", s.TokensUsed)
	}
}

func TestParseSessionFromLog_NegativeDuration(t *testing.T) {
	// If timestamps are in wrong order, duration should be clamped to 0
	content := []byte("2025-06-01T10:00:00 end\n2025-06-01T09:00:00 start")
	s := extraction.ParseSessionFromLog(content, "lib")
	if s.DurationMinutes < 0 {
		t.Errorf("DurationMinutes = %f, should not be negative", s.DurationMinutes)
	}
}

func TestParseSessionFromLog_ErrorRate(t *testing.T) {
	content := []byte("tool_call: exec a\ntool_call: exec b\nerror: boom\ntool_call: exec c\nerror: crash")
	s := extraction.ParseSessionFromLog(content, "lib")
	if s.ToolCallCount == 0 {
		t.Fatal("no tool calls detected")
	}
	expectedRate := float64(s.ErrorCount) / float64(s.ToolCallCount)
	if s.ErrorRate != expectedRate {
		t.Errorf("ErrorRate = %f, want %f", s.ErrorRate, expectedRate)
	}
}

func TestParseSessionFromLog_ZeroToolCalls(t *testing.T) {
	content := []byte("just some text\nno tools here")
	s := extraction.ParseSessionFromLog(content, "lib")
	if s.ToolCallCount != 0 {
		t.Errorf("ToolCallCount = %d, want 0", s.ToolCallCount)
	}
	if s.ErrorRate != 0 {
		t.Errorf("ErrorRate = %f, want 0 (no tool calls)", s.ErrorRate)
	}
}

func TestParseSessionFromLog_UUID(t *testing.T) {
	s1 := extraction.ParseSessionFromLog([]byte("a"), "lib")
	s2 := extraction.ParseSessionFromLog([]byte("b"), "lib")
	if s1.ID == s2.ID {
		t.Error("expected unique IDs")
	}
	if len(s1.ID) != 36 {
		t.Errorf("ID length = %d, want 36 (UUID format)", len(s1.ID))
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := extraction.EstimateTokens([]byte("1234")); got != 1 {
		t.Errorf("extraction.EstimateTokens(4 bytes) = %d, want 1", got)
	}
	if got := extraction.EstimateTokens([]byte("")); got != 0 {
		t.Errorf("extraction.EstimateTokens(empty) = %d, want 0", got)
	}
}

func TestStoreQuerier_NilResult(t *testing.T) {
	// Verify that storage.NewSkillQuerier returns extraction.SkillQuerier.
	var _ extraction.SkillQuerier = storage.NewSkillQuerier(nil)
}

func TestExitError(t *testing.T) {
	e := &exitError{code: 2, msg: "test"}
	if e.Error() != "test" {
		t.Errorf("Error() = %q", e.Error())
	}
	if e.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d", e.ExitCode())
	}
}

// Ensure parseSessionFromLog handles large files without crash.
func TestParseSessionFromLog_LargeInput(t *testing.T) {
	// Build a 2MB log
	var lines []byte
	ts := time.Now().Format("2006-01-02T15:04:05")
	for i := 0; i < 20000; i++ {
		lines = append(lines, []byte(ts+" tool_call: exec something\n")...)
	}
	s := extraction.ParseSessionFromLog(lines, "lib")
	if s.ToolCallCount < 10000 {
		t.Errorf("ToolCallCount = %d, expected many", s.ToolCallCount)
	}
}
