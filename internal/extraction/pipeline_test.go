package extraction

import (
	"github.com/kinoko-dev/kinoko/internal/model"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- Mocks ---

type mockStage1 struct {
	result *model.Stage1Result
}

func (m *mockStage1) Filter(_ model.SessionRecord) *model.Stage1Result { return m.result }

type mockStage2 struct {
	result *model.Stage2Result
	err    error
}

func (m *mockStage2) Score(_ context.Context, _ model.SessionRecord, _ []byte) (*model.Stage2Result, error) {
	return m.result, m.err
}

type mockStage3 struct {
	result *model.Stage3Result
	err    error
}

func (m *mockStage3) Evaluate(_ context.Context, _ model.SessionRecord, _ []byte, _ *model.Stage2Result) (*model.Stage3Result, error) {
	return m.result, m.err
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

func passStage1() *model.Stage1Result {
	return &model.Stage1Result{Passed: true}
}

func failStage1(reason string) *model.Stage1Result {
	return &model.Stage1Result{Passed: false, Reason: reason}
}

func passStage2() *model.Stage2Result {
	return &model.Stage2Result{
		Passed:             true,
		ClassifiedCategory: model.CategoryTactical,
		ClassifiedPatterns: []string{"FIX/Backend/DatabaseConnection"},
		RubricScores: model.QualityScores{
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

func failStage2(reason string) *model.Stage2Result {
	return &model.Stage2Result{Passed: false, Reason: reason}
}

func passStage3() *model.Stage3Result {
	return &model.Stage3Result{
		Passed:        true,
		CriticVerdict: "extract",
		RefinedScores: model.QualityScores{
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

func failStage3() *model.Stage3Result {
	return &model.Stage3Result{
		Passed:          false,
		CriticVerdict:   "reject",
		CriticReasoning: "not reusable",
	}
}

func pipelineTestSession() model.SessionRecord {
	return model.SessionRecord{
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
		s1             *model.Stage1Result
		s2             *model.Stage2Result
		s2Err          error
		s3             *model.Stage3Result
		s3Err          error
		wantStatus model.ExtractionStatus
		wantError  bool
		wantSkill  bool
	}{
		{
			name:       "full pass-through",
			s1:         passStage1(),
			s2:         passStage2(),
			s3:         passStage3(),
			wantStatus: model.StatusExtracted,
			wantSkill:  true,
		},
		{
			name:       "reject at stage1",
			s1:         failStage1("too short"),
			wantStatus: model.StatusRejected,
		},
		{
			name:       "reject at stage2",
			s1:         passStage1(),
			s2:         failStage2("novelty too low"),
			wantStatus: model.StatusRejected,
		},
		{
			name:       "reject at stage3",
			s1:         passStage1(),
			s2:         passStage2(),
			s3:         failStage3(),
			wantStatus: model.StatusRejected,
		},
		{
			name:       "error at stage2",
			s1:         passStage1(),
			s2Err:      errors.New("embed failed"),
			wantStatus: model.StatusError,
			wantError:  true,
		},
		{
			name:       "error at stage3",
			s1:         passStage1(),
			s2:         passStage2(),
			s3Err:      errors.New("llm timeout"),
			wantStatus: model.StatusError,
			wantError:  true,
		},
		// Store errors no longer apply: pipeline writes via git, not SQLite directly.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := NewPipeline(PipelineConfig{
				Stage1: &mockStage1{result: tt.s1},
				Stage2: &mockStage2{result: tt.s2, err: tt.s2Err},
				Stage3: &mockStage3{result: tt.s3, err: tt.s3Err},
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
		})
	}
}

func TestPipelineTiming(t *testing.T) {
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
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
		{"sampled at 1%", 0.01, 50, true},       // 50 < 100 (0.01*10000)
		{"not sampled at 1%", 0.01, 500, false},  // 500 >= 100
		{"always sampled", 1.0, 9999, true},      // 9999 < 10000
		{"never sampled", 0.0, 0, false},         // rate=0
		{"boundary sampled", 0.01, 99, true},     // 99 < 100
		{"boundary not sampled", 0.01, 100, false}, // 100 >= 100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rev := &mockReviewer{}
			p, _ := NewPipeline(PipelineConfig{
				Stage1:     &mockStage1{result: failStage1("test")},
				Stage2:     &mockStage2{},
				Stage3:     &mockStage3{},
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
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
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
		category model.SkillCategory
		want     string
	}{
		{"pattern to kebab", []string{"FIX/Backend/DatabaseConnection"}, model.CategoryTactical, "fix-backend-database-connection"},
		{"build pattern", []string{"BUILD/Frontend/ComponentDesign"}, model.CategoryFoundational, "build-frontend-component-design"},
		{"no patterns", nil, model.CategoryTactical, "tactical-skill"},
		{"empty patterns", []string{}, model.CategoryContextual, "contextual-skill"},
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
	p, _ := NewPipeline(PipelineConfig{
		Stage1:    &mockStage1{result: passStage1()},
		Stage2:    &mockStage2{result: passStage2()},
		Stage3:    &mockStage3{result: passStage3()},
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
	if s.Category != model.CategoryTactical {
		t.Errorf("Category = %q, want %q", s.Category, model.CategoryTactical)
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
	if len(s.ID) != 36 || s.ID[8] != '-' {
		t.Errorf("ID = %q, want UUIDv7 format", s.ID)
	}
	if s.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if s.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
	if s.DecayScore != 1.0 {
		t.Errorf("DecayScore = %f, want 1.0 (new skills must be fully active)", s.DecayScore)
	}
}

func TestPipelineNewSkillDecayScore(t *testing.T) {
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		Stage3: &mockStage3{result: passStage3()},
		Log:    testLog(),
	})

	result, err := p.Extract(context.Background(), pipelineTestSession(), []byte("content"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Skill == nil {
		t.Fatal("expected skill")
	}
	if result.Skill.DecayScore != 1.0 {
		t.Errorf("DecayScore = %f, want 1.0; new skills with 0.0 are invisible to MinDecay filters", result.Skill.DecayScore)
	}
}

func TestPipelineSessionID(t *testing.T) {
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: failStage1("nope")},
		Stage2: &mockStage2{},
		Stage3: &mockStage3{},
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
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: failStage2("too similar")},
		Stage3: &mockStage3{},
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
	p, _ := NewPipeline(PipelineConfig{
		Stage1:     &mockStage1{result: passStage1()},
		Stage2:     &mockStage2{result: passStage2()},
		Stage3:     &mockStage3{result: passStage3()},
		Reviewer:   rev,
		Log:        testLog(),
		SampleRate: 1.0,
		RandIntn:   fixedRand(0),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("fix thing"))
	if !rev.called {
		t.Error("expected sampling on successful extraction")
	}
	if result.Status != model.StatusExtracted {
		t.Errorf("status = %q, want extracted", result.Status)
	}
}

func TestPipelineStratifiedSamplingBalance(t *testing.T) {
	// Simulate 90 rejected, 10 extracted sessions.
	// With stratified sampling, the reviewer should see roughly equal counts.
	rev := &countingReviewer{}
	callCount := 0
	p, _ := NewPipeline(PipelineConfig{
		Stage1:     &mockStage1{result: passStage1()},
		Stage2:     &mockStage2{result: passStage2()},
		Stage3:     &mockStage3{result: passStage3()},
		Reviewer:   rev,
		Log:        testLog(),
		SampleRate: 0.5, // high rate so we actually get samples
		RandIntn:   func(n int) int { callCount++; return callCount % n },
	})

	sess := pipelineTestSession()

	// Interleave: 1 extracted per 9 rejected (10% base rate, like reality).
	extIdx := 0
	rejIdx := 0
	for i := 0; i < 100; i++ {
		if i%10 == 0 && extIdx < 10 {
			sess.ID = fmt.Sprintf("ext-%d", extIdx)
			p.stage1 = &mockStage1{result: passStage1()}
			p.stage3 = &mockStage3{result: passStage3()}
			extIdx++
		} else {
			sess.ID = fmt.Sprintf("rej-%d", rejIdx)
			p.stage1 = &mockStage1{result: failStage1("nope")}
			rejIdx++
		}
		p.Extract(context.Background(), sess, []byte("content"))
	}

	// Extracted samples should be >= rejected samples (since extracted pool
	// is always underrepresented, all extracted get sampled).
	ext := p.extractedSamples.Load()
	rej := p.rejectedSamples.Load()
	if ext == 0 {
		t.Error("no extracted samples collected")
	}
	if rej == 0 {
		t.Error("no rejected samples collected")
	}
	// The ratio should be much closer to 50/50 than the input 10/90.
	total := ext + rej
	extractedPct := float64(ext) / float64(total)
	if extractedPct < 0.2 {
		t.Errorf("extracted = %d/%d (%.0f%%), want >= 20%% (stratified)", ext, total, extractedPct*100)
	}
}

type countingReviewer struct {
	count atomic.Int64
}

func (r *countingReviewer) InsertReviewSample(_ context.Context, _ string, _ []byte) error {
	r.count.Add(1)
	return nil
}

func TestBuildSkillMDContent(t *testing.T) {
	skill := &model.SkillRecord{
		ID:              "test-id",
		Name:            "fix-database",
		Version:         1,
		Category:        model.CategoryTactical,
		Patterns:        []string{"FIX/Backend/DatabaseConnection"},
		Quality:         model.QualityScores{CompositeScore: 3.6, CriticConfidence: 0.85},
		SourceSessionID: "sess-001",
		ExtractedBy:     "test-v1",
		CreatedAt:       time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}

	body := string(buildSkillMD(skill, &model.Stage3Result{
		CriticReasoning:          "Effective connection pool recovery pattern",
		ContradictsBestPractices: true,
	}, []byte("session content here")))

	// Body should contain real content, not just placeholders.
	if strings.Contains(body, "<!-- ") {
		t.Error("body still contains HTML comment placeholders")
	}
	if !strings.Contains(body, "Effective connection pool recovery pattern") {
		t.Error("body missing critic reasoning in Why It Works")
	}
	if !strings.Contains(body, "session content here") {
		t.Error("body missing session content in Solution")
	}
	if !strings.Contains(body, "contradict established best practices") {
		t.Error("body missing contradiction warning in Pitfalls")
	}
	if !strings.Contains(body, "FIX/Backend/DatabaseConnection") {
		t.Error("body missing pattern in When to Use")
	}
}

func TestBuildSkillMDNilStage3(t *testing.T) {
	skill := &model.SkillRecord{
		ID:        "test-id",
		Name:      "test-skill",
		Version:   1,
		Category:  model.CategoryTactical,
		CreatedAt: time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}
	// Should not panic with nil stage3.
	body := buildSkillMD(skill, nil, []byte("content"))
	if len(body) == 0 {
		t.Error("empty body")
	}
}

func TestNewPipelineNilDeps(t *testing.T) {
	_, err := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: passStage1()},
		Stage2: &mockStage2{result: passStage2()},
		// Stage3 missing
		Log:    testLog(),
	})
	if err == nil {
		t.Error("expected error for nil Stage3")
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
			p, _ := NewPipeline(PipelineConfig{
				Stage1: &mockStage1{result: passStage1()},
				Stage2: &mockStage2{result: passStage2(), err: tt.s2Err},
				Stage3: &mockStage3{result: passStage3(), err: tt.s3Err},
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
	skill := &model.SkillRecord{
		ID:              "01234567-89ab-7def-8000-000000000001",
		Name:            "fix-backend-database-connection",
		Version:         1,
		Category:        model.CategoryTactical,
		Patterns:        []string{"FIX/Backend/DatabaseConnection"},
		Quality:         model.QualityScores{CompositeScore: 3.6, CriticConfidence: 0.85},
		SourceSessionID: "sess-001",
		ExtractedBy:     "test-v1",
		CreatedAt:       time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC),
	}

	body := string(buildSkillMD(skill, &model.Stage3Result{
		CriticReasoning:          "The solution demonstrates a clean pattern for database connection pooling recovery.",
		ContradictsBestPractices: false,
	}, []byte("fix database connection pooling issue by implementing retry logic")))

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
	p, _ := NewPipeline(PipelineConfig{
		Stage1: &mockStage1{result: failStage1("nope")},
		Stage2: &mockStage2{},
		Stage3: &mockStage3{},
		Log:    testLog(),
	})

	result, _ := p.Extract(context.Background(), pipelineTestSession(), []byte("x"))
	if result.DurationMs < 0 {
		t.Errorf("DurationMs = %d on reject, want >= 0", result.DurationMs)
	}
}
