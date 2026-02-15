package decay

import (
	"context"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/extraction"
)

// --- mocks ---

type mockReader struct {
	skills []extraction.SkillRecord
	err    error
}

func (m *mockReader) ListByDecay(_ context.Context, _ string, _ int) ([]extraction.SkillRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return a copy to avoid mutation issues.
	out := make([]extraction.SkillRecord, len(m.skills))
	copy(out, m.skills)
	return out, nil
}

type decayUpdate struct {
	id    string
	score float64
}

type mockWriter struct {
	updates []decayUpdate
	err     error
}

func (m *mockWriter) UpdateDecay(_ context.Context, id string, score float64) error {
	if m.err != nil {
		return m.err
	}
	m.updates = append(m.updates, decayUpdate{id, score})
	return nil
}

// --- helpers ---

func fixedNow() time.Time {
	return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
}

func skill(id string, cat extraction.SkillCategory, decay float64, updatedAt time.Time) extraction.SkillRecord {
	return extraction.SkillRecord{
		ID:         id,
		Category:   cat,
		DecayScore: decay,
		UpdatedAt:  updatedAt,
	}
}

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

// --- tests ---

func TestHalfLifeFormula(t *testing.T) {
	// After exactly one half-life, decay should be halved.
	tests := []struct {
		name     string
		category extraction.SkillCategory
		halfLife int
	}{
		{"foundational", extraction.CategoryFoundational, 365},
		{"tactical", extraction.CategoryTactical, 90},
		{"contextual", extraction.CategoryContextual, 180},
	}

	now := fixedNow()
	cfg := DefaultConfig()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedAt := now.AddDate(0, 0, -tt.halfLife)
			w := &mockWriter{}
			r := NewRunner(
				&mockReader{skills: []extraction.SkillRecord{skill("s1", tt.category, 1.0, updatedAt)}},
				w, cfg, slog.Default(),
			)
			r.now = func() time.Time { return now }

			res, err := r.RunCycle(context.Background(), "lib1")
			if err != nil {
				t.Fatal(err)
			}
			if res.Processed != 1 {
				t.Fatalf("processed=%d, want 1", res.Processed)
			}
			if len(w.updates) != 1 {
				t.Fatalf("updates=%d, want 1", len(w.updates))
			}
			if !almostEqual(w.updates[0].score, 0.5, 0.001) {
				t.Errorf("score=%.4f, want ~0.5", w.updates[0].score)
			}
		})
	}
}

func TestRescueLogic(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	tests := []struct {
		name           string
		lastInjected   time.Time
		successCorr    float64
		wantRescued    int
		wantScoreAbove float64
	}{
		{
			name:           "recent_success_rescues",
			lastInjected:   now.AddDate(0, 0, -5),
			successCorr:    0.8,
			wantRescued:    1,
			wantScoreAbove: 0.5, // decayed + 0.3 rescue boost
		},
		{
			name:           "old_injection_no_rescue",
			lastInjected:   now.AddDate(0, 0, -60),
			successCorr:    0.8,
			wantRescued:    0,
			wantScoreAbove: 0.0,
		},
		{
			name:           "negative_correlation_no_rescue",
			lastInjected:   now.AddDate(0, 0, -5),
			successCorr:    -0.2,
			wantRescued:    0,
			wantScoreAbove: 0.0,
		},
		{
			name:           "zero_correlation_no_rescue",
			lastInjected:   now.AddDate(0, 0, -5),
			successCorr:    0.0,
			wantRescued:    0,
			wantScoreAbove: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := skill("s1", extraction.CategoryTactical, 0.6, now.AddDate(0, 0, -90))
			s.LastInjectedAt = tt.lastInjected
			s.SuccessCorrelation = tt.successCorr

			w := &mockWriter{}
			r := NewRunner(&mockReader{skills: []extraction.SkillRecord{s}}, w, cfg, slog.Default())
			r.now = func() time.Time { return now }

			res, err := r.RunCycle(context.Background(), "lib1")
			if err != nil {
				t.Fatal(err)
			}
			if res.Rescued != tt.wantRescued {
				t.Errorf("rescued=%d, want %d", res.Rescued, tt.wantRescued)
			}
		})
	}
}

func TestDeprecationThreshold(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	// Skill with very low decay, updated long ago — should go to 0.
	s := skill("s1", extraction.CategoryTactical, 0.06, now.AddDate(0, 0, -180))
	w := &mockWriter{}
	r := NewRunner(&mockReader{skills: []extraction.SkillRecord{s}}, w, cfg, slog.Default())
	r.now = func() time.Time { return now }

	res, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Deprecated != 1 {
		t.Errorf("deprecated=%d, want 1", res.Deprecated)
	}
	if len(w.updates) != 1 || w.updates[0].score != 0.0 {
		t.Errorf("expected score=0.0, got updates=%v", w.updates)
	}
}

func TestEmptyLibrary(t *testing.T) {
	w := &mockWriter{}
	r := NewRunner(&mockReader{}, w, DefaultConfig(), slog.Default())
	r.now = func() time.Time { return fixedNow() }

	res, err := r.RunCycle(context.Background(), "empty")
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed != 0 || res.Demoted != 0 || res.Deprecated != 0 || res.Rescued != 0 {
		t.Errorf("expected all zeros, got %+v", res)
	}
	if len(w.updates) != 0 {
		t.Errorf("expected no updates, got %d", len(w.updates))
	}
}

func TestAllFreshSkills(t *testing.T) {
	now := fixedNow()
	// Skills updated just now — no decay.
	skills := []extraction.SkillRecord{
		skill("s1", extraction.CategoryFoundational, 1.0, now),
		skill("s2", extraction.CategoryTactical, 1.0, now),
		skill("s3", extraction.CategoryContextual, 1.0, now),
	}

	w := &mockWriter{}
	r := NewRunner(&mockReader{skills: skills}, w, DefaultConfig(), slog.Default())
	r.now = func() time.Time { return now }

	res, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed != 3 {
		t.Errorf("processed=%d, want 3", res.Processed)
	}
	if res.Demoted != 0 || res.Deprecated != 0 || res.Rescued != 0 {
		t.Errorf("expected no changes, got %+v", res)
	}
	if len(w.updates) != 0 {
		t.Errorf("expected no updates for fresh skills, got %d", len(w.updates))
	}
}

func TestCategorySpecificRates(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	// All start at 1.0, all updated 90 days ago.
	// Tactical (half-life 90) should be ~0.5.
	// Contextual (half-life 180) should be ~0.707.
	// Foundational (half-life 365) should be ~0.843.
	daysAgo := 90
	updatedAt := now.AddDate(0, 0, -daysAgo)
	skills := []extraction.SkillRecord{
		skill("found", extraction.CategoryFoundational, 1.0, updatedAt),
		skill("tact", extraction.CategoryTactical, 1.0, updatedAt),
		skill("ctx", extraction.CategoryContextual, 1.0, updatedAt),
	}

	w := &mockWriter{}
	r := NewRunner(&mockReader{skills: skills}, w, cfg, slog.Default())
	r.now = func() time.Time { return now }

	_, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]float64{
		"found": math.Pow(0.5, 90.0/365.0),
		"tact":  math.Pow(0.5, 90.0/90.0),
		"ctx":   math.Pow(0.5, 90.0/180.0),
	}

	for _, u := range w.updates {
		want, ok := expected[u.id]
		if !ok {
			t.Errorf("unexpected update for %s", u.id)
			continue
		}
		if !almostEqual(u.score, want, 0.001) {
			t.Errorf("id=%s score=%.4f, want=%.4f", u.id, u.score, want)
		}
	}
}
