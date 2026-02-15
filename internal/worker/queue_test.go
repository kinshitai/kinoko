package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/model"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

func setupQueue(t *testing.T) (*SQLiteQueue, string) {
	t.Helper()
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// Disable FK checks for tests — we don't insert skill rows.
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	dataDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.QueueDepthWarning = 5
	cfg.QueueDepthCritical = 10
	cfg.InitialBackoff = 1 * time.Second
	cfg.MaxBackoff = 10 * time.Second

	q := NewSQLiteQueue(store, dataDir, cfg, slog.Default())
	return q, dataDir
}

func makeSession(id string) model.SessionRecord {
	now := time.Now().UTC()
	return model.SessionRecord{
		ID:              id,
		StartedAt:       now.Add(-10 * time.Minute),
		EndedAt:         now,
		DurationMinutes: 10,
		ToolCallCount:   5,
		ErrorCount:      0,
		MessageCount:    20,
		ErrorRate:       0,
		HasSuccessfulExec: true,
		TokensUsed:      1000,
		AgentModel:      "test-model",
		UserID:          "user-1",
		LibraryID:       "lib-1",
	}
}

func TestEnqueueAndClaim(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	session := makeSession("sess-1")
	if err := q.Enqueue(ctx, session, []byte("log content")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	entry, err := q.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.SessionID != "sess-1" {
		t.Errorf("session id = %q, want sess-1", entry.SessionID)
	}
	if entry.LibraryID != "lib-1" {
		t.Errorf("library id = %q, want lib-1", entry.LibraryID)
	}
	if entry.RetryCount != 0 {
		t.Errorf("retry count = %d, want 0", entry.RetryCount)
	}

	// Log file should exist.
	content, err := os.ReadFile(entry.LogContentPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(content) != "log content" {
		t.Errorf("log content = %q, want %q", content, "log content")
	}
}

func TestClaimEmptyQueue(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	entry, err := q.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected nil, got %+v", entry)
	}
}

func TestClaimAtomic(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("sess-1"), []byte("log")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var mu sync.Mutex
	var claimed []string
	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			entry, err := q.Claim(ctx, fmt.Sprintf("worker-%d", id))
			if err != nil {
				t.Errorf("claim error: %v", err)
				return
			}
			if entry != nil {
				mu.Lock()
				claimed = append(claimed, entry.SessionID)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if len(claimed) != 1 {
		t.Errorf("expected 1 claim, got %d", len(claimed))
	}
}

func TestFailAndRetry(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("sess-1"), []byte("log")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	entry, _ := q.Claim(ctx, "worker-1")
	if entry == nil {
		t.Fatal("expected entry")
	}

	// Fail it.
	if err := q.Fail(ctx, "sess-1", errors.New("llm timeout")); err != nil {
		t.Fatalf("fail: %v", err)
	}

	// Should not be claimable immediately (next_retry_at in future).
	entry, _ = q.Claim(ctx, "worker-1")
	if entry != nil {
		t.Fatal("expected nil after fail (retry not due)")
	}

	// Fast-forward: set next_retry_at to past.
	q.store.DB().ExecContext(ctx, `UPDATE sessions SET next_retry_at = datetime('now', '-1 minute') WHERE id = 'sess-1'`)

	entry, err := q.Claim(ctx, "worker-1")
	if err != nil {
		t.Fatalf("claim after retry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry after retry window")
	}
	if entry.RetryCount != 1 {
		t.Errorf("retry count = %d, want 1", entry.RetryCount)
	}
}

func TestFailPermanent(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("sess-1"), []byte("log")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	q.Claim(ctx, "worker-1")

	if err := q.FailPermanent(ctx, "sess-1", errors.New("bad data")); err != nil {
		t.Fatalf("fail permanent: %v", err)
	}

	// Check status is failed.
	var status string
	q.store.DB().QueryRowContext(ctx, `SELECT extraction_status FROM sessions WHERE id = 'sess-1'`).Scan(&status)
	if status != "failed" {
		t.Errorf("status = %q, want failed", status)
	}

	// Should not be claimable.
	entry, _ := q.Claim(ctx, "worker-1")
	if entry != nil {
		t.Fatal("expected nil for permanently failed session")
	}
}

func TestDepth(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	depth, _ := q.Depth(ctx)
	if depth != 0 {
		t.Errorf("initial depth = %d, want 0", depth)
	}

	for i := 0; i < 3; i++ {
		q.Enqueue(ctx, makeSession(fmt.Sprintf("sess-%d", i)), []byte("log"))
	}

	depth, _ = q.Depth(ctx)
	if depth != 3 {
		t.Errorf("depth = %d, want 3", depth)
	}

	// Claim one — depth should decrease.
	q.Claim(ctx, "worker-1")
	depth, _ = q.Depth(ctx)
	if depth != 2 {
		t.Errorf("depth after claim = %d, want 2", depth)
	}
}

func TestBackpressureRejection(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	// Fill to critical (10).
	for i := 0; i < 10; i++ {
		if err := q.Enqueue(ctx, makeSession(fmt.Sprintf("sess-%d", i)), []byte("log")); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Next enqueue should fail.
	err := q.Enqueue(ctx, makeSession("sess-overflow"), []byte("log"))
	if !errors.Is(err, ErrBackpressure) {
		t.Errorf("expected ErrBackpressure, got %v", err)
	}
}

func TestRequeueStale(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("sess-1"), []byte("log")); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	entry, _ := q.Claim(ctx, "worker-1")
	if entry == nil {
		t.Fatal("expected entry")
	}

	// Set claimed_at to 20 minutes ago.
	q.store.DB().ExecContext(ctx, `UPDATE sessions SET claimed_at = datetime('now', '-20 minutes') WHERE id = 'sess-1'`)

	n, err := q.RequeueStale(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("requeue stale: %v", err)
	}
	if n != 1 {
		t.Errorf("requeued = %d, want 1", n)
	}

	// Should be claimable again.
	entry, _ = q.Claim(ctx, "worker-2")
	if entry == nil {
		t.Fatal("expected entry after requeue")
	}
}

func TestFIFOOrdering(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	// Enqueue in order with slight delay to ensure different created_at.
	for i := 0; i < 3; i++ {
		q.Enqueue(ctx, makeSession(fmt.Sprintf("sess-%d", i)), []byte("log"))
	}

	// Claims should come back in FIFO order.
	for i := 0; i < 3; i++ {
		entry, _ := q.Claim(ctx, "worker-1")
		if entry == nil {
			t.Fatalf("claim %d: expected entry", i)
		}
		expected := fmt.Sprintf("sess-%d", i)
		if entry.SessionID != expected {
			t.Errorf("claim %d: got %s, want %s", i, entry.SessionID, expected)
		}
	}
}

func TestComplete(t *testing.T) {
	q, _ := setupQueue(t)
	ctx := context.Background()

	q.Enqueue(ctx, makeSession("sess-1"), []byte("log"))
	q.Claim(ctx, "worker-1")

	result := &model.ExtractionResult{
		Status: model.StatusExtracted,
		Skill:  &model.SkillRecord{ID: "skill-1"},
	}
	if err := q.Complete(ctx, "sess-1", result); err != nil {
		t.Fatalf("complete: %v", err)
	}

	var status, skillID string
	q.store.DB().QueryRowContext(ctx, `SELECT extraction_status, COALESCE(extracted_skill_id,'') FROM sessions WHERE id = 'sess-1'`).Scan(&status, &skillID)
	if status != "extracted" {
		t.Errorf("status = %q, want extracted", status)
	}
	if skillID != "skill-1" {
		t.Errorf("skill id = %q, want skill-1", skillID)
	}
}

// end
