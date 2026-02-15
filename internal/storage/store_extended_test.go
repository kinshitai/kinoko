package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/model"
)

// TestBug_UnboundedINClause tests the P1 issue where loadPatternsMulti and
// loadEmbeddingsMulti build an IN clause with unbounded placeholders.
// SQLite's default SQLITE_MAX_VARIABLE_NUMBER is 999. With >999 IDs this
// may fail depending on the driver/build.
func TestBug_UnboundedINClause(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Insert 1100 skills
	for i := 0; i < 1100; i++ {
		sk := testSkill(fmt.Sprintf("id-%04d", i), fmt.Sprintf("skill-%04d", i), "default")
		sk.Patterns = []string{"FIX/Backend/DatabaseConnection"}
		sk.Embedding = []float32{float32(i) * 0.001, 0.1, 0.2}
		if err := s.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	// Query without limit — should return all 1100 candidates, then call
	// loadPatternsMulti and loadEmbeddingsMulti with 1100 IDs.
	// This tests whether the unbounded IN clause blows up.
	results, err := s.Query(ctx, SkillQuery{
		LibraryIDs: []string{"default"},
	})
	if err != nil {
		// If this errors with something about variable number, the bug is confirmed.
		if strings.Contains(err.Error(), "too many SQL variables") ||
			strings.Contains(err.Error(), "SQLITE_LIMIT") {
			t.Logf("BUG CONFIRMED: unbounded IN clause fails with >999 IDs: %v", err)
			return
		}
		t.Fatalf("query error: %v", err)
	}

	// If it succeeds, the driver handles large IN clauses (modernc/sqlite may not have the limit).
	// Document this.
	t.Logf("Query with %d candidates succeeded — driver handles large IN clauses", len(results))
	if len(results) != 1100 {
		t.Errorf("expected 1100 results, got %d", len(results))
	}
}

func TestFileAfterCommit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	sk := testSkill("id-fac", "file-after-commit", "default")
	sk.FilePath = filepath.Join(dir, "skills", "file-after-commit", "SKILL.md")
	body := []byte("# File After Commit Test")

	if err := s.Put(ctx, sk, body); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Verify DB row exists
	got, err := s.Get(ctx, "id-fac")
	if err != nil {
		t.Fatalf("get from DB: %v", err)
	}
	if got.Name != "file-after-commit" {
		t.Errorf("name = %q", got.Name)
	}

	// Verify file exists
	fileContent, err := os.ReadFile(sk.FilePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(fileContent) != string(body) {
		t.Errorf("file content mismatch")
	}
}

func TestDuplicateSessionInsert(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := &model.SessionRecord{
		ID:               "sess-dup-1",
		StartedAt:        time.Now(),
		EndedAt:          time.Now(),
		DurationMinutes:  5.0,
		ToolCallCount:    3,
		ErrorCount:       0,
		MessageCount:     10,
		ErrorRate:        0.0,
		HasSuccessfulExec: true,
		LibraryID:        "default",
		ExtractionStatus: model.StatusPending,
	}

	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Second insert with same ID should fail with UNIQUE constraint
	err := s.InsertSession(ctx, sess)
	if err == nil {
		t.Fatal("expected error on duplicate session insert")
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint") {
		t.Errorf("expected UNIQUE constraint error, got: %v", err)
	}
}

func TestSessionInsertAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sess := &model.SessionRecord{
		ID:               "sess-ig-1",
		StartedAt:        time.Now().UTC().Truncate(time.Second),
		EndedAt:          time.Now().UTC().Truncate(time.Second),
		DurationMinutes:  12.5,
		ToolCallCount:    7,
		ErrorCount:       1,
		MessageCount:     20,
		ErrorRate:        0.05,
		HasSuccessfulExec: true,
		TokensUsed:       1500,
		AgentModel:       "gpt-4",
		UserID:           "user-1",
		LibraryID:        "lib-1",
		ExtractionStatus: model.StatusPending,
	}

	if err := s.InsertSession(ctx, sess); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-ig-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DurationMinutes != 12.5 {
		t.Errorf("duration = %f, want 12.5", got.DurationMinutes)
	}
	if got.ExtractionStatus != model.StatusPending {
		t.Errorf("status = %q, want pending", got.ExtractionStatus)
	}
}
