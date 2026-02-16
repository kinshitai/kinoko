//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/injection"
	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestABTestFlow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	sk := &model.SkillRecord{
		ID: "skill-ab-1", Name: "fix-deploy", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/DevOps/DeploymentFailure"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("fix deployment failure"),
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-deploy/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "DevOps", []string{"FIX/DevOps/DeploymentFailure"}),
	}
	innerInj := injection.New(embedder, store, llm, nil, testLogger())
	abInj := injection.NewABInjector(innerInj, store, injection.ABConfig{
		Enabled: true, ControlRatio: 0.5, MinSampleSize: 10,
	}, testLogger())

	abInj.SetRandFunc(func() float64 { return 0.9 })
	respT, err := abInj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix deployment failure", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-ab-treatment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(respT.Skills) == 0 {
		t.Error("treatment group should receive skills")
	}

	abInj.SetRandFunc(func() float64 { return 0.1 })
	respC, err := abInj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix deployment failure", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-ab-control",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(respC.Skills) != 0 {
		t.Error("control group should NOT receive skills")
	}

	var treatmentEvents, controlEvents int
	store.DB().QueryRow("SELECT COUNT(*) FROM injection_events WHERE session_id = ? AND ab_group = 'treatment' AND delivered = 1", "sess-ab-treatment").Scan(&treatmentEvents)
	store.DB().QueryRow("SELECT COUNT(*) FROM injection_events WHERE session_id = ? AND ab_group = 'control' AND delivered = 0", "sess-ab-control").Scan(&controlEvents)
	if treatmentEvents == 0 {
		t.Error("no treatment injection events logged")
	}
	if controlEvents == 0 {
		t.Error("no control injection events logged")
	}
}

func TestABEventDeduplication(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	sk := &model.SkillRecord{
		ID: "skill-dedup-1", Name: "fix-deploy", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/DevOps/DeploymentFailure"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("fix deployment failure"),
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-deploy/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	llm := &predictableLLM{
		classifyResponse: classifyJSON("FIX", "DevOps", []string{"FIX/DevOps/DeploymentFailure"}),
	}
	innerInj := injection.New(embedder, store, llm, nil, testLogger())
	abInj := injection.NewABInjector(innerInj, store, injection.ABConfig{
		Enabled: true, ControlRatio: 0.5, MinSampleSize: 10,
	}, testLogger())

	abInj.SetRandFunc(func() float64 { return 0.9 })
	_, err := abInj.Inject(ctx, model.InjectionRequest{
		Prompt: "fix deployment failure", LibraryIDs: []string{"test-lib"},
		MaxSkills: 5, SessionID: "sess-dedup-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	var totalEvents int
	store.DB().QueryRow("SELECT COUNT(*) FROM injection_events WHERE session_id = ?", "sess-dedup-1").Scan(&totalEvents)
	if totalEvents != 1 {
		t.Errorf("total events = %d, want exactly 1", totalEvents)
	}

	var ungrouped int
	store.DB().QueryRow("SELECT COUNT(*) FROM injection_events WHERE session_id = ? AND ab_group = ''", "sess-dedup-1").Scan(&ungrouped)
	if ungrouped != 0 {
		t.Errorf("ungrouped events = %d, want 0", ungrouped)
	}
}
