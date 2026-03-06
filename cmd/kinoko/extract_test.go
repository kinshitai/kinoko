package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/apiclient"
	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestParseSession_Empty(t *testing.T) {
	_, err := extraction.ParseSession(bytes.NewReader(nil))
	if !errors.Is(err, extraction.ErrEmptyContent) {
		t.Errorf("expected ErrEmptyContent, got %v", err)
	}
}

func TestParseSession_NoTimestamps(t *testing.T) {
	content := []byte("Some log without timestamps\ntool_call: exec ls\nerror: oops")
	// No timestamps → FallbackParser won't match (CanParse checks for timestamps).
	// This becomes ErrUnrecognizedFormat.
	_, err := extraction.ParseSession(bytes.NewReader(content))
	if !errors.Is(err, extraction.ErrUnrecognizedFormat) {
		t.Errorf("expected ErrUnrecognizedFormat, got %v", err)
	}
}

func TestParseSession_MultipleFormats(t *testing.T) {
	content := []byte("2025-06-01 09:00:00 start\n2025-06-01T09:30:00 end")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes < 29 || rec.DurationMinutes > 31 {
		t.Errorf("DurationMinutes = %f, want ~30", rec.DurationMinutes)
	}
}

func TestParseSession_OnlyOneTimestamp(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 something happened")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With only one timestamp, duration is zero (no lying about it).
	if rec.DurationMinutes != 0 {
		t.Errorf("DurationMinutes = %f, want 0", rec.DurationMinutes)
	}
}

func TestParseSession_ToolCallPatterns(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 start\ntool_call: something\nfunction_call: more\n<tool_use>action</tool_use>\n\"type\": \"function\"\n<invoke name=\"test\">")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount < 4 {
		t.Errorf("ToolCallCount = %d, want >= 4", rec.ToolCallCount)
	}
}

func TestParseSession_ErrorPatterns(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 start\nnormal line\nerror: something broke\nERROR: critical\ntraceback (most recent call last):\npanic: runtime error\nFAILED test\nexit status 1")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ErrorCount < 5 {
		t.Errorf("ErrorCount = %d, want >= 5", rec.ErrorCount)
	}
}

func TestParseSession_ExecDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"tool_call exec", "2025-06-01T09:00:00 tool_call: exec ls", true},
		{"shell_exec", "2025-06-01T09:00:00 shell_exec: pwd", true},
		{"command_output", "2025-06-01T09:00:00 command_output: success", true},
		{`"name":"exec"`, `2025-06-01T09:00:00 "name": "exec"`, true},
		{"no exec", "2025-06-01T09:00:00 tool_call: read_file foo.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := extraction.ParseSession(strings.NewReader(tt.content))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.HasSuccessfulExec != tt.want {
				t.Errorf("HasSuccessfulExec = %v, want %v", rec.HasSuccessfulExec, tt.want)
			}
		})
	}
}

func TestParseSession_ModelDetection(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 Starting session model=gpt-4-turbo\nDoing stuff")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.AgentModel != "gpt-4-turbo" {
		t.Errorf("AgentModel = %q, want gpt-4-turbo", rec.AgentModel)
	}
}

func TestParseSession_TokenEstimate(t *testing.T) {
	// 400 bytes of content with a timestamp so FallbackParser matches.
	content := make([]byte, 0, 430)
	content = append(content, []byte("2025-06-01T09:00:00 ")...)
	for i := 0; i < 400; i++ {
		content = append(content, 'a')
	}
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := len(content) / 4
	if rec.TokensUsed != expected {
		t.Errorf("TokensUsed = %d, want %d", rec.TokensUsed, expected)
	}
}

func TestParseSession_NegativeDuration(t *testing.T) {
	// If timestamps are in wrong order, duration should be clamped to 0
	content := []byte("2025-06-01T10:00:00 end\n2025-06-01T09:00:00 start")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes < 0 {
		t.Errorf("DurationMinutes = %f, should not be negative", rec.DurationMinutes)
	}
}

func TestParseSession_ErrorRate(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 start\ntool_call: exec a\ntool_call: exec b\nerror: boom\ntool_call: exec c\nerror: crash")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount == 0 {
		t.Fatal("no tool calls detected")
	}
	expectedRate := float64(rec.ErrorCount) / float64(rec.ToolCallCount)
	if rec.ErrorRate != expectedRate {
		t.Errorf("ErrorRate = %f, want %f", rec.ErrorRate, expectedRate)
	}
}

func TestParseSession_ZeroToolCalls(t *testing.T) {
	content := []byte("2025-06-01T09:00:00 just some text\nno tools here")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount != 0 {
		t.Errorf("ToolCallCount = %d, want 0", rec.ToolCallCount)
	}
	if rec.ErrorRate != 0 {
		t.Errorf("ErrorRate = %f, want 0 (no tool calls)", rec.ErrorRate)
	}
}

func TestParseSession_NoIDGenerated(t *testing.T) {
	// ParseSession does NOT generate UUIDs — caller stamps them.
	content := []byte("2025-06-01T09:00:00 hello")
	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ID != "" {
		t.Errorf("ID = %q, want empty (caller stamps it)", rec.ID)
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

func TestHTTPQuerier_Interface(t *testing.T) {
	// Verify that apiclient.NewHTTPQuerier returns model.SkillQuerier.
	var _ model.SkillQuerier = apiclient.NewHTTPQuerier(nil)
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

// Ensure ParseSession handles large files without crash.
func TestParseSession_LargeInput(t *testing.T) {
	var buf bytes.Buffer
	ts := time.Now().Format("2006-01-02T15:04:05")
	for i := 0; i < 20000; i++ {
		buf.WriteString(ts + " tool_call: exec something\n")
	}
	rec, err := extraction.ParseSession(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount < 10000 {
		t.Errorf("ToolCallCount = %d, expected many", rec.ToolCallCount)
	}
}
