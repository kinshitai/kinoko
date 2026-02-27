//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/injection"
	"github.com/kinoko-dev/kinoko/internal/serve/storage"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestFullExtractionFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	embedder := newPredictableEmbedder(3)
	llm := &predictableLLM{
		rubricResponse: goodRubricJSON(),
		criticResponse: extractVerdictJSON(),
	}
	reviewer := &mockReviewWriter{}

	s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
	s2 := extraction.NewStage2Scorer(llm, defaultExtractionConfig(), testLogger())
	s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

	indexer := storage.NewSQLiteIndexer(store)
	committer := &indexingCommitter{indexer: indexer, embedder: embedder}

	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:    s1,
		Stage2:    s2,
		Stage3:    s3,
		Committer: committer,
		Reviewer:  reviewer,

		Log:        testLogger(),
		SampleRate: 0,
		Extractor:  "integration-test-v1",
	})
	if err != nil {
		t.Fatalf("new pipeline: %v", err)
	}

	session := goodSession("sess-e2e-001", "test-lib")
	content := []byte("User asked to fix database connection pooling. Agent diagnosed stale connections, implemented retry logic with exponential backoff, verified fix with load test.")

	result, err := pipeline.Extract(ctx, session, content)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if result.Status != model.StatusExtracted {
		t.Fatalf("status = %q, want extracted (error: %s)", result.Status, result.Error)
	}
	if result.Skill == nil {
		t.Fatal("expected skill in result")
	}
	if result.Stage1 == nil || !result.Stage1.Passed {
		t.Error("stage1 should have passed")
	}
	if result.Stage2 == nil || !result.Stage2.Passed {
		t.Error("stage2 should have passed")
	}
	if result.Stage3 == nil || !result.Stage3.Passed {
		t.Error("stage3 should have passed")
	}

	skillID := result.Skill.ID
	dbSkill, err := store.Get(ctx, skillID)
	if err != nil {
		t.Fatalf("get skill from db: %v", err)
	}

	if dbSkill.Name != result.Skill.Name {
		t.Errorf("db name = %q, result name = %q", dbSkill.Name, result.Skill.Name)
	}
	if dbSkill.LibraryID != "test-lib" {
		t.Errorf("library = %q, want test-lib", dbSkill.LibraryID)
	}
	if dbSkill.ExtractedBy != "integration-test-v1" {
		t.Errorf("extracted_by = %q", dbSkill.ExtractedBy)
	}
	if dbSkill.Category != model.CategoryTactical {
		t.Errorf("category = %q, want tactical", dbSkill.Category)
	}
	if dbSkill.Quality.ProblemSpecificity != 4 {
		t.Errorf("problem_specificity = %d, want 4", dbSkill.Quality.ProblemSpecificity)
	}
	if len(dbSkill.Patterns) == 0 {
		t.Error("expected patterns in DB")
	}
	if len(dbSkill.Embedding) != 3 {
		t.Errorf("embedding dims = %d, want 3", len(dbSkill.Embedding))
	}
	if result.DurationMs < 0 {
		t.Errorf("duration = %d, want >= 0", result.DurationMs)
	}
}

func TestRejectionFlows(t *testing.T) {
	t.Run("rejected at stage1", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
		llm := &predictableLLM{rubricResponse: goodRubricJSON(), criticResponse: extractVerdictJSON()}
		s2 := extraction.NewStage2Scorer(llm, defaultExtractionConfig(), testLogger())
		s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

		pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
			Stage1: s1, Stage2: s2, Stage3: s3, Committer: noopCommitter{}, Log: testLogger(),
		})

		session := shortSession("sess-rej1", "test-lib")
		result, err := pipeline.Extract(ctx, session, []byte("trivial"))
		if err != nil {
			t.Fatal(err)
		}

		if result.Status != model.StatusRejected {
			t.Fatalf("status = %q, want rejected", result.Status)
		}
		if result.Stage1 == nil || result.Stage1.Passed {
			t.Error("stage1 should have failed")
		}
		if result.Stage2 != nil {
			t.Error("stage2 should not have been called")
		}

		results, err := store.Query(ctx, storage.SkillQuery{LibraryIDs: []string{"test-lib"}, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 skills in DB, got %d", len(results))
		}
	})

	t.Run("rejected at stage3", func(t *testing.T) {
		store := newTestStore(t)
		ctx := context.Background()

		llm := &predictableLLM{
			rubricResponse: goodRubricJSON(),
			criticResponse: rejectVerdictJSON(),
		}

		s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
		s2 := extraction.NewStage2Scorer(llm, defaultExtractionConfig(), testLogger())
		s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

		pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
			Stage1: s1, Stage2: s2, Stage3: s3, Committer: noopCommitter{}, Log: testLogger(),
		})

		session := goodSession("sess-rej3", "test-lib")
		result, err := pipeline.Extract(ctx, session, []byte("This session fixed a complex database connection pooling issue."))
		if err != nil {
			t.Fatal(err)
		}

		if result.Status != model.StatusRejected {
			t.Fatalf("status = %q, want rejected", result.Status)
		}
		if result.Stage3 == nil || result.Stage3.Passed {
			t.Error("stage3 should have rejected")
		}
		if result.Skill != nil {
			t.Error("no skill should exist after stage3 rejection")
		}

		results, _ := store.Query(ctx, storage.SkillQuery{LibraryIDs: []string{"test-lib"}, Limit: 10})
		if len(results) != 0 {
			t.Errorf("expected 0 skills, got %d", len(results))
		}
	})
}

func TestInjectionDegradedModeNoEmbedder(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	sk := &model.SkillRecord{
		ID: "skill-fallback", Name: "fix-db", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		ExtractedBy: "test",
		FilePath:    "skills/fix-db/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	classifyLLM := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(nil, store, classifyLLM, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt:     "fix db connection",
		LibraryIDs: []string{"test-lib"},
		MaxSkills:  3,
	})
	if err != nil {
		t.Fatalf("injection should work in degraded mode: %v", err)
	}
	if len(resp.Skills) == 0 {
		t.Error("pattern-only injection should still return matching skills")
	}
}

func TestConcurrentExtractions(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(tmpDir+"/concurrent.db", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	const n = 10
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		go func(idx int) {
			embedder := newPredictableEmbedder(3)
			llm := &predictableLLM{
				rubricResponse: goodRubricJSON(),
				criticResponse: extractVerdictJSON(),
			}

			s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
			s2 := extraction.NewStage2Scorer(llm, defaultExtractionConfig(), testLogger())
			s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())

			indexer := storage.NewSQLiteIndexer(store)
			committer := &indexingCommitter{indexer: indexer, embedder: embedder}
			pipeline, _ := extraction.NewPipeline(extraction.PipelineConfig{
				Stage1: s1, Stage2: s2, Stage3: s3, Committer: committer, Log: testLogger(),
			})

			sess := goodSession(fmt.Sprintf("sess-conc-%d", idx), "test-lib")
			content := []byte(fmt.Sprintf("unique problem %d requiring specialized solution", idx))
			result, err := pipeline.Extract(ctx, sess, content)
			if err != nil {
				errs <- err
				return
			}
			if result.Status == model.StatusError {
				errs <- fmt.Errorf("session %d: %s", idx, result.Error)
				return
			}
			errs <- nil
		}(i)
	}

	var failures int
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Logf("concurrent extraction %d: %v", i, err)
			failures++
		}
	}

	results, err := store.Query(ctx, storage.SkillQuery{LibraryIDs: []string{"test-lib"}, Limit: 100})
	if err != nil {
		t.Fatalf("query after concurrent: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one skill from concurrent extraction")
	}

	var integrity string
	store.DB().QueryRow("PRAGMA integrity_check").Scan(&integrity)
	if integrity != "ok" {
		t.Errorf("DB integrity: %s", integrity)
	}
}
