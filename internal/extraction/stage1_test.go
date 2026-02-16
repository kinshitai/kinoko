package extraction

import (
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"

	"github.com/kinoko-dev/kinoko/internal/config"
)

func defaultTestConfig() config.ExtractionConfig {
	c := config.DefaultConfig()
	return c.Extraction
}

func passingSession() model.SessionRecord {
	return model.SessionRecord{
		ID:                "test-session-1",
		StartedAt:         time.Now().Add(-10 * time.Minute),
		EndedAt:           time.Now(),
		DurationMinutes:   10,
		ToolCallCount:     5,
		ErrorCount:        1,
		MessageCount:      12,
		ErrorRate:         0.2,
		HasSuccessfulExec: true,
		TokensUsed:        5000,
		AgentModel:        "claude-3",
		UserID:            "user-1",
		LibraryID:         "lib-1",
	}
}

func TestStage1Filter(t *testing.T) {
	tests := []struct {
		name            string
		mutate          func(*model.SessionRecord)
		cfg             *config.ExtractionConfig
		passed          bool
		durationOK      bool
		toolCallCountOK bool
		errorRateOK     bool
		hasSuccessExec  bool
		reasonContains  []string
	}{
		{
			name:            "happy path",
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name:            "duration too short",
			mutate:          func(s *model.SessionRecord) { s.DurationMinutes = 1 },
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
		{
			name:            "duration too long",
			mutate:          func(s *model.SessionRecord) { s.DurationMinutes = 200 },
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
		{
			name:            "too few tool calls",
			mutate:          func(s *model.SessionRecord) { s.ToolCallCount = 2; s.ErrorCount = 0; s.ErrorRate = 0 },
			durationOK:      true,
			toolCallCountOK: false,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"tool_calls"},
		},
		{
			name:            "error rate too high",
			mutate:          func(s *model.SessionRecord) { s.ErrorRate = 0.8; s.ErrorCount = 4 },
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     false,
			hasSuccessExec:  true,
			reasonContains:  []string{"error_rate"},
		},
		{
			name:            "no successful exec",
			mutate:          func(s *model.SessionRecord) { s.HasSuccessfulExec = false },
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  false,
			reasonContains:  []string{"no successful execution"},
		},
		{
			name:            "boundary min duration",
			mutate:          func(s *model.SessionRecord) { s.DurationMinutes = 2 },
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name:            "boundary max duration",
			mutate:          func(s *model.SessionRecord) { s.DurationMinutes = 180 },
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name:            "boundary min tool calls",
			mutate:          func(s *model.SessionRecord) { s.ToolCallCount = 3; s.ErrorCount = 0; s.ErrorRate = 0 },
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name:            "boundary max error rate",
			mutate:          func(s *model.SessionRecord) { s.ErrorRate = 0.7; s.ErrorCount = 3 },
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name: "zero tool calls fails min check",
			mutate: func(s *model.SessionRecord) {
				s.ToolCallCount = 0
				s.ErrorCount = 0
				s.ErrorRate = 0
			},
			durationOK:      true,
			toolCallCountOK: false,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"tool_calls"},
		},
		{
			name: "multiple failures",
			mutate: func(s *model.SessionRecord) {
				s.DurationMinutes = 0.5
				s.ToolCallCount = 1
				s.ErrorCount = 1
				s.ErrorRate = 0.9
				s.HasSuccessfulExec = false
			},
			durationOK:      false,
			toolCallCountOK: false,
			errorRateOK:     false,
			hasSuccessExec:  false,
			reasonContains:  []string{"duration", "tool_calls", "error_rate", "no successful execution"},
		},
		{
			name: "custom config",
			cfg: &config.ExtractionConfig{
				MinDurationMinutes: 5,
				MaxDurationMinutes: 60,
				MinToolCalls:       10,
				MaxErrorRate:       0.3,
			},
			mutate:          nil, // uses passingSession (duration=10, tools=5)
			durationOK:      true,
			toolCallCountOK: false,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"tool_calls"},
		},
		// Edge cases
		{
			name:            "negative duration",
			mutate:          func(s *model.SessionRecord) { s.DurationMinutes = -5 },
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
		{
			name: "negative tool calls",
			mutate: func(s *model.SessionRecord) {
				s.ToolCallCount = -1
				s.ErrorCount = 0
				s.ErrorRate = 0
			},
			durationOK:      true,
			toolCallCountOK: false,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"tool_calls"},
		},
		{
			name: "NaN duration",
			mutate: func(s *model.SessionRecord) {
				s.DurationMinutes = math.NaN()
			},
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
		{
			name: "Inf duration",
			mutate: func(s *model.SessionRecord) {
				s.DurationMinutes = math.Inf(1)
			},
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
		{
			name: "NaN error rate",
			mutate: func(s *model.SessionRecord) {
				s.ErrorRate = math.NaN()
			},
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     false,
			hasSuccessExec:  true,
			reasonContains:  []string{"error_rate"},
		},
		{
			name: "zero config values",
			cfg: &config.ExtractionConfig{
				MinDurationMinutes: 0,
				MaxDurationMinutes: 0,
				MinToolCalls:       0,
				MaxErrorRate:       0,
			},
			mutate: func(s *model.SessionRecord) {
				s.DurationMinutes = 0
				s.ToolCallCount = 0
				s.ErrorCount = 0
				s.ErrorRate = 0
			},
			passed:          true,
			durationOK:      true,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
		},
		{
			name: "inverted min/max duration config",
			cfg: &config.ExtractionConfig{
				MinDurationMinutes: 100,
				MaxDurationMinutes: 10,
				MinToolCalls:       3,
				MaxErrorRate:       0.7,
			},
			durationOK:      false,
			toolCallCountOK: true,
			errorRateOK:     true,
			hasSuccessExec:  true,
			reasonContains:  []string{"duration"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultTestConfig()
			if tt.cfg != nil {
				cfg = *tt.cfg
			}
			f := NewStage1Filter(cfg, slog.New(slog.NewTextHandler(devNull{}, nil)))

			s := passingSession()
			if tt.mutate != nil {
				tt.mutate(&s)
			}

			r := f.Filter(s)

			if r.DurationOK != tt.durationOK {
				t.Errorf("DurationOK = %v, want %v", r.DurationOK, tt.durationOK)
			}
			if r.ToolCallCountOK != tt.toolCallCountOK {
				t.Errorf("ToolCallCountOK = %v, want %v", r.ToolCallCountOK, tt.toolCallCountOK)
			}
			if r.ErrorRateOK != tt.errorRateOK {
				t.Errorf("ErrorRateOK = %v, want %v", r.ErrorRateOK, tt.errorRateOK)
			}
			if r.HasSuccessExec != tt.hasSuccessExec {
				t.Errorf("HasSuccessExec = %v, want %v", r.HasSuccessExec, tt.hasSuccessExec)
			}

			wantPassed := tt.passed
			if !tt.passed {
				// If passed not explicitly set to true, derive it
				wantPassed = tt.durationOK && tt.toolCallCountOK && tt.errorRateOK && tt.hasSuccessExec
			}
			if r.Passed != wantPassed {
				t.Errorf("Passed = %v, want %v (reason: %s)", r.Passed, wantPassed, r.Reason)
			}

			for _, substr := range tt.reasonContains {
				if !strings.Contains(r.Reason, substr) {
					t.Errorf("reason %q missing substring %q", r.Reason, substr)
				}
			}

			if r.Passed && r.Reason != "" {
				t.Errorf("passed but has reason: %s", r.Reason)
			}
		})
	}
}

// devNull implements io.Writer for discarding log output in tests.
type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }
