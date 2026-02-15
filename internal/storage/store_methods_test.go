package storage

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/model"
)

// Tests for store methods that lack coverage — R6 area.
// Must exist BEFORE splitting store.go into focused files.

// insertTestSession creates a session for FK references.
func insertTestSession(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	sess := &model.SessionRecord{
		ID:               id,
		StartedAt:        time.Now().UTC(),
		EndedAt:          time.Now().UTC(),
		LibraryID:        "default",
		ExtractionStatus: model.StatusPending,
	}
	if err := s.InsertSession(context.Background(), sess); err != nil {
		t.Fatalf("insert test session: %v", err)
	}
}

// insertTestSkillForFK creates a skill for FK references.
func insertTestSkillForFK(t *testing.T, s *SQLiteStore, id string) {
	t.Helper()
	sk := testSkill(id, "fk-skill-"+id, "default")
	if err := s.Put(context.Background(), sk, nil); err != nil {
		t.Fatalf("insert test skill: %v", err)
	}
}

func TestWriteInjectionEvent(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	insertTestSkillForFK(t, s, "skill-iev")

	ev := InjectionEventRecord{
		ID:             "iev-1",
		SessionID:      "sess-1",
		SkillID:        "skill-iev",
		RankPosition:   1,
		MatchScore:     0.85,
		PatternOverlap: 0.67,
		CosineSim:      0.92,
		HistoricalRate: 0.75,
		InjectedAt:     time.Now().UTC(),
		ABGroup:        "treatment",
		Delivered:      true,
	}

	if err := s.WriteInjectionEvent(ctx, ev); err != nil {
		t.Fatalf("write injection event: %v", err)
	}

	var id, sessID, skillID, abGroup string
	var rank int
	var delivered bool
	err := s.db.QueryRowContext(ctx,
		`SELECT id, session_id, skill_id, rank_position, ab_group, delivered FROM injection_events WHERE id = ?`,
		"iev-1").Scan(&id, &sessID, &skillID, &rank, &abGroup, &delivered)
	if err != nil {
		t.Fatalf("query injection event: %v", err)
	}
	if sessID != "sess-1" || skillID != "skill-iev" || rank != 1 || abGroup != "treatment" || !delivered {
		t.Errorf("got (%s, %s, %d, %s, %v)", sessID, skillID, rank, abGroup, delivered)
	}
}

func TestWriteInjectionEvent_ControlGroup(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	insertTestSkillForFK(t, s, "skill-ctrl")

	ev := InjectionEventRecord{
		ID:         "iev-ctrl",
		SessionID:  "sess-ctrl",
		SkillID:    "skill-ctrl",
		InjectedAt: time.Now().UTC(),
		ABGroup:    "control",
		Delivered:  false,
	}

	if err := s.WriteInjectionEvent(ctx, ev); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestUpdateInjectionOutcome(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	insertTestSkillForFK(t, s, "skill-out")

	ev := InjectionEventRecord{
		ID:         "iev-out",
		SessionID:  "sess-out",
		SkillID:    "skill-out",
		InjectedAt: time.Now().UTC(),
	}
	if err := s.WriteInjectionEvent(ctx, ev); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := s.UpdateInjectionOutcome(ctx, "sess-out", "success"); err != nil {
		t.Fatalf("update outcome: %v", err)
	}

	var outcome string
	err := s.db.QueryRowContext(ctx,
		`SELECT session_outcome FROM injection_events WHERE session_id = ?`, "sess-out").Scan(&outcome)
	if err != nil {
		t.Fatalf("query outcome: %v", err)
	}
	if outcome != "success" {
		t.Errorf("outcome = %q, want success", outcome)
	}
}

func TestUpdateInjectionOutcome_MultipleEvents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		insertTestSkillForFK(t, s, "skill-m"+string(rune('a'+i)))
	}

	for i, id := range []string{"iev-m1", "iev-m2", "iev-m3"} {
		ev := InjectionEventRecord{
			ID:           id,
			SessionID:    "sess-multi",
			SkillID:      "skill-m" + string(rune('a'+i)),
			RankPosition: i + 1,
			InjectedAt:   time.Now().UTC(),
		}
		if err := s.WriteInjectionEvent(ctx, ev); err != nil {
			t.Fatalf("write %s: %v", id, err)
		}
	}

	if err := s.UpdateInjectionOutcome(ctx, "sess-multi", "failure"); err != nil {
		t.Fatalf("update: %v", err)
	}

	var count int
	s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM injection_events WHERE session_id = ? AND session_outcome = ?`,
		"sess-multi", "failure").Scan(&count)
	if count != 3 {
		t.Errorf("updated %d events, want 3", count)
	}
}

func TestInsertReviewSample(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	insertTestSession(t, s, "sess-review")

	resultJSON := []byte(`{"status":"extracted","skill":{"name":"test-skill"}}`)
	if err := s.InsertReviewSample(ctx, "sess-review", resultJSON); err != nil {
		t.Fatalf("insert review sample: %v", err)
	}

	var sessionID, result string
	err := s.db.QueryRowContext(ctx,
		`SELECT session_id, extraction_result FROM human_review_samples WHERE session_id = ?`,
		"sess-review").Scan(&sessionID, &result)
	if err != nil {
		t.Fatalf("query review sample: %v", err)
	}
	if sessionID != "sess-review" {
		t.Errorf("session_id = %q", sessionID)
	}
	if result != string(resultJSON) {
		t.Errorf("result mismatch")
	}
}

func TestUpdateSessionResult(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := &model.SessionRecord{
		ID:               "sess-upd",
		StartedAt:        time.Now().UTC(),
		EndedAt:          time.Now().UTC(),
		DurationMinutes:  5.0,
		LibraryID:        "default",
		ExtractionStatus: model.StatusPending,
	}
	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("insert: %v", err)
	}

	sess.ExtractionStatus = model.StatusExtracted
	sess.ExtractedSkillID = "" // No FK skill needed if empty
	if err := s.UpdateSessionResult(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-upd")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ExtractionStatus != model.StatusExtracted {
		t.Errorf("status = %q, want extracted", got.ExtractionStatus)
	}
}

func TestUpdateSessionResult_Rejection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := &model.SessionRecord{
		ID:               "sess-rej",
		StartedAt:        time.Now().UTC(),
		EndedAt:          time.Now().UTC(),
		LibraryID:        "default",
		ExtractionStatus: model.StatusPending,
	}
	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("insert: %v", err)
	}

	sess.ExtractionStatus = model.StatusRejected
	sess.RejectedAtStage = 2
	sess.RejectionReason = "novelty too low"
	if err := s.UpdateSessionResult(ctx, sess); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-rej")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RejectedAtStage != 2 {
		t.Errorf("rejected_at_stage = %d, want 2", got.RejectedAtStage)
	}
	if got.RejectionReason != "novelty too low" {
		t.Errorf("rejection_reason = %q", got.RejectionReason)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetSession(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFloat32sRoundTrip(t *testing.T) {
	tests := [][]float32{
		{0.1, 0.2, 0.3},
		{-1.0, 0, 1.0},
		{math.MaxFloat32, math.SmallestNonzeroFloat32},
		{},
		nil,
	}
	for _, input := range tests {
		b := float32sToBytes(input)
		got := bytesToFloat32s(b)
		if len(got) != len(input) {
			t.Errorf("len mismatch: %d vs %d", len(got), len(input))
			continue
		}
		for i := range input {
			if got[i] != input[i] {
				t.Errorf("index %d: %f != %f", i, got[i], input[i])
			}
		}
	}
}

func TestNullString(t *testing.T) {
	ns := nullString("")
	if ns.Valid {
		t.Error("empty string should be invalid")
	}
	ns = nullString("hello")
	if !ns.Valid || ns.String != "hello" {
		t.Errorf("got %v", ns)
	}
}

func TestNullTime(t *testing.T) {
	nt := nullTime(time.Time{})
	if nt.Valid {
		t.Error("zero time should be invalid")
	}
	now := time.Now()
	nt = nullTime(now)
	if !nt.Valid || nt.Time != now {
		t.Errorf("got %v", nt)
	}
}

func TestCosineSimilarity_EdgeCases(t *testing.T) {
	if v := cosineSimilarity([]float32{1, 0}, []float32{1}); v != 0 {
		t.Errorf("different lengths = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{0, 0, 0}, []float32{0, 0, 0}); v != 0 {
		t.Errorf("zero vectors = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{0, 0, 0}); v != 0 {
		t.Errorf("one zero = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0}); math.Abs(v+1.0) > 0.001 {
		t.Errorf("opposite = %f, want -1", v)
	}
}

func TestUpdateUsage_SuccessCorrelation(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-corr", "corr-skill", "default")
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Insert injection events with outcomes
	for i, outcome := range []string{"success", "success", "failure"} {
		ev := InjectionEventRecord{
			ID:         "iev-corr-" + string(rune('a'+i)),
			SessionID:  "sess-corr-" + string(rune('a'+i)),
			SkillID:    "id-corr",
			InjectedAt: time.Now().UTC(),
		}
		if err := s.WriteInjectionEvent(ctx, ev); err != nil {
			t.Fatalf("write event: %v", err)
		}
		s.db.ExecContext(ctx,
			`UPDATE injection_events SET session_outcome = ? WHERE id = ?`,
			outcome, ev.ID)
	}

	if err := s.UpdateUsage(ctx, "id-corr", "success"); err != nil {
		t.Fatalf("update usage: %v", err)
	}

	got, _ := s.Get(ctx, "id-corr")
	// (2 success - 1 failure) / 3 total = 1/3 ≈ 0.333
	if math.Abs(got.SuccessCorrelation-1.0/3.0) > 0.01 {
		t.Errorf("success_correlation = %f, want ~0.333", got.SuccessCorrelation)
	}
}
