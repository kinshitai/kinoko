package extraction

import (
	"testing"
	"time"
)

func validSession() SessionRecord {
	return SessionRecord{
		ID:                "test-session-1",
		StartedAt:         time.Now().Add(-10 * time.Minute),
		EndedAt:           time.Now(),
		DurationMinutes:   10,
		ToolCallCount:     5,
		ErrorCount:        1,
		ErrorRate:         0.2,
		HasSuccessfulExec: true,
	}
}

func TestStage1Filter_HappyPath(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	r := f.Filter(validSession())
	if !r.Passed {
		t.Fatalf("expected pass, got reject: %s", r.Reason)
	}
	if !r.DurationOK || !r.ToolCallCountOK || !r.ErrorRateOK || !r.HasSuccessExec {
		t.Fatal("expected all checks true")
	}
	if r.Reason != "" {
		t.Fatalf("expected empty reason, got: %s", r.Reason)
	}
}

func TestStage1Filter_DurationTooShort(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.DurationMinutes = 1
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	if r.DurationOK {
		t.Fatal("expected DurationOK false")
	}
}

func TestStage1Filter_DurationTooLong(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.DurationMinutes = 200
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	if r.DurationOK {
		t.Fatal("expected DurationOK false")
	}
}

func TestStage1Filter_TooFewToolCalls(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.ToolCallCount = 2
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	if r.ToolCallCountOK {
		t.Fatal("expected ToolCallCountOK false")
	}
}

func TestStage1Filter_ErrorRateTooHigh(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.ErrorRate = 0.8
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	if r.ErrorRateOK {
		t.Fatal("expected ErrorRateOK false")
	}
}

func TestStage1Filter_NoSuccessfulExec(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.HasSuccessfulExec = false
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	if r.HasSuccessExec {
		t.Fatal("expected HasSuccessExec false")
	}
}

func TestStage1Filter_BoundaryMinDuration(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.DurationMinutes = 2 // exactly at threshold
	r := f.Filter(s)
	if !r.DurationOK {
		t.Fatal("exactly at min duration should pass")
	}
}

func TestStage1Filter_BoundaryMaxDuration(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.DurationMinutes = 180 // exactly at threshold
	r := f.Filter(s)
	if !r.DurationOK {
		t.Fatal("exactly at max duration should pass")
	}
}

func TestStage1Filter_BoundaryMinToolCalls(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.ToolCallCount = 3 // exactly at threshold
	r := f.Filter(s)
	if !r.ToolCallCountOK {
		t.Fatal("exactly at min tool calls should pass")
	}
}

func TestStage1Filter_BoundaryMaxErrorRate(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.ErrorRate = 0.7 // exactly at threshold
	r := f.Filter(s)
	if !r.ErrorRateOK {
		t.Fatal("exactly at max error rate should pass")
	}
}

func TestStage1Filter_ZeroToolCalls(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := validSession()
	s.ToolCallCount = 0
	s.ErrorRate = 0 // spec: 0 if no tool calls
	r := f.Filter(s)
	// ErrorRateOK should be true (0 <= 0.7), but ToolCallCountOK should be false
	if !r.ErrorRateOK {
		t.Fatal("zero tool calls should have OK error rate")
	}
	if r.ToolCallCountOK {
		t.Fatal("zero tool calls should fail min tool calls check")
	}
	if r.Passed {
		t.Fatal("should not pass overall")
	}
}

func TestStage1Filter_MultipleFailures(t *testing.T) {
	f := NewStage1Filter(DefaultStage1Config())
	s := SessionRecord{
		ID:                "multi-fail",
		DurationMinutes:   0.5,
		ToolCallCount:     1,
		ErrorRate:         0.9,
		HasSuccessfulExec: false,
	}
	r := f.Filter(s)
	if r.Passed {
		t.Fatal("expected reject")
	}
	// All four checks should fail
	if r.DurationOK || r.ToolCallCountOK || r.ErrorRateOK || r.HasSuccessExec {
		t.Fatal("expected all checks false")
	}
}

func TestStage1Filter_CustomConfig(t *testing.T) {
	cfg := Stage1Config{
		MinDurationMinutes: 5,
		MaxDurationMinutes: 60,
		MinToolCalls:       10,
		MaxErrorRate:       0.3,
	}
	f := NewStage1Filter(cfg)
	s := validSession() // duration=10, tools=5, errorRate=0.2
	r := f.Filter(s)
	// Should fail on tool calls (5 < 10)
	if r.Passed {
		t.Fatal("expected reject with custom config")
	}
	if r.ToolCallCountOK {
		t.Fatal("5 tool calls should fail min 10")
	}
}
