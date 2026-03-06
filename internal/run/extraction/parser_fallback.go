package extraction

import (
	"bufio"
	"io"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Log-parsing regex patterns.
var (
	tsPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2})`),
	}
	toolPattern  = regexp.MustCompile(`(tool_call|function_call|<tool_use>|<invoke|"type"\s*:\s*"function")`)
	errorPattern = regexp.MustCompile(`((?:^|\s)error[:\s=]|(?:^|\s)ERROR[:\s=]|traceback \(most recent|panic:|fatal:|FAILED|exit status [1-9])`)
	execPattern  = regexp.MustCompile(`(tool_call.*exec|<exec|command_output|shell_exec|"name"\s*:\s*"exec")`)
	modelPattern = regexp.MustCompile(`(?i)model[=: ]+([a-zA-Z0-9._-]+)`)
)

// FallbackParser handles generic text session logs using regex patterns.
type FallbackParser struct{}

// CanParse returns true if the header is valid UTF-8 containing timestamp patterns.
func (p *FallbackParser) CanParse(header []byte) bool {
	if !utf8.Valid(header) {
		return false
	}
	for _, pat := range tsPatterns {
		if pat.Match(header) {
			return true
		}
	}
	return false
}

// Parse extracts session metadata using regex-based heuristics.
func (p *FallbackParser) Parse(r io.Reader) (*model.SessionRecord, error) {
	rec := &model.SessionRecord{}

	var timestamps []time.Time
	toolCalls := 0
	errorCount := 0
	msgCount := 0
	hasExec := false
	byteCount := 0

	scanner := bufio.NewScanner(r)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Bytes()
		byteCount += len(line) + 1
		msgCount++

		for _, pat := range tsPatterns {
			if m := pat.Find(line); m != nil {
				for _, layout := range []string{
					"2006-01-02T15:04:05",
					"2006-01-02 15:04:05",
				} {
					if t, parseErr := time.Parse(layout, string(m)); parseErr == nil {
						timestamps = append(timestamps, t)
						break
					}
				}
			}
		}

		if toolPattern.Match(line) {
			toolCalls++
		}
		if errorPattern.Match(line) {
			errorCount++
		}
		if execPattern.Match(line) {
			hasExec = true
		}
		if m := modelPattern.FindSubmatch(line); len(m) > 1 && rec.AgentModel == "" {
			rec.AgentModel = string(m[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if byteCount == 0 {
		return nil, ErrEmptyContent
	}

	if len(timestamps) >= 2 {
		rec.StartedAt = timestamps[0]
		rec.EndedAt = timestamps[len(timestamps)-1]
	}
	// No timestamps → zero StartedAt/EndedAt, zero duration. This is correct:
	// lying about duration corrupts Stage 1 filtering.

	rec.DurationMinutes = rec.EndedAt.Sub(rec.StartedAt).Minutes()
	if rec.DurationMinutes < 0 {
		rec.DurationMinutes = 0
	}

	rec.ToolCallCount = toolCalls
	rec.ErrorCount = errorCount
	rec.MessageCount = msgCount
	rec.HasSuccessfulExec = hasExec

	if rec.ToolCallCount > 0 {
		rec.ErrorRate = float64(rec.ErrorCount) / float64(rec.ToolCallCount)
	}

	rec.TokensUsed = byteCount / 4

	return rec, nil
}

// EstimateTokens provides a rough token estimate (~4 chars per token).
func EstimateTokens(content []byte) int {
	return len(content) / 4
}
