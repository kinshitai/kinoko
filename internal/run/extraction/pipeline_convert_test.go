package extraction

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/debug"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// mockStage1Panics is a Stage1 that panics if called — used to verify Stage 1 is skipped.
type mockStage1Panics struct{}

func (m *mockStage1Panics) Filter(_ model.SessionRecord) *model.Stage1Result {
	panic("Stage 1 should not be called during ConvertExtract")
}

// mockStage2WithSourceType records the sourceType it was called with.
type mockStage2WithSourceType struct {
	result     *model.Stage2Result
	err        error
	sourceType string
}

func (m *mockStage2WithSourceType) Score(_ context.Context, _ model.SessionRecord, _ []byte, sourceType string) (*model.Stage2Result, error) {
	m.sourceType = sourceType
	return m.result, m.err
}

// mockStage3WithSourceType records the sourceType it was called with.
type mockStage3WithSourceType struct {
	result     *model.Stage3Result
	err        error
	sourceType string
}

func (m *mockStage3WithSourceType) Evaluate(_ context.Context, _ model.SessionRecord, _ []byte, _ *model.Stage2Result, sourceType string) (*model.Stage3Result, error) {
	m.sourceType = sourceType
	return m.result, m.err
}

func TestConvertExtract_SkipsStage1(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{result: passStage3()}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1Panics{},
		Stage2:    s2,
		Stage3:    s3,
		Committer: stubCommitter{},
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "test-convert-1", LibraryID: "test"}
	// This should NOT panic because Stage 1 is skipped
	result, err := p.ConvertExtract(context.Background(), session, []byte("document content"))
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if result.Status != model.StatusExtracted {
		t.Errorf("expected status extracted, got %s", result.Status)
	}
}

func TestConvertExtract_SourceTypeIsConvert(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{result: passStage3()}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1Panics{},
		Stage2:    s2,
		Stage3:    s3,
		Committer: stubCommitter{},
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "test-convert-2", LibraryID: "test"}
	_, err = p.ConvertExtract(context.Background(), session, []byte("document content"))
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if s2.sourceType != "convert" {
		t.Errorf("Stage2 sourceType = %q, want %q", s2.sourceType, "convert")
	}
	if s3.sourceType != "convert" {
		t.Errorf("Stage3 sourceType = %q, want %q", s3.sourceType, "convert")
	}
}

func TestExtract_SourceTypeIsSession(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{result: passStage3()}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    s2,
		Stage3:    s3,
		Committer: stubCommitter{},
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "test-session-1", LibraryID: "test"}
	_, err = p.Extract(context.Background(), session, []byte("session content"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if s2.sourceType != "session" {
		t.Errorf("Stage2 sourceType = %q, want %q", s2.sourceType, "session")
	}
	if s3.sourceType != "session" {
		t.Errorf("Stage3 sourceType = %q, want %q", s3.sourceType, "session")
	}
}

func TestConvertExtract_Stage2Rejection(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: &model.Stage2Result{
		Passed: false,
		Reason: "rubric scores below minimum",
	}}
	s3 := &mockStage3WithSourceType{result: passStage3()}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1Panics{},
		Stage2:    s2,
		Stage3:    s3,
		Committer: stubCommitter{},
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "test-convert-reject", LibraryID: "test"}
	result, err := p.ConvertExtract(context.Background(), session, []byte("low quality content"))
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if result.Status != model.StatusRejected {
		t.Errorf("expected status rejected, got %s", result.Status)
	}
}
