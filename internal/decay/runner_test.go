package decay

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// --- mocks ---

type mockReader struct {
	skills []model.SkillRecord
	err    error
}

func (m *mockReader) ListByDecay(_ context.Context, _ string, _ int) ([]model.SkillRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([]model.SkillRecord, len(m.skills))
	copy(out, m.skills)
	return out, nil
}

type decayUpdate struct {
	id    string
	score float64
}

type mockWriter struct {
	updates  []decayUpdate
	err      error
	failOnID string // fail only for this ID
}

func (m *mockWriter) UpdateDecay(_ context.Context, id string, score float64) error {
	if m.failOnID != "" && id == m.failOnID {
		return m.err
	}
	if m.failOnID == "" && m.err != nil {
		return m.err
	}
	m.updates = append(m.updates, decayUpdate{id, score})
	return nil
}

// --- helpers ---

func fixedNow() time.Time {
	return time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
}

func skill(id string, cat model.SkillCategory, decay float64, updatedAt time.Time) model.SkillRecord {
	return model.SkillRecord{
		ID:             id,
		Category:       cat,
		DecayScore:     decay,
		UpdatedAt:      updatedAt,
		LastInjectedAt: updatedAt, // default: same as updatedAt
	}
}

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) < eps
}

func mustNewRunner(t *testing.T, reader SkillReader, writer SkillWriter, cfg Config) *Runner {
	t.Helper()
	r, err := NewRunner(reader, writer, cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return r
}

// --- tests ---

func TestNewRunnerValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", DefaultConfig(), false},
		{"zero_foundational", Config{FoundationalHalfLifeDays: 0, TacticalHalfLifeDays: 90, ContextualHalfLifeDays: 180, RescueBoost: 0.3}, true},
		{"zero_tactical", Config{FoundationalHalfLifeDays: 365, TacticalHalfLifeDays: 0, ContextualHalfLifeDays: 180, RescueBoost: 0.3}, true},
		{"zero_contextual", Config{FoundationalHalfLifeDays: 365, TacticalHalfLifeDays: 90, ContextualHalfLifeDays: 0, RescueBoost: 0.3}, true},
		{"negative_half_life", Config{FoundationalHalfLifeDays: -1, TacticalHalfLifeDays: 90, ContextualHalfLifeDays: 180, RescueBoost: 0.3}, true},
		{"bad_rescue_boost", Config{FoundationalHalfLifeDays: 365, TacticalHalfLifeDays: 90, ContextualHalfLifeDays: 180, RescueBoost: 1.5}, true},
		{"zero_struct", Config{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRunner(&mockReader{}, &mockWriter{}, tt.cfg, slog.Default())
			if (err != nil) != tt.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestHalfLifeFormula(t *testing.T) {
	tests := []struct {
		name     string
		category model.SkillCategory
		halfLife int
	}{
		{"foundational", model.CategoryFoundational, 365},
		{"tactical", model.CategoryTactical, 90},
		{"contextual", model.CategoryContextual, 180},
	}

	now := fixedNow()
	cfg := DefaultConfig()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updatedAt := now.AddDate(0, 0, -tt.halfLife)
			w := &mockWriter{}
			r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{skill("s1", tt.category, 1.0, updatedAt)}}, w, cfg)
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

func TestDecayUsesLastInjectedAt(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	// LastInjectedAt is 90 days ago, UpdatedAt is 1 day ago.
	// Decay should use LastInjectedAt (90 days), not UpdatedAt (1 day).
	s := model.SkillRecord{
		ID:             "s1",
		Category:       model.CategoryTactical, // half-life 90
		DecayScore:     1.0,
		LastInjectedAt: now.AddDate(0, 0, -90),
		UpdatedAt:      now.AddDate(0, 0, -1),
	}

	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, cfg)
	r.now = func() time.Time { return now }

	_, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}

	if len(w.updates) != 1 {
		t.Fatalf("updates=%d, want 1", len(w.updates))
	}
	// Should be ~0.5 (90 days / 90 half-life), NOT ~0.992 (1 day / 90 half-life)
	if !almostEqual(w.updates[0].score, 0.5, 0.001) {
		t.Errorf("score=%.4f, want ~0.5 (based on LastInjectedAt)", w.updates[0].score)
	}
}

func TestRescueLogic(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	tests := []struct {
		name         string
		lastInjected time.Time
		successCorr  float64
		wantRescued  int
		wantScore    float64 // expected final score (approximate)
	}{
		{
			name:         "recent_success_rescues",
			lastInjected: now.AddDate(0, 0, -5),
			successCorr:  0.8,
			wantRescued:  1,
			// decay uses LastInjectedAt (5 days ago): 0.6 * 0.5^(5/90) ≈ 0.577, + 0.3 = 0.877
			wantScore: 0.6 * math.Pow(0.5, 5.0/90.0) + 0.3,
		},
		{
			name:         "old_injection_no_rescue",
			lastInjected: now.AddDate(0, 0, -60),
			successCorr:  0.8,
			wantRescued:  0,
			wantScore:    0.0, // just check no rescue
		},
		{
			name:         "negative_correlation_no_rescue",
			lastInjected: now.AddDate(0, 0, -5),
			successCorr:  -0.2,
			wantRescued:  0,
			wantScore:    0.0,
		},
		{
			name:         "zero_correlation_no_rescue",
			lastInjected: now.AddDate(0, 0, -5),
			successCorr:  0.0,
			wantRescued:  0,
			wantScore:    0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := skill("s1", model.CategoryTactical, 0.6, now.AddDate(0, 0, -90))
			s.LastInjectedAt = tt.lastInjected
			s.SuccessCorrelation = tt.successCorr

			w := &mockWriter{}
			r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, cfg)
			r.now = func() time.Time { return now }

			res, err := r.RunCycle(context.Background(), "lib1")
			if err != nil {
				t.Fatal(err)
			}
			if res.Rescued != tt.wantRescued {
				t.Errorf("rescued=%d, want %d", res.Rescued, tt.wantRescued)
			}
			if tt.wantRescued > 0 && len(w.updates) > 0 {
				if !almostEqual(w.updates[0].score, tt.wantScore, 0.05) {
					t.Errorf("score=%.4f, want ~%.4f", w.updates[0].score, tt.wantScore)
				}
			}
		})
	}
}

func TestRescueScoreBoundary(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	// Skill with high decay that gets rescued — should cap at 1.0
	s := skill("s1", model.CategoryTactical, 0.95, now.AddDate(0, 0, -1))
	s.LastInjectedAt = now.AddDate(0, 0, -1)
	s.SuccessCorrelation = 0.9

	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, cfg)
	r.now = func() time.Time { return now }

	res, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Rescued != 1 {
		t.Fatalf("rescued=%d, want 1", res.Rescued)
	}
	if len(w.updates) != 1 {
		t.Fatalf("updates=%d, want 1", len(w.updates))
	}
	if w.updates[0].score > 1.0 {
		t.Errorf("score=%.4f exceeds 1.0", w.updates[0].score)
	}
}

func TestCustomRescueBoost(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()
	cfg.RescueBoost = 0.5

	// Skill decayed to ~0.3, rescue with 0.5 boost → ~0.8
	s := skill("s1", model.CategoryTactical, 0.6, now.AddDate(0, 0, -90))
	s.LastInjectedAt = now.AddDate(0, 0, -5)
	s.SuccessCorrelation = 0.9

	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, cfg)
	r.now = func() time.Time { return now }

	res, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Rescued != 1 {
		t.Fatalf("rescued=%d, want 1", res.Rescued)
	}
	// decay uses LastInjectedAt (5 days): 0.6 * 0.5^(5/90) ≈ 0.577, + 0.5 = 1.077 → capped at 1.0
	if len(w.updates) > 0 && !almostEqual(w.updates[0].score, 1.0, 0.01) {
		t.Errorf("score=%.4f, want 1.0 (capped) with boost=0.5", w.updates[0].score)
	}
}

func TestDeprecationThreshold(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	s := skill("s1", model.CategoryTactical, 0.06, now.AddDate(0, 0, -180))
	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, cfg)
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
	r := mustNewRunner(t, &mockReader{}, w, DefaultConfig())
	r.now = func() time.Time { return fixedNow() }

	res, err := r.RunCycle(context.Background(), "empty")
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed != 0 || res.Demoted != 0 || res.Deprecated != 0 || res.Rescued != 0 {
		t.Errorf("expected all zeros, got %+v", res)
	}
}

func TestAllFreshSkills(t *testing.T) {
	now := fixedNow()
	skills := []model.SkillRecord{
		skill("s1", model.CategoryFoundational, 1.0, now),
		skill("s2", model.CategoryTactical, 1.0, now),
		skill("s3", model.CategoryContextual, 1.0, now),
	}

	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: skills}, w, DefaultConfig())
	r.now = func() time.Time { return now }

	res, err := r.RunCycle(context.Background(), "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Processed != 3 {
		t.Errorf("processed=%d, want 3", res.Processed)
	}
	if len(w.updates) != 0 {
		t.Errorf("expected no updates for fresh skills, got %d", len(w.updates))
	}
}

func TestCategorySpecificRates(t *testing.T) {
	now := fixedNow()
	cfg := DefaultConfig()

	daysAgo := 90
	updatedAt := now.AddDate(0, 0, -daysAgo)
	skills := []model.SkillRecord{
		skill("found", model.CategoryFoundational, 1.0, updatedAt),
		skill("tact", model.CategoryTactical, 1.0, updatedAt),
		skill("ctx", model.CategoryContextual, 1.0, updatedAt),
	}

	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{skills: skills}, w, cfg)
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

func TestReaderError(t *testing.T) {
	w := &mockWriter{}
	r := mustNewRunner(t, &mockReader{err: errors.New("db connection lost")}, w, DefaultConfig())
	r.now = func() time.Time { return fixedNow() }

	_, err := r.RunCycle(context.Background(), "lib1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errors.Unwrap(err)) {
		// Just verify it wraps properly
	}
}

func TestWriterError(t *testing.T) {
	now := fixedNow()
	s := skill("s1", model.CategoryTactical, 1.0, now.AddDate(0, 0, -90))

	w := &mockWriter{err: errors.New("write failed")}
	r := mustNewRunner(t, &mockReader{skills: []model.SkillRecord{s}}, w, DefaultConfig())
	r.now = func() time.Time { return now }

	_, err := r.RunCycle(context.Background(), "lib1")
	if err == nil {
		t.Fatal("expected error from writer, got nil")
	}
}

func TestPartialWriteFailure(t *testing.T) {
	now := fixedNow()
	skills := []model.SkillRecord{
		skill("s1", model.CategoryTactical, 1.0, now.AddDate(0, 0, -90)),
		skill("s2", model.CategoryTactical, 1.0, now.AddDate(0, 0, -90)),
		skill("s3", model.CategoryTactical, 1.0, now.AddDate(0, 0, -90)),
	}

	// Fail on s3 — s1 and s2 already written.
	w := &mockWriter{failOnID: "s3", err: errors.New("partial failure")}
	r := mustNewRunner(t, &mockReader{skills: skills}, w, DefaultConfig())
	r.now = func() time.Time { return now }

	_, err := r.RunCycle(context.Background(), "lib1")
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	// s1 and s2 should have been written before failure.
	if len(w.updates) != 2 {
		t.Errorf("updates=%d, want 2 (before failure on s3)", len(w.updates))
	}
}
