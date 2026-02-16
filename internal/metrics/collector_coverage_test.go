package metrics

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	tables := `
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			extraction_status TEXT DEFAULT 'pending',
			rejected_at_stage INTEGER DEFAULT 0
		);
		CREATE TABLE skills (
			id TEXT PRIMARY KEY,
			category TEXT DEFAULT 'general',
			q_composite_score REAL DEFAULT 0,
			q_critic_confidence REAL DEFAULT 0,
			decay_score REAL DEFAULT 1.0
		);
		CREATE TABLE injection_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT,
			skill_id TEXT,
			delivered INTEGER DEFAULT 0,
			ab_group TEXT DEFAULT '',
			session_outcome TEXT DEFAULT ''
		);
		CREATE TABLE human_review_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			verdict TEXT
		);
	`
	if _, err := db.Exec(tables); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNewCollector_Default(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)
	if c.minSampleSize != 100 {
		t.Fatalf("expected default minSampleSize=100, got %d", c.minSampleSize)
	}
}

func TestWithMinSampleSize(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db, WithMinSampleSize(50))
	if c.minSampleSize != 50 {
		t.Fatalf("expected minSampleSize=50, got %d", c.minSampleSize)
	}
}

func TestWithMinSampleSize_Zero(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db, WithMinSampleSize(0))
	if c.minSampleSize != 100 {
		t.Fatalf("expected default minSampleSize=100 for n=0, got %d", c.minSampleSize)
	}
}

func TestCollect_Empty(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)
	m, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}
	if m.TotalSessions != 0 {
		t.Fatalf("expected 0 sessions, got %d", m.TotalSessions)
	}
	if m.AB != nil {
		t.Fatal("expected nil AB for empty db")
	}
}

func TestCollect_WithData(t *testing.T) {
	db := setupTestDB(t)

	// Insert sessions.
	db.Exec(`INSERT INTO sessions (id, extraction_status, rejected_at_stage) VALUES ('s1', 'extracted', 0)`)
	db.Exec(`INSERT INTO sessions (id, extraction_status, rejected_at_stage) VALUES ('s2', 'rejected', 1)`)
	db.Exec(`INSERT INTO sessions (id, extraction_status, rejected_at_stage) VALUES ('s3', 'error', 0)`)
	db.Exec(`INSERT INTO sessions (id, extraction_status, rejected_at_stage) VALUES ('s4', 'extracted', 0)`)

	// Insert skills.
	db.Exec(`INSERT INTO skills (id, category, q_composite_score, q_critic_confidence, decay_score) VALUES ('sk1', 'debugging', 0.8, 0.9, 0.9)`)
	db.Exec(`INSERT INTO skills (id, category, q_composite_score, q_critic_confidence, decay_score) VALUES ('sk2', 'testing', 0.7, 0.8, 0.3)`)

	// Insert injection events.
	db.Exec(`INSERT INTO injection_events (session_id, skill_id, delivered, ab_group, session_outcome) VALUES ('s1', 'sk1', 1, '', '')`)

	// Insert review samples.
	db.Exec(`INSERT INTO human_review_samples (verdict) VALUES ('agree')`)
	db.Exec(`INSERT INTO human_review_samples (verdict) VALUES ('disagree_should_reject')`)

	c := NewCollector(db)
	m, err := c.Collect()
	if err != nil {
		t.Fatal(err)
	}

	if m.TotalSessions != 4 {
		t.Fatalf("TotalSessions = %d, want 4", m.TotalSessions)
	}
	if m.Extracted != 2 {
		t.Fatalf("Extracted = %d, want 2", m.Extracted)
	}
	if m.Rejected != 1 {
		t.Fatalf("Rejected = %d, want 1", m.Rejected)
	}
	if m.Errored != 1 {
		t.Fatalf("Errored = %d, want 1", m.Errored)
	}
	if m.TotalSkills != 2 {
		t.Fatalf("TotalSkills = %d, want 2", m.TotalSkills)
	}
	if m.HumanReviewTotal != 2 {
		t.Fatalf("HumanReviewTotal = %d, want 2", m.HumanReviewTotal)
	}
	if m.ExtractionPrecision != 0.5 {
		t.Fatalf("ExtractionPrecision = %f, want 0.5", m.ExtractionPrecision)
	}
}

func TestCollectAB_NoData(t *testing.T) {
	db := setupTestDB(t)
	c := NewCollector(db)
	ab, err := c.collectAB()
	if err != nil {
		t.Fatal(err)
	}
	if ab != nil {
		t.Fatal("expected nil AB for no AB data")
	}
}

func TestCollectAB_WithData(t *testing.T) {
	db := setupTestDB(t)

	// Insert treatment and control events.
	for i := 0; i < 5; i++ {
		outcome := "success"
		if i >= 3 {
			outcome = "failure"
		}
		db.Exec(`INSERT INTO injection_events (session_id, skill_id, delivered, ab_group, session_outcome) VALUES (?, 'sk1', 1, 'treatment', ?)`,
			"t"+string(rune('0'+i)), outcome)
	}
	for i := 0; i < 5; i++ {
		outcome := "success"
		if i >= 4 {
			outcome = "failure"
		}
		db.Exec(`INSERT INTO injection_events (session_id, skill_id, delivered, ab_group, session_outcome) VALUES (?, 'sk1', 1, 'control', ?)`,
			"c"+string(rune('0'+i)), outcome)
	}

	c := NewCollector(db, WithMinSampleSize(3))
	ab, err := c.collectAB()
	if err != nil {
		t.Fatal(err)
	}
	if ab == nil {
		t.Fatal("expected non-nil AB result")
	}
	if ab.TreatmentSessions != 5 {
		t.Fatalf("TreatmentSessions = %d, want 5", ab.TreatmentSessions)
	}
	if ab.ControlSessions != 5 {
		t.Fatalf("ControlSessions = %d, want 5", ab.ControlSessions)
	}
	if ab.TreatmentSuccess != 3 {
		t.Fatalf("TreatmentSuccess = %d, want 3", ab.TreatmentSuccess)
	}
	if ab.ControlSuccess != 4 {
		t.Fatalf("ControlSuccess = %d, want 4", ab.ControlSuccess)
	}
	if !ab.SufficientData {
		t.Fatal("expected SufficientData=true with minSampleSize=3")
	}
	// Z-test should have been computed.
	if ab.ZScore == 0 && ab.PValue == 0 {
		t.Fatal("expected z-test to be computed")
	}
}

func TestCollectAB_InsufficientData(t *testing.T) {
	db := setupTestDB(t)
	db.Exec(`INSERT INTO injection_events (session_id, skill_id, delivered, ab_group, session_outcome) VALUES ('s1', 'sk1', 1, 'treatment', 'success')`)

	c := NewCollector(db, WithMinSampleSize(100))
	ab, err := c.collectAB()
	if err != nil {
		t.Fatal(err)
	}
	if ab == nil {
		t.Fatal("expected non-nil AB result")
	}
	if ab.SufficientData {
		t.Fatal("expected SufficientData=false")
	}
	if ab.ZScore != 0 {
		t.Fatal("z-test should not be computed with insufficient data")
	}
}
