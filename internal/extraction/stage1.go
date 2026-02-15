package extraction

import (
	"fmt"
	"log/slog"
	"strings"
)

// Stage1Filter performs metadata pre-filtering. Synchronous, cheap, no I/O.
type Stage1Filter interface {
	Filter(session SessionRecord) *Stage1Result
}

// Stage1Config holds thresholds for Stage 1 filtering.
type Stage1Config struct {
	MinDurationMinutes float64 // default: 2
	MaxDurationMinutes float64 // default: 180
	MinToolCalls       int     // default: 3
	MaxErrorRate       float64 // default: 0.7
}

// DefaultStage1Config returns the spec defaults.
func DefaultStage1Config() Stage1Config {
	return Stage1Config{
		MinDurationMinutes: 2,
		MaxDurationMinutes: 180,
		MinToolCalls:       3,
		MaxErrorRate:       0.7,
	}
}

type stage1Filter struct {
	cfg Stage1Config
}

// NewStage1Filter creates a Stage1Filter with the given config.
func NewStage1Filter(cfg Stage1Config) Stage1Filter {
	return &stage1Filter{cfg: cfg}
}

func (f *stage1Filter) Filter(session SessionRecord) *Stage1Result {
	result := &Stage1Result{
		DurationOK:      session.DurationMinutes >= f.cfg.MinDurationMinutes && session.DurationMinutes <= f.cfg.MaxDurationMinutes,
		ToolCallCountOK: session.ToolCallCount >= f.cfg.MinToolCalls,
		HasSuccessExec:  session.HasSuccessfulExec,
	}

	// Error rate: if zero tool calls, error rate is 0 (from spec: "0 if no tool calls")
	if session.ToolCallCount == 0 {
		result.ErrorRateOK = true // 0 error rate is within threshold, but will fail MinToolCalls
	} else {
		result.ErrorRateOK = session.ErrorRate <= f.cfg.MaxErrorRate
	}

	result.Passed = result.DurationOK && result.ToolCallCountOK && result.ErrorRateOK && result.HasSuccessExec

	if !result.Passed {
		var reasons []string
		if !result.DurationOK {
			reasons = append(reasons, fmt.Sprintf("duration %.1fm outside [%.1f, %.1f]", session.DurationMinutes, f.cfg.MinDurationMinutes, f.cfg.MaxDurationMinutes))
		}
		if !result.ToolCallCountOK {
			reasons = append(reasons, fmt.Sprintf("tool_calls %d < %d", session.ToolCallCount, f.cfg.MinToolCalls))
		}
		if !result.ErrorRateOK {
			reasons = append(reasons, fmt.Sprintf("error_rate %.2f > %.2f", session.ErrorRate, f.cfg.MaxErrorRate))
		}
		if !result.HasSuccessExec {
			reasons = append(reasons, "no successful execution")
		}
		result.Reason = strings.Join(reasons, "; ")

		slog.Info("stage1 reject", "session_id", session.ID, "reason", result.Reason)
	} else {
		slog.Info("stage1 pass", "session_id", session.ID)
	}

	return result
}
