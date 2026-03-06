package extraction

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

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
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, ErrEmptyContent
	}

	rec := &model.SessionRecord{}

	var timestamps []time.Time
	toolCalls := 0
	errorCount := 0
	msgCount := 0
	hasExec := false

	scanner := bufio.NewScanner(bytes.NewReader(content))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Bytes()
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

	rec.TokensUsed = EstimateTokens(content)

	return rec, nil
}

// EstimateTokens provides a rough token estimate (~4 chars per token).
func EstimateTokens(content []byte) int {
	return len(content) / 4
}

// ParseSessionFromLog is a compatibility shim for callers not yet migrated
// to ParseSession. It preserves legacy behavior (10min default for missing
// timestamps) to avoid breaking callers. Will be removed in the next commit.
func ParseSessionFromLog(content []byte, libraryID string) model.SessionRecord {
	p := &FallbackParser{}
	rec, err := p.Parse(bytes.NewReader(content))
	if err != nil || rec == nil {
		rec = &model.SessionRecord{}
	}
	rec.ID = uuid.Must(uuid.NewV7()).String()
	rec.LibraryID = libraryID

	// Legacy behavior: fake 10min duration when no timestamps found.
	if rec.StartedAt.IsZero() && rec.EndedAt.IsZero() {
		now := time.Now()
		rec.StartedAt = now.Add(-10 * time.Minute)
		rec.EndedAt = now
		rec.DurationMinutes = rec.EndedAt.Sub(rec.StartedAt).Minutes()
	}

	return *rec
}
