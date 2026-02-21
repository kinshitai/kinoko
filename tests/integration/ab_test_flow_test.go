//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/run/injection"
	"github.com/kinoko-dev/kinoko/pkg/model"
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
	abInj := injection.NewABInjector(innerInj, nil, injection.ABConfig{
		Enabled: true, ControlRatio: 0.5, MinSampleSize: 10,
	}, testLogger())

	// Treatment group (rand > controlRatio) — should receive skills.
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

	// Control group (rand < controlRatio) — should NOT receive skills.
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
}
