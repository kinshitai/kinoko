//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/injection"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func TestConcurrentExtractionAndInjection(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewSQLiteStore(tmpDir+"/concurrent-ei.db", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	sk := &model.SkillRecord{
		ID: "pre-seed-1", Name: "pre-seeded", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("pre-seeded skill"),
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/pre-seeded/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	const n = 5
	errs := make(chan error, n*2)

	for i := 0; i < n; i++ {
		go func(idx int) {
			emb := newPredictableEmbedder(3)
			llm := &predictableLLM{rubricResponse: goodRubricJSON(), criticResponse: extractVerdictJSON()}
			s1 := extraction.NewStage1Filter(defaultExtractionConfig(), testLogger())
			s2 := extraction.NewStage2Scorer(emb, &mockQuerier{sim: 0.5}, llm, defaultExtractionConfig(), testLogger())
			s3 := extraction.NewStage3Critic(llm, defaultExtractionConfig(), testLogger())
			p, _ := extraction.NewPipeline(extraction.PipelineConfig{
				Stage1: s1, Stage2: s2, Stage3: s3, Writer: store, Committer: noopCommitter{}, Log: testLogger(),
			})
			sess := goodSession(fmt.Sprintf("sess-cei-%d", idx), "test-lib")
			_, err := p.Extract(ctx, sess, []byte(fmt.Sprintf("unique extraction problem %d", idx)))
			errs <- err
		}(i)
	}

	for i := 0; i < n; i++ {
		go func(idx int) {
			emb := newPredictableEmbedder(3)
			llm := &predictableLLM{classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"})}
			inj := injection.New(emb, store, llm, store, testLogger())
			_, err := inj.Inject(ctx, model.InjectionRequest{
				Prompt: "fix database issue", LibraryIDs: []string{"test-lib"},
				MaxSkills: 3, SessionID: fmt.Sprintf("sess-cinj-%d", idx),
			})
			errs <- err
		}(i)
	}

	var failures int
	for i := 0; i < n*2; i++ {
		if err := <-errs; err != nil {
			t.Logf("concurrent op %d: %v", i, err)
			failures++
		}
	}

	var integrity string
	store.DB().QueryRow("PRAGMA integrity_check").Scan(&integrity)
	if integrity != "ok" {
		t.Fatalf("DB integrity: %s", integrity)
	}
}
