package extraction

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/debug"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// --- DryRun tests ---

func TestConvertExtract_DryRunSkipsCommit(t *testing.T) {
	committer := &recordingCommitter{}
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{result: passStage3()}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1Panics{},
		Stage2:    s2,
		Stage3:    s3,
		Committer: committer,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "dry-run-convert", LibraryID: "test"}
	result, err := p.ConvertExtract(context.Background(), session, []byte("document content"), "")
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if result.Status != model.StatusExtracted {
		t.Errorf("expected status extracted, got %s", result.Status)
	}
	if committer.called {
		t.Error("committer should NOT be called in dry-run mode")
	}
	if result.CommitHash != "" {
		t.Errorf("expected empty commit hash in dry-run, got %q", result.CommitHash)
	}
	if result.Skill == nil {
		t.Error("expected skill record even in dry-run mode")
	}
}

func TestExtract_DryRunSkipsCommit(t *testing.T) {
	committer := &recordingCommitter{}

	p, err := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Committer: committer,
		Log:       slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Tracer:    debug.NewTracer(t.TempDir()),
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("NewPipeline: %v", err)
	}

	session := model.SessionRecord{ID: "dry-run-session", LibraryID: "test"}
	result, err := p.Extract(context.Background(), session, []byte("session content"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if result.Status != model.StatusExtracted {
		t.Errorf("expected status extracted, got %s", result.Status)
	}
	if committer.called {
		t.Error("committer should NOT be called in dry-run mode")
	}
	if result.CommitHash != "" {
		t.Errorf("expected empty commit hash in dry-run, got %q", result.CommitHash)
	}
}

// --- ConvertExtract Stage3 rejection & error ---

func TestConvertExtract_Stage3Rejection(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{result: failStage3()}

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

	session := model.SessionRecord{ID: "convert-s3-reject", LibraryID: "test"}
	result, err := p.ConvertExtract(context.Background(), session, []byte("mediocre content"), "")
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if result.Status != model.StatusRejected {
		t.Errorf("expected status rejected, got %s", result.Status)
	}
	if result.Stage3 == nil {
		t.Error("expected stage3 result on stage3 rejection")
	}
}

func TestConvertExtract_Stage2Error(t *testing.T) {
	s2 := &mockStage2WithSourceType{err: errors.New("llm timeout")}
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

	session := model.SessionRecord{ID: "convert-s2-error", LibraryID: "test"}
	result, err := p.ConvertExtract(context.Background(), session, []byte("content"), "")
	if err != nil {
		t.Fatalf("ConvertExtract should not return error: %v", err)
	}

	if result.Status != model.StatusError {
		t.Errorf("expected status error, got %s", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message in result")
	}
}

func TestConvertExtract_Stage3Error(t *testing.T) {
	s2 := &mockStage2WithSourceType{result: passStage2()}
	s3 := &mockStage3WithSourceType{err: errors.New("model overloaded")}

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

	session := model.SessionRecord{ID: "convert-s3-error", LibraryID: "test"}
	result, err := p.ConvertExtract(context.Background(), session, []byte("content"), "")
	if err != nil {
		t.Fatalf("ConvertExtract should not return error: %v", err)
	}

	if result.Status != model.StatusError {
		t.Errorf("expected status error, got %s", result.Status)
	}
	if !strings.Contains(result.Error, "model overloaded") {
		t.Errorf("expected error to contain original message, got %q", result.Error)
	}
}

// --- Taxonomy hint tests ---

// mockStage2WithHint records the taxonomyHint it was called with.
type mockStage2WithHint struct {
	result       *model.Stage2Result
	err          error
	sourceType   SourceType
	taxonomyHint string
}

func (m *mockStage2WithHint) Score(_ context.Context, _ model.SessionRecord, _ []byte, sourceType SourceType, taxonomyHint string) (*model.Stage2Result, error) {
	m.sourceType = sourceType
	m.taxonomyHint = taxonomyHint
	return m.result, m.err
}

// mockStage3WithHint records the taxonomyHint it was called with.
type mockStage3WithHint struct {
	result       *model.Stage3Result
	err          error
	sourceType   SourceType
	taxonomyHint string
}

func (m *mockStage3WithHint) Evaluate(_ context.Context, _ model.SessionRecord, _ []byte, _ *model.Stage2Result, sourceType SourceType, taxonomyHint string) (*model.Stage3Result, error) {
	m.sourceType = sourceType
	m.taxonomyHint = taxonomyHint
	return m.result, m.err
}

func TestConvertExtract_TaxonomyHintPropagated(t *testing.T) {
	s2 := &mockStage2WithHint{result: passStage2()}
	s3 := &mockStage3WithHint{result: passStage3()}

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

	hint := "BUILD/Backend/APIDesign"
	session := model.SessionRecord{ID: "convert-taxonomy", LibraryID: "test"}
	_, err = p.ConvertExtract(context.Background(), session, []byte("document about API design"), hint)
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if s2.taxonomyHint != hint {
		t.Errorf("Stage2 taxonomyHint = %q, want %q", s2.taxonomyHint, hint)
	}
	if s3.taxonomyHint != hint {
		t.Errorf("Stage3 taxonomyHint = %q, want %q", s3.taxonomyHint, hint)
	}
}

func TestConvertExtract_EmptyTaxonomyHint(t *testing.T) {
	s2 := &mockStage2WithHint{result: passStage2()}
	s3 := &mockStage3WithHint{result: passStage3()}

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

	session := model.SessionRecord{ID: "convert-no-hint", LibraryID: "test"}
	_, err = p.ConvertExtract(context.Background(), session, []byte("generic document"), "")
	if err != nil {
		t.Fatalf("ConvertExtract: %v", err)
	}

	if s2.taxonomyHint != "" {
		t.Errorf("Stage2 taxonomyHint = %q, want empty", s2.taxonomyHint)
	}
	if s3.taxonomyHint != "" {
		t.Errorf("Stage3 taxonomyHint = %q, want empty", s3.taxonomyHint)
	}
}

func TestBuildRubricPrompt_TaxonomyHintIncluded(t *testing.T) {
	content := []byte("document about caching strategies")
	hint := "OPTIMIZE/Performance/Caching"

	prompt := buildRubricPrompt(content, SourceTypeConvert, hint)

	if !strings.Contains(prompt, "Suggested taxonomy pattern: OPTIMIZE/Performance/Caching") {
		t.Error("prompt should contain taxonomy hint")
	}
	if !strings.Contains(prompt, "Use this as a hint but override") {
		t.Error("prompt should contain hint override instruction")
	}
}

func TestBuildRubricPrompt_NoTaxonomyHintWhenEmpty(t *testing.T) {
	content := []byte("document about something")

	prompt := buildRubricPrompt(content, SourceTypeConvert, "")

	if strings.Contains(prompt, "Suggested taxonomy pattern") {
		t.Error("prompt should NOT contain taxonomy hint when empty")
	}
}

func TestBuildRubricPrompt_TaxonomyHintInSessionMode(t *testing.T) {
	// Taxonomy hint should work in session mode too (even if not typical).
	content := []byte("session about query optimization")
	hint := "OPTIMIZE/Backend/QueryOptimization"

	prompt := buildRubricPrompt(content, SourceTypeSession, hint)

	if !strings.Contains(prompt, "Suggested taxonomy pattern: OPTIMIZE/Backend/QueryOptimization") {
		t.Error("taxonomy hint should be included even in session mode")
	}
}

// --- SourceType typed constant tests ---

func TestSourceTypeConstants(t *testing.T) {
	// Verify SourceType is a typed string, not a raw string — ensures no
	// accidental comparison with untyped string literals.
	var st SourceType
	st = SourceTypeSession
	if st != "session" {
		t.Errorf("SourceTypeSession = %q, want %q", st, "session")
	}
	st = SourceTypeConvert
	if st != "convert" {
		t.Errorf("SourceTypeConvert = %q, want %q", st, "convert")
	}
}

// --- Helper ---

// recordingCommitter tracks whether CommitSkill was called.
type recordingCommitter struct {
	called bool
}

func (c *recordingCommitter) CommitSkill(_ context.Context, _ string, _ *model.SkillRecord, _ []byte) (string, error) {
	c.called = true
	return "abc123", nil
}
