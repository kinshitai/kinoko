//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/injection"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestFullInjectionFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	skill1 := &model.SkillRecord{
		ID: "skill-inj-1", Name: "fix-db-conn", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.85,
		},
		Embedding:  embedder.deterministicVector("fix database connection"),
		DecayScore: 1.0, SuccessCorrelation: 0.5,
		ExtractedBy: "test", FilePath: "skills/fix-db-conn/SKILL.md",
	}
	skill2 := &model.SkillRecord{
		ID: "skill-inj-2", Name: "build-api", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryFoundational,
		Patterns: []string{"BUILD/Backend/APIDesign"},
		Quality: model.QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 4,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3,
			InnovationLevel: 2, CompositeScore: 3.0, CriticConfidence: 0.7,
		},
		Embedding:  embedder.deterministicVector("build REST API design"),
		DecayScore: 0.8, SuccessCorrelation: 0.3,
		ExtractedBy: "test", FilePath: "skills/build-api/SKILL.md",
	}

	if err := store.Put(ctx, skill1, nil); err != nil {
		t.Fatalf("put skill1: %v", err)
	}
	if err := store.Put(ctx, skill2, nil); err != nil {
		t.Fatalf("put skill2: %v", err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix database connection pooling issue", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-inj-001",
	})
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	if len(resp.Skills) == 0 {
		t.Fatal("expected at least one skill injected")
	}
	if len(resp.Skills) != 2 {
		t.Errorf("got %d skills, want 2", len(resp.Skills))
	}
	if resp.Skills[0].SkillID != "skill-inj-1" {
		t.Errorf("top skill = %q, want skill-inj-1", resp.Skills[0].SkillID)
	}

}

func TestDeadSkillFiltering(t *testing.T) {
	t.Skip("TODO(#89): decay filtering is now client-side — server no longer has decay_score column")
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	alive := &model.SkillRecord{
		ID: "alive", Name: "alive-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("alive skill"),
		DecayScore: 0.8, ExtractedBy: "test", FilePath: "skills/alive/SKILL.md",
	}
	dead := &model.SkillRecord{
		ID: "dead", Name: "dead-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("dead skill"),
		DecayScore: 0.0, ExtractedBy: "test", FilePath: "skills/dead/SKILL.md",
	}

	store.Put(ctx, alive, nil)
	store.Put(ctx, dead, nil)

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix db", LibraryIDs: []string{"test-lib"},
		MaxSkills: 10, SessionID: "sess-dead-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range resp.Skills {
		if s.SkillID == "dead" {
			t.Error("dead skill (decay=0) should not appear in injection results")
		}
	}
	if len(resp.Skills) == 0 {
		t.Error("alive skill should still appear")
	}
}

func TestEmbeddingCosineSimilarityE2E(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	sk := &model.SkillRecord{
		ID: "skill-cos-1", Name: "fix-db-conn", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("fix database connection"),
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-db/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"}),
	}
	inj := injection.New(embedder, store, llm, nil, testLogger())

	resp, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix database connection", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-cos-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) == 0 {
		t.Fatal("expected injected skills")
	}
	if resp.Skills[0].CosineSim < 0.99 {
		t.Errorf("cosine sim = %f, want ~1.0", resp.Skills[0].CosineSim)
	}

	resp2, err := inj.Inject(ctx, model.InjectionRequest{
		Prompt: "build REST API design patterns", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-cos-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp2.Skills) == 0 {
		t.Fatal("expected skills even for different prompt")
	}
	if resp2.Skills[0].CosineSim >= 0.99 {
		t.Errorf("cosine sim = %f, should be < 0.99 for different embedding", resp2.Skills[0].CosineSim)
	}
}
