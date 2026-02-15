package extraction

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// --- Mocks ---

type mockStage1 struct {
	result *Stage1Result
}

func (m *mockStage1) Filter(_ SessionRecord) *Stage1Result { return m.result }

type mockStage2 struct {
	result *Stage2Result
	err    error
}

func (m *mockStage2) Score(_ context.Context, _ SessionRecord, _ []byte) (*Stage2Result, error) {
	return m.result, m.err
}

type mockStage3 struct {
	result *Stage3Result
	err    error
}

func (m *mockStage3) Evaluate(_ context.Context, _ SessionRecord, _ []byte, _ *Stage2Result) (*Stage3Result, error) {
	return m.result, m.err
}

type mockWriter struct {
	err    error
	called bool
	skill  *SkillRecord
	body   []byte
}

func (m *mockWriter) Put(_ context.Context, skill *SkillRecord, body []byte) error {
	m.called = true
	m.skill = skill
	m.body = body
	return m.err
}

type mockReviewer struct {
	called    bool
	sessionID string
	data      []byte
	err       error
}

func (m *mockReviewer) InsertReviewSample(_ context.Context, sessionID string, data []byte) error {
	m.called = true
	m.sessionID = sessionID
	m.data = data
	return m.err
}

// --- Helpers ---

func passStage1() *Stage1Result {
	return &Stage1Result{Passed: true}
}

func failStage1(reason string) *Stage1Result {
	return &Stage1Result{Passed: false, Reason: reason}
}

func passStage2() *Stage2Result {
	return &Stage2Result{
		Passed:             true,
		ClassifiedCategory: CategoryTactical,
		ClassifiedPatterns: []string{"FIX/Backend/DatabaseConnection"},
		RubricScores: QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.6,
		},
	}
}

func failStage2(reason string) *Stage2Result {
	return &Stage2Result{Passed: false, Reason: reason}
}

func passStage3() *Stage3Result {
	return &Stage3Result{
		Passed:        true,
		CriticVerdict: "extract",
		RefinedScores: QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.6,
			CriticConfidence:      0.85,
		},
	}
}

func failStage3() *Stage3Result {
	return &Stage3Result{
		Passed:          false,
		CriticVerdict:   "reject",
		CriticReasoning: "not reusable",
	}
}

func pipelineTestSession() SessionRecord {
	return SessionRecord{
		ID:        "sess-001",
		LibraryID: "lib-1",
	}
}

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func fixedRand(val int) RandIntn {
	return func(n int) int { return val }
}

// --- Tests ---

func TestPipelineExtract(t *testing.T) {
	tests := []struct {
		name           string
		s1             *Stage1Result
		s2             *Stage2Result
		s2Err          error
		s3             *Stage3Result
		s3Err          error
		storeErr       error
		wantStatus     ExtractionStatus
		wantError      bool
		wantSkill      bool
		wantStoreCalled bool
	}{
		{
			name:       "full pass-through",
			s1:         passStage1(),
			s2:         passStage2(),
			s3:         passStage3(),
			wantStatus: StatusExtracted,
			wantSkill:  true,
			wantStoreCalled: true,
		},
		{
			name:       "reject at stage1",
			s1:         failStage1("too short"),
			wantStatus: StatusRejected,
		},
		{
			name:       "reject at stage2",
			s1:         passStage1(),
			s2:         failStage2("novelty too low"),
			wantStatus: StatusRejected,
		},
		{
			name:       "reject at stage3",
			s1:         passStage1(),
			s2:         passStage2(),
			s3:         failStage3(),
			wantStatus: StatusRejected,
		},
		{
			name:       "error at stage2",
			s1:         passStage1(),
			s2Err:      errors.New("embed failed"),
			wantStatus: StatusError,
			wantError:  true,
		},
		{
			name:       "error at stage3",
			s1:         passStage1(),
			s2:         passStage2(),
			s3Err:      errors.New("llm timeout"),
			wantStatus: StatusError,
			wantError:  true,
		},
		{
			name:       "error at store",
			s1:         passStage1(),
			s2:         passStage2(),
			s3:         passStage3(),
			storeErr:   errors.New("disk full"),
			wantStatus: StatusError,
			wantError:  true,
			wantStoreCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockWriter{err: tt.storeErr}
			p := NewPipeline(PipelineConfig{
				Stage1: &mockStage1{result: tt.s1},
				Stage2: &mockStage2{result: tt.s2, err: tt.s2Err},
				Stage3: &mockStage3{result: tt.s3, err: tt.s3Err},
				Writer: w,
				Log:    testLog(),
			})

			result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix database connection pooling issue"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", result.Status, tt.wantStatus)
			}
			if tt.wantError && result.Error == "" {
				t.Error("expected error message in result")
			}
			if !tt.wantError && result.Error != "" {
				t.Errorf("unexpected error in result: %s", result.Error)
			}
			if tt.wantSkill && result.Skill == nil {
				t.Error("expected skill in result")
			}
			if !tt.wantSkill && result.Skill != nil {
				t.Error("unexpected skill in result")
			}
			if tt.wantStoreCalled != w.called {
				t.Errorf("store called = %v, want %v", w.called, tt.wantStoreCalled)
			}
		})
	}
}

func TestPipelineTiming(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Writer: &mockWriter{},
		Log:    testLog(),
	})

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("fix something"))
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", result.DurationMs)
	}
	if result.ProcessedAt.IsZero() {
		t.Error("ProcessedAt is zero")
	}
}

func TestPipelineSampling(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
		randVal    int
		wantSample bool
	}{
		{"sampled at 1%", 0.01, 50, true},        // 50 < 200 (0.01*2*10000)
		{"not sampled at 1%", 0.01, 500, false},   // 500 >= 200
		{"always sampled", 1.0, 9999, true},       // 9999 < 10000
		{"never sampled", 0.0, 0, false},          // rate=0
		{"boundary sampled", 0.01, 199, true},      // 199 < 200 (stratified 2x)
		{"boundary not sampled", 0.01, 200, false}, // 200 >= 200 (stratified 2x)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev := &mockReviewer{}
			p := NewPipeline(PipelineConfig{
				Stage1:     &mockStage1{result: failStage1("test")},
				Stage2:     &mockStage2{},
				Stage3:     &mockStage3{},
				Writer:     &mockWriter{},
				Reviewer:   rev,
				Log:        testLog(),
				SampleRate: tt.sampleRate,
				RandIntn:   fixedRand(tt.randVal),
			})

			_, _ = p.Extract(context.Background(), pipelineTestSession(), []byte("content"))

			if rev.called != tt.wantSample {
				t.Errorf("sampled = %v, want %v", rev.called, tt.wantSample)
			}
		})
	}
}

func TestPipelineSkillNameFromClassification(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Writer: &mockWriter{},
		Log:    testLog(),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("anything"))
	if result.Skill == nil {
		t.Fatal("expected skill")
	}
	// passStage2 classifies as FIX/Backend/DatabaseConnection
	want := "fix-backend-database-connection"
	if result.Skill.Name != want {
		t.Errorf("skill name = %q, want %q", result.Skill.Name, want)
	}
}

func TestSkillNameFromClassification(t *testing.T) {
	tests := []struct {
		name     string
		patterns []string
		category SkillCategory
		want     string
	}{
		{"pattern to kebab", []string{"FIX/Backend/DatabaseConnection"}, CategoryTactical, "fix-backend-database-connection"},
		{"build pattern", []string{"BUILD/Frontend/ComponentDesign"}, CategoryFoundational, "build-frontend-component-design"},
		{"no patterns", nil, CategoryTactical, "tactical-skill"},
		{"empty patterns", []string{}, CategoryContextual, "contextual-skill"},
		{"no category", nil, "", "unnamed-skill"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skillNameFromClassification(tt.patterns, tt.category)
			if got != tt.want {
				t.Errorf("skillNameFromClassification(%v, %q) = %q, want %q", tt.patterns, tt.category, got, tt.want)
			}
		})
	}
}

func TestPipelineSkillFields(t *testing.T) {
	w := &mockWriter{}
	p := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
		Writer:    w,
		Log:       testLog(),
		Extractor: "test-v1",
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("solve caching problem"))
	if result.Skill == nil {
		t.Fatal("expected skill")
	}

	s := result.Skill
	if s.LibraryID != "lib-1" {
		t.Errorf("LibraryID = %q, want %q", s.LibraryID, "lib-1")
	}
	if s.Category != CategoryTactical {
		t.Errorf("Category = %q, want %q", s.Category, CategoryTactical)
	}
	if s.SourceSessionID != "sess-001" {
		t.Errorf("SourceSessionID = %q, want %q", s.SourceSessionID, "sess-001")
	}
	if s.ExtractedBy != "test-v1" {
		t.Errorf("ExtractedBy = %q, want %q", s.ExtractedBy, "test-v1")
	}
	if s.Version != 1 {
		t.Errorf("Version = %d, want 1", s.Version)
	}
	// UUIDv7 check: must be valid UUID
	if len(s.ID) != 36 || s.ID[8] != '-' {
		t.Errorf("ID = %q, want UUIDv7 format", s.ID)
	}
	// Timestamps must be set
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
	if !w.called {
		t.Error("writer not called")
	}
	if len(w.body) == 0 {
		t.Error("empty skill body")
	}
}

func TestPipelineSessionID(t *testing.T) {
	p := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: failStage1("nope")},
		Stage2: &mockStage2{},
		Stage3: &mockStage3{},
		Writer: &mockWriter{},
		Log:    testLog(),
	})

	sess := pipelineTestSession()
	sess.ID = "unique-session-42"
	result, _ := p.Extract(context.Background(), sess, []byte("x"))
	if result.SessionID != "unique-session-42" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "unique-session-42")
	}
}

func TestPipelineStageResults(t *testing.T) {
	// Rejected at stage2 should have stage1 and stage2 results but no stage3.
	p := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: failStage2("too similar")},
		Stage3: &mockStage3{},
		Writer: &mockWriter{},
		Log:    testLog(),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("content"))
	if result.Stage1 == nil {
		t.Error("expected stage1 result")
	}
	if result.Stage2 == nil {
		t.Error("expected stage2 result")
	}
	if result.Stage3 != nil {
		t.Error("unexpected stage3 result")
	}
}

func TestPipelineSamplingOnExtract(t *testing.T) {
	rev := &mockReviewer{}
	p := NewPipeline(PipelineConfig{
		Stage1:     &mockStage1{result: passStage1()},
		Stage2:     &mockStage2{result: passStage2()},
		Stage3:     &mockStage3{result: passStage3()},
		Writer:     &mockWriter{},
		Reviewer:   rev,
		Log:        testLog(),
		SampleRate: 1.0,
		RandIntn:   fixedRand(0),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("fix thing"))
	if !rev.called {
		t.Error("expected sampling on successful extraction")
	}
	if result.Status != StatusExtracted {
		t.Errorf("status = %q, want extracted", result.Status)
	}
}

// Ensure Extract never returns a non-nil error (errors go into result.Error).
func TestPipelineNeverReturnsError(t *testing.T) {
	tests := []struct {
		name  string
		s2Err error
		s3Err error
	}{
		{"stage2 error", errors.New("boom"), nil},
		{"stage3 error", nil, errors.New("boom")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPipeline(PipelineConfig{
				Stage1: &mockStage1{result: passStage1()},
				Stage2: &mockStage2{result: passStage2(), err: tt.s2Err},
				Stage3: &mockStage3{result: passStage3(), err: tt.s3Err},
				Writer: &mockWriter{},
				Log:    testLog(),
			})

			_, err := p.Extract(context.Background(), pipelineTestSession(), []byte("x"))
			if err != nil {
				t.Errorf("Extract returned error: %v", err)
			}
		})
	}
}

func TestBuildSkillMD(t *testing.T) {
	skill := &SkillRecord{
		ID:              "01234567-89ab-7def-8000-000000000001",
		Name:            "fix-backend-database-connection",
		Version:         1,
		Category:        CategoryTactical,
		Patterns:        []string{"FIX/Backend/DatabaseConnection"},
		Quality:         QualityScores{CompositeScore: 3.6, CriticConfidence: 0.85},
		SourceSessionID: "sess-001",
		ExtractedBy:     "test-v1",
		CreatedAt:       time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}

	body := string(buildSkillMD(skill))

	// Check front matter fields
	for _, want := range []string{
		"name: fix-backend-database-connection",
		"id: 01234567-89ab-7def-8000-000000000001",
		"version: 1",
		"category: tactical",
		"extracted_by: test-v1",
		"quality: 3.60",
		"confidence: 0.85",
		"source_session: sess-001",
		"created: 2026-02-15",
		"  - FIX/Backend/DatabaseConnection",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	// Check structured sections
	for _, section := range []string{
		"## When to Use",
		"## Solution",
		"## Why It Works",
		"## Pitfalls",
	} {
		if !strings.Contains(body, section) {
			t.Errorf("body missing section %q", section)
		}
	}
}

// Verify timing is non-negative even for rejected sessions.
func TestPipelineTimingOnReject(t *testing.T) {
	_ = time.Now() // warm up
	p := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: failStage1("nope")},
		Stage2: &mockStage2{},
		Stage3: &mockStage3{},
		Writer: &mockWriter{},
		Log:    testLog(),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("x"))
	if result.DurationMs < 0 {
		t.Errorf("DurationMs = %d on reject, want >= 0", result.DurationMs)
	}
}
