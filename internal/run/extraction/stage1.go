package extraction

import (
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

type stage1Filter struct {
	minDuration float64
	maxDuration float64
	minTools    int
	maxError    float64
	log         *slog.Logger
}

// NewStage1Filter creates a Stage1Filter from ExtractionConfig.
func NewStage1Filter(cfg config.ExtractionConfig, log *slog.Logger) Stage1Filter {
	return &stage1Filter{
		minDuration: cfg.MinDurationMinutes,
		maxDuration: cfg.MaxDurationMinutes,
		minTools:    cfg.MinToolCalls,
		maxError:    cfg.MaxErrorRate,
		log:         log,
	}
}

func (f *stage1Filter) Filter(session model.SessionRecord) *model.Stage1Result {
	// Validate ErrorRate consistency
	if session.ToolCallCount > 0 {
		expected := float64(session.ErrorCount) / float64(session.ToolCallCount)
		if math.Abs(expected-session.ErrorRate) > 0.001 {
			f.log.Warn("stage1: ErrorRate inconsistent",
				"session_id", session.ID,
				"error_rate", session.ErrorRate,
				"expected", expected,
				"error_count", session.ErrorCount,
				"tool_call_count", session.ToolCallCount,
			)
		}
	}

	result := &model.Stage1Result{
		DurationOK:      session.DurationMinutes >= f.minDuration && session.DurationMinutes <= f.maxDuration,
		ToolCallCountOK: session.ToolCallCount >= f.minTools,
		ErrorRateOK:     session.ErrorRate <= f.maxError,
		HasSuccessExec:  session.HasSuccessfulExec,
	}

	result.Passed = result.DurationOK && result.ToolCallCountOK && result.ErrorRateOK && result.HasSuccessExec

	if !result.Passed {
		var reasons []string
		if !result.DurationOK {
			reasons = append(reasons, fmt.Sprintf("duration %.1fm outside [%.1f, %.1f]", session.DurationMinutes, f.minDuration, f.maxDuration))
		}
		if !result.ToolCallCountOK {
			reasons = append(reasons, fmt.Sprintf("tool_calls %d < %d", session.ToolCallCount, f.minTools))
		}
		if !result.ErrorRateOK {
			reasons = append(reasons, fmt.Sprintf("error_rate %.2f > %.2f", session.ErrorRate, f.maxError))
		}
		if !result.HasSuccessExec {
			reasons = append(reasons, "no successful execution")
		}
		result.Reason = strings.Join(reasons, "; ")
		f.log.Info("stage1 reject", "session_id", session.ID, "reason", result.Reason)
	} else {
		f.log.Info("stage1 pass", "session_id", session.ID)
	}

	return result
}
