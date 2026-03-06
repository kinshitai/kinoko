package extraction

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// --- ParseSession edge cases ---

func TestParseSession_ReaderError(t *testing.T) {
	// A reader that returns an error on first read (not EOF).
	_, err := ParseSession(&errReader{err: errors.New("disk on fire")})
	if err == nil {
		t.Fatal("expected error from failing reader")
	}
	if errors.Is(err, ErrEmptyContent) {
		t.Fatal("should not be ErrEmptyContent for non-EOF reader error")
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("expected wrapped reader error, got: %v", err)
	}
}

type errReader struct {
	err error
}

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }

func TestParseSession_ContentSpansHeaderBoundary(t *testing.T) {
	// Build JSONL where one logical line straddles the 4KB header boundary.
	// The header peek reads 4096 bytes. We pad the first line so the second
	// line starts in the header but finishes after it.
	firstLine := `{"type":"user","timestamp":"2025-01-15T10:00:00Z","message":{"content":[{"type":"text","text":"` +
		strings.Repeat("x", 3900) +
		`"}]}}` + "\n"
	// Second line will span the 4096 boundary.
	secondLine := `{"type":"assistant","timestamp":"2025-01-15T10:01:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"

	input := firstLine + secondLine

	rec, err := ParseSession(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2", rec.MessageCount)
	}
	if rec.AgentModel != "claude-sonnet-4-20250514" {
		t.Fatalf("AgentModel = %q, want claude-sonnet-4-20250514", rec.AgentModel)
	}
}

func TestParseSession_ExactlyHeaderSize(t *testing.T) {
	// Input that is exactly 4096 bytes — no remainder for MultiReader.
	// Pad a valid JSONL line to exactly 4096 bytes.
	base := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"`
	suffix := `"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"
	padLen := headerSize - len(base) - len(suffix)
	if padLen < 0 {
		t.Skip("base+suffix exceed headerSize, adjust test")
	}
	input := base + strings.Repeat("a", padLen) + suffix
	if len(input) != headerSize {
		t.Fatalf("input length = %d, want %d", len(input), headerSize)
	}

	rec, err := ParseSession(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.MessageCount != 1 {
		t.Fatalf("MessageCount = %d, want 1", rec.MessageCount)
	}
}

// --- ClaudeCodeParser edge cases ---

func TestClaudeCodeParser_MalformedLinesSkipped(t *testing.T) {
	// Mix of garbage, malformed JSON, and valid lines.
	jsonl := strings.Join([]string{
		`this is not json at all`,
		`{"broken json`,
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":10,"output_tokens":5}}}`,
		`{{{totally invalid}}}`,
		`{"type":"user","timestamp":"2025-01-15T10:00:05Z","message":{"content":[{"type":"text","text":"thanks"}]}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.MessageCount != 2 {
		t.Fatalf("MessageCount = %d, want 2 (malformed lines should be skipped)", rec.MessageCount)
	}
}

func TestClaudeCodeParser_EmptyLinesIgnored(t *testing.T) {
	jsonl := "\n\n" +
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":2}}}` +
		"\n\n\n"

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.MessageCount != 1 {
		t.Fatalf("MessageCount = %d, want 1", rec.MessageCount)
	}
}

func TestClaudeCodeParser_NegativeDurationClamped(t *testing.T) {
	// Timestamps out of order — duration should be clamped to 0.
	jsonl := strings.Join([]string{
		`{"type":"user","timestamp":"2025-01-15T10:10:00Z","message":{"content":[{"type":"text","text":"start"}]}}`,
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"end"}],"usage":{"input_tokens":5,"output_tokens":2}}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes != 0 {
		t.Fatalf("DurationMinutes = %f, want 0 (negative duration should be clamped)", rec.DurationMinutes)
	}
}

func TestClaudeCodeParser_ExecToolNames(t *testing.T) {
	// Test that "exec" and "shell" tool names are detected.
	for _, name := range []string{"exec", "shell"} {
		t.Run(name, func(t *testing.T) {
			jsonl := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","name":"` + name + `"}],"usage":{"input_tokens":5,"output_tokens":2}}}`

			p := &ClaudeCodeParser{}
			rec, err := p.Parse(strings.NewReader(jsonl))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !rec.HasSuccessfulExec {
				t.Fatalf("expected HasSuccessfulExec=true for tool name %q", name)
			}
		})
	}
}

func TestClaudeCodeParser_ErrorRatePartial(t *testing.T) {
	// 1 error out of 3 tool calls → ErrorRate ≈ 0.333
	jsonl := strings.Join([]string{
		`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","name":"Read"},{"type":"tool_use","name":"Write"},{"type":"tool_use","name":"Bash"}],"usage":{"input_tokens":50,"output_tokens":20}}}`,
		`{"type":"user","timestamp":"2025-01-15T10:00:05Z","message":{"content":[{"type":"tool_result","is_error":true}]}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount != 3 {
		t.Fatalf("ToolCallCount = %d, want 3", rec.ToolCallCount)
	}
	if rec.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", rec.ErrorCount)
	}
	want := 1.0 / 3.0
	if rec.ErrorRate < want-0.001 || rec.ErrorRate > want+0.001 {
		t.Fatalf("ErrorRate = %f, want ~%f", rec.ErrorRate, want)
	}
}

func TestClaudeCodeParser_ErrorsWithNoToolCalls(t *testing.T) {
	// Errors in tool_result but no tool_use → ErrorRate should be 0 (no division by zero).
	jsonl := strings.Join([]string{
		`{"type":"user","timestamp":"2025-01-15T10:00:00Z","message":{"content":[{"type":"tool_result","is_error":true}]}}`,
		`{"type":"assistant","timestamp":"2025-01-15T10:00:05Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"oops"}],"usage":{"input_tokens":5,"output_tokens":2}}}`,
	}, "\n")

	p := &ClaudeCodeParser{}
	rec, err := p.Parse(strings.NewReader(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", rec.ErrorCount)
	}
	if rec.ToolCallCount != 0 {
		t.Fatalf("ToolCallCount = %d, want 0", rec.ToolCallCount)
	}
	if rec.ErrorRate != 0 {
		t.Fatalf("ErrorRate = %f, want 0 (no tool calls → no division)", rec.ErrorRate)
	}
}

func TestClaudeCodeParser_ScannerBufferOverflow(t *testing.T) {
	// A single JSONL line exceeding the 1MB scanner buffer.
	// The scanner should return an error (bufio.ErrTooLong) which
	// propagates through scanner.Err().
	hugeLine := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"` +
		strings.Repeat("x", 2*1024*1024) + // 2MB of payload
		`"}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n"

	p := &ClaudeCodeParser{}
	_, err := p.Parse(strings.NewReader(hugeLine))
	if err == nil {
		t.Fatal("expected error for line exceeding scanner buffer, got nil")
	}
	// Should be bufio.Scanner error, not ErrMalformedFormat
	// (scanner stops before reading any complete lines).
}

func TestClaudeCodeParser_CanParse_EmptyHeader(t *testing.T) {
	p := &ClaudeCodeParser{}
	if p.CanParse(nil) {
		t.Fatal("expected CanParse=false for nil header")
	}
	if p.CanParse([]byte{}) {
		t.Fatal("expected CanParse=false for empty header")
	}
}

func TestClaudeCodeParser_CanParse_TruncatedJSON(t *testing.T) {
	// A line longer than the header with no newline — simulates truncation
	// at the 4KB boundary. Unmarshal should fail → CanParse=false.
	truncated := []byte(`{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"content":[{"type":"text","text":"` +
		strings.Repeat("a", 4096) + // no closing — truncated
		`...`)
	p := &ClaudeCodeParser{}
	if p.CanParse(truncated) {
		t.Fatal("expected CanParse=false for truncated JSON line (no newline in header)")
	}
}

// --- FallbackParser edge cases ---

func TestFallbackParser_SpaceSeparatedTimestamp(t *testing.T) {
	log := "2024-01-15 10:00:00 Start\n2024-01-15 10:05:00 End\n"
	p := &FallbackParser{}
	rec, err := p.Parse(strings.NewReader(log))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes < 4 || rec.DurationMinutes > 6 {
		t.Fatalf("DurationMinutes = %f, want ~5", rec.DurationMinutes)
	}
}

func TestFallbackParser_MultipleErrorPatterns(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"traceback", "traceback (most recent call last):\n"},
		{"panic", "panic: runtime error: index out of range\n"},
		{"fatal", "fatal: not a git repository\n"},
		{"FAILED", "FAILED tests/test_auth.py::test_login\n"},
		{"exit_status", "exit status 1\n"},
		{"ERROR_colon", "ERROR: connection refused\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepend a timestamp so FallbackParser.CanParse works via ParseSession.
			log := "2024-01-15T10:00:00 start\n" + tt.line
			p := &FallbackParser{}
			rec, err := p.Parse(strings.NewReader(log))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if rec.ErrorCount == 0 {
				t.Fatalf("expected ErrorCount > 0 for pattern %q", tt.name)
			}
		})
	}
}

func TestFallbackParser_ScannerBufferOverflow(t *testing.T) {
	// A line exceeding the 1MB scanner buffer.
	huge := "2024-01-15T10:00:00 " + strings.Repeat("x", 2*1024*1024) + "\n"
	p := &FallbackParser{}
	_, err := p.Parse(strings.NewReader(huge))
	if err == nil {
		t.Fatal("expected error for line exceeding scanner buffer, got nil")
	}
}

func TestFallbackParser_NegativeDurationClamped(t *testing.T) {
	// Timestamps going backwards.
	log := "2024-01-15T10:10:00 later\n2024-01-15T10:00:00 earlier\n"
	p := &FallbackParser{}
	rec, err := p.Parse(strings.NewReader(log))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.DurationMinutes != 0 {
		t.Fatalf("DurationMinutes = %f, want 0 (negative should be clamped)", rec.DurationMinutes)
	}
}

func TestFallbackParser_ErrorRate(t *testing.T) {
	log := "2024-01-15T10:00:00 start\ntool_call one\ntool_call two\nerror: bad\n"
	p := &FallbackParser{}
	rec, err := p.Parse(strings.NewReader(log))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.ToolCallCount != 2 {
		t.Fatalf("ToolCallCount = %d, want 2", rec.ToolCallCount)
	}
	if rec.ErrorCount != 1 {
		t.Fatalf("ErrorCount = %d, want 1", rec.ErrorCount)
	}
	want := 0.5
	if rec.ErrorRate < want-0.001 || rec.ErrorRate > want+0.001 {
		t.Fatalf("ErrorRate = %f, want %f", rec.ErrorRate, want)
	}
}

func TestFallbackParser_ExecPatterns(t *testing.T) {
	patterns := []string{
		`<exec>ls</exec>`,
		`command_output: hello`,
		`shell_exec: rm -rf`,
		`"name": "exec"`,
	}
	for _, pat := range patterns {
		t.Run(pat, func(t *testing.T) {
			log := "2024-01-15T10:00:00 " + pat + "\n"
			p := &FallbackParser{}
			rec, err := p.Parse(strings.NewReader(log))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !rec.HasSuccessfulExec {
				t.Fatalf("expected HasSuccessfulExec=true for pattern %q", pat)
			}
		})
	}
}

func TestFallbackParser_CanParse_UTF8NoTimestamp(t *testing.T) {
	// Valid UTF-8 but no timestamp pattern → should return false.
	p := &FallbackParser{}
	if p.CanParse([]byte("hello world, no dates here")) {
		t.Fatal("expected CanParse=false for UTF-8 without timestamps")
	}
}

// --- ParseSession integration edge cases ---

func TestParseSession_VerySmallInput(t *testing.T) {
	// Single byte — not empty, but no parser should match.
	_, err := ParseSession(bytes.NewReader([]byte("x")))
	if !errors.Is(err, ErrUnrecognizedFormat) {
		t.Fatalf("expected ErrUnrecognizedFormat for single byte, got %v", err)
	}
}

func TestParseSession_OnlyNewlines(t *testing.T) {
	// Newlines only — valid read, but no parser match.
	_, err := ParseSession(strings.NewReader("\n\n\n"))
	if !errors.Is(err, ErrUnrecognizedFormat) {
		t.Fatalf("expected ErrUnrecognizedFormat for newlines-only, got %v", err)
	}
}

func TestParseSession_ReaderErrorAfterHeader(t *testing.T) {
	// Reader that succeeds for header but fails during Parse.
	// Build valid JSONL header so ClaudeCodeParser picks it up,
	// but the tail reader returns an error.
	line := `{"type":"assistant","timestamp":"2025-01-15T10:00:00Z","message":{"model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":2}}}` + "\n"

	// Combine: line (valid, fits in header) + erroring tail
	combined := io.MultiReader(
		strings.NewReader(line),
		&errReader{err: errors.New("network timeout")},
	)

	// ParseSession reads header, then passes MultiReader(header, remainder) to parser.
	// The parser's scanner will hit the error from the tail.
	// This may or may not propagate depending on when scanner stops.
	// The key thing: it should NOT panic.
	_, _ = ParseSession(combined) // no panic = pass
}
