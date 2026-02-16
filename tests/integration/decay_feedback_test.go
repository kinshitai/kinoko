//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func TestFeedbackLoop(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	skill := &model.SkillRecord{
		ID: "skill-fb-1", Name: "fix-auth-flow", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/AuthFlow"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		Embedding:  embedder.deterministicVector("fix auth flow"),
		DecayScore: 1.0, SuccessCorrelation: 0.0,
		ExtractedBy: "test", FilePath: "skills/fix-auth/SKILL.md",
	}
	if err := store.Put(ctx, skill, nil); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for i, outcome := range []string{"success", "success", "failure", "success"} {
		ev := storage.InjectionEventRecord{
			ID: fmt.Sprintf("ev-fb-%d", i), SessionID: fmt.Sprintf("sess-fb-%d", i),
			SkillID: "skill-fb-1", RankPosition: 1, MatchScore: 0.8, InjectedAt: now,
		}
		if err := store.WriteInjectionEvent(ctx, ev); err != nil {
			t.Fatalf("write event %d: %v", i, err)
		}
		store.DB().Exec("UPDATE injection_events SET session_outcome = ? WHERE id = ?", outcome, ev.ID)
	}

	if err := store.UpdateUsage(ctx, "skill-fb-1", "success"); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(ctx, "skill-fb-1")
	if err != nil {
		t.Fatal(err)
	}
	assertApprox(t, got.SuccessCorrelation, 0.5, 0.01, "success_correlation")
}

func TestDecayCycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	skills := []struct {
		id       string
		name     string
		category model.SkillCategory
		age      int
	}{
		{"skill-d1", "foundational-skill", model.CategoryFoundational, 365},
		{"skill-d2", "tactical-skill", model.CategoryTactical, 90},
		{"skill-d3", "contextual-skill", model.CategoryContextual, 180},
		{"skill-d4", "fresh-tactical", model.CategoryTactical, 1},
		{"skill-d5", "nearly-dead", model.CategoryTactical, 900},
	}

	for _, s := range skills {
		lastInjected := now.AddDate(0, 0, -s.age)
		sk := &model.SkillRecord{
			ID: s.id, Name: s.name, Version: 1, LibraryID: "test-lib",
			Category: s.category, Patterns: []string{"FIX/Backend/DatabaseConnection"},
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
			},
			DecayScore: 1.0, ExtractedBy: "test",
			FilePath: "skills/" + s.name + "/SKILL.md", LastInjectedAt: lastInjected,
		}
		if err := store.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %s: %v", s.id, err)
		}
		store.DB().Exec("UPDATE skills SET last_injected_at = ? WHERE id = ?", lastInjected, s.id)
	}

	cfg := decay.DefaultConfig()
	runner, err := decay.NewRunner(store, store, cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	runner.SetNow(func() time.Time { return now })

	result, err := runner.RunCycle(ctx, "test-lib")
	if err != nil {
		t.Fatal(err)
	}
	if result.Processed != 5 {
		t.Errorf("processed = %d, want 5", result.Processed)
	}

	for _, tc := range []struct {
		id  string
		w   float64
		eps float64
	}{
		{"skill-d1", 0.5, 0.01},
		{"skill-d2", 0.5, 0.01},
		{"skill-d3", 0.5, 0.01},
		{"skill-d4", 0.992, 0.01},
		{"skill-d5", 0.0, 0.001},
	} {
		sk, err := store.Get(ctx, tc.id)
		if err != nil {
			t.Fatalf("get %s: %v", tc.id, err)
		}
		assertApprox(t, sk.DecayScore, tc.w, tc.eps, tc.id+" decay")
	}
	if result.Deprecated < 1 {
		t.Errorf("deprecated = %d, want >= 1", result.Deprecated)
	}
}

func TestDecayRescue(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	sk := &model.SkillRecord{
		ID: "skill-rescue", Name: "rescue-me", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore: 0.4, SuccessCorrelation: 0.8,
		ExtractedBy: "test", FilePath: "skills/rescue-me/SKILL.md",
	}
	if err := store.Put(ctx, sk, nil); err != nil {
		t.Fatal(err)
	}

	lastInjected := now.AddDate(0, 0, -5)
	store.DB().Exec("UPDATE skills SET last_injected_at = ?, success_correlation = 0.8 WHERE id = ?", lastInjected, "skill-rescue")

	cfg := decay.DefaultConfig()
	runner, err := decay.NewRunner(store, store, cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	runner.SetNow(func() time.Time { return now })

	result, err := runner.RunCycle(ctx, "test-lib")
	if err != nil {
		t.Fatal(err)
	}
	if result.Rescued != 1 {
		t.Errorf("rescued = %d, want 1", result.Rescued)
	}

	got, _ := store.Get(ctx, "skill-rescue")
	if got.DecayScore < 0.6 {
		t.Errorf("rescued decay = %.4f, want > 0.6", got.DecayScore)
	}
}
