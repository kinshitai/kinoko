package extraction

import (
	"bufio"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kinoko-dev/kinoko/internal/model"
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

// ParseSessionFromLog extracts metadata from a session log file.
// Looks for common patterns: timestamps, tool calls, errors, model info.
func ParseSessionFromLog(content []byte, libraryID string) model.SessionRecord {
	lines := strings.Split(string(content), "\n")

	session := model.SessionRecord{
		ID:        uuid.Must(uuid.NewV7()).String(),
		LibraryID: libraryID,
	}

	var timestamps []time.Time
	toolCalls := 0
	errorCount := 0
	msgCount := len(lines)
	hasExec := false

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()

		for _, pat := range tsPatterns {
			if m := pat.FindString(line); m != "" {
				for _, layout := range []string{
					"2006-01-02T15:04:05",
					"2006-01-02 15:04:05",
				} {
					if t, err := time.Parse(layout, m); err == nil {
						timestamps = append(timestamps, t)
						break
					}
				}
			}
		}

		if toolPattern.MatchString(line) {
			toolCalls++
		}
		if errorPattern.MatchString(line) {
			errorCount++
		}
		if execPattern.MatchString(line) {
			hasExec = true
		}
		if m := modelPattern.FindStringSubmatch(line); len(m) > 1 && session.AgentModel == "" {
			session.AgentModel = m[1]
		}
	}

	now := time.Now()
	if len(timestamps) >= 2 {
		session.StartedAt = timestamps[0]
		session.EndedAt = timestamps[len(timestamps)-1]
	} else {
		session.StartedAt = now.Add(-10 * time.Minute)
		session.EndedAt = now
	}

	session.DurationMinutes = session.EndedAt.Sub(session.StartedAt).Minutes()
	if session.DurationMinutes < 0 {
		session.DurationMinutes = 0
	}

	session.ToolCallCount = toolCalls
	session.ErrorCount = errorCount
	session.MessageCount = msgCount
	session.HasSuccessfulExec = hasExec

	if session.ToolCallCount > 0 {
		session.ErrorRate = float64(session.ErrorCount) / float64(session.ToolCallCount)
	}

	session.TokensUsed = EstimateTokens(content)

	return session
}

// EstimateTokens provides a rough token estimate (~4 chars per token).
func EstimateTokens(content []byte) int {
	return len(content) / 4
}
