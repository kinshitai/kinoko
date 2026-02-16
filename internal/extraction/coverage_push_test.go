package extraction

import (
	"testing"

	"github.com/kinoko-dev/kinoko/internal/debug"
	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestWriteTraceSummary_NilTrace(t *testing.T) {
	p := &Pipeline{}
	// Should not panic with nil trace
	p.writeTraceSummary(nil, &model.ExtractionResult{}, nil, 0, 0, 0, 0)
}

func TestWriteTraceSummary_WithTrace(t *testing.T) {
	tracer := debug.NewTracer(t.TempDir())
	trace := tracer.StartRun()
	p := &Pipeline{}

	result := &model.ExtractionResult{
		Stage1: &model.Stage1Result{Passed: true},
		Stage2: &model.Stage2Result{Passed: true},
		Stage3: &model.Stage3Result{Passed: true, TokensUsed: 100},
	}
	rejected := "stage1"
	p.writeTraceSummary(trace, result, &rejected, 1, 10, 20, 30)
}
