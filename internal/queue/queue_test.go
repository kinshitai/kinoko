package queue

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/worker"
)

func setup(t *testing.T) (*Store, *Queue, string) {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "queue.db")
	store, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	cfg := worker.DefaultConfig()
	cfg.QueueDepthCritical = 5
	cfg.QueueDepthWarning = 3
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	q := NewQueue(store, dir, cfg, log)
	return store, q, dir
}

func makeSession(id string) model.SessionRecord {
	now := time.Now().UTC()
	return model.SessionRecord{
		ID:                id,
		StartedAt:         now.Add(-10 * time.Minute),
		EndedAt:           now,
		DurationMinutes:   10,
		ToolCallCount:     5,
		ErrorCount:        1,
		MessageCount:      20,
		ErrorRate:         0.05,
		HasSuccessfulExec: true,
		TokensUsed:        1000,
		AgentModel:        "test-model",
		UserID:            "user1",
		LibraryID:         "lib1",
	}
}

func TestEnqueueClaimComplete(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("s1"), []byte("log content")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	depth, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if depth != 1 {
		t.Fatalf("expected depth 1, got %d", depth)
	}

	entry, err := q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if entry == nil {
		t.Fatal("Claim returned nil")
	}
	if entry.SessionID != "s1" {
		t.Fatalf("expected s1, got %s", entry.SessionID)
	}
	if entry.LibraryID != "lib1" {
		t.Fatalf("expected lib1, got %s", entry.LibraryID)
	}

	// Depth should be 0 after claim (only counts 'queued').
	depth, _ = q.Depth(ctx)
	if depth != 0 {
		t.Fatalf("expected depth 0 after claim, got %d", depth)
	}

	result := &model.ExtractionResult{Status: model.StatusExtracted}
	if err := q.Complete(ctx, "s1", result); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Nothing left to claim.
	entry, err = q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim after complete: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil after complete")
	}
}

func TestEnqueueClaimFailRetry(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("s2"), []byte("log")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	entry, err := q.Claim(ctx, "w1")
	if err != nil || entry == nil {
		t.Fatalf("Claim: %v / %v", err, entry)
	}

	if err := q.Fail(ctx, "s2", fmt.Errorf("transient")); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// Immediately after fail, next_retry_at is in the future, so Claim returns nil.
	entry, err = q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim after fail: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil — retry not yet due")
	}

	// Manually set next_retry_at to the past to simulate time passing.
	_, err = q.store.DB().ExecContext(ctx, `UPDATE queue_entries SET next_retry_at = datetime('now', '-1 seconds') WHERE session_id = 's2'`)
	if err != nil {
		t.Fatalf("manual update: %v", err)
	}

	entry, err = q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim retry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry on retry")
	}
	if entry.RetryCount != 1 {
		t.Fatalf("expected retry_count 1, got %d", entry.RetryCount)
	}
}

func TestBackpressure(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	// QueueDepthCritical is 5.
	for i := 0; i < 5; i++ {
		if err := q.Enqueue(ctx, makeSession(fmt.Sprintf("bp-%d", i)), []byte("x")); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	err := q.Enqueue(ctx, makeSession("bp-overflow"), []byte("x"))
	if err == nil {
		t.Fatal("expected backpressure error")
	}
	if err != worker.ErrBackpressure {
		t.Fatalf("expected ErrBackpressure, got %v", err)
	}
}

func TestRequeueStale(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("stale1"), []byte("x")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	entry, err := q.Claim(ctx, "w1")
	if err != nil || entry == nil {
		t.Fatalf("Claim: %v / %v", err, entry)
	}

	// Set claimed_at to 1 hour ago.
	_, err = q.store.DB().ExecContext(ctx, `UPDATE queue_entries SET claimed_at = datetime('now', '-3600 seconds') WHERE session_id = 'stale1'`)
	if err != nil {
		t.Fatalf("manual update: %v", err)
	}

	n, err := q.RequeueStale(ctx, 10*time.Minute)
	if err != nil {
		t.Fatalf("RequeueStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 requeued, got %d", n)
	}

	// Should be claimable again.
	entry, err = q.Claim(ctx, "w2")
	if err != nil || entry == nil {
		t.Fatal("expected claimable after requeue")
	}
}

func TestConcurrentClaims(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("conc1"), []byte("x")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var wg sync.WaitGroup
	results := make([]*worker.QueueEntry, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = q.Claim(ctx, fmt.Sprintf("w%d", idx))
		}(i)
	}
	wg.Wait()

	claimed := 0
	for i := 0; i < 2; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d error: %v", i, errs[i])
		}
		if results[i] != nil {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("expected exactly 1 claim, got %d", claimed)
	}
}

func TestSessionMetadataWriteRead(t *testing.T) {
	store, q, _ := setup(t)
	ctx := context.Background()

	// Enqueue writes session_metadata as a side effect.
	sess := makeSession("meta1")
	if err := q.Enqueue(ctx, sess, []byte("log")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	m, err := GetSessionMetadata(ctx, store, "meta1")
	if err != nil {
		t.Fatalf("GetSessionMetadata: %v", err)
	}
	if m.SessionID != "meta1" {
		t.Fatalf("expected meta1, got %s", m.SessionID)
	}
	if m.ToolCallCount != 5 {
		t.Fatalf("expected 5 tool calls, got %d", m.ToolCallCount)
	}
	if m.LibraryID != "lib1" {
		t.Fatalf("expected lib1, got %s", m.LibraryID)
	}

	// Test PutSessionMetadata (upsert).
	m.ToolCallCount = 99
	if err := PutSessionMetadata(ctx, store, m); err != nil {
		t.Fatalf("PutSessionMetadata: %v", err)
	}
	m2, err := GetSessionMetadata(ctx, store, "meta1")
	if err != nil {
		t.Fatalf("GetSessionMetadata after put: %v", err)
	}
	if m2.ToolCallCount != 99 {
		t.Fatalf("expected 99 after upsert, got %d", m2.ToolCallCount)
	}
}

func TestFailPermanent(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("fp1"), []byte("log")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	entry, err := q.Claim(ctx, "w1")
	if err != nil || entry == nil {
		t.Fatalf("Claim: %v / %v", err, entry)
	}

	if err := q.FailPermanent(ctx, "fp1", fmt.Errorf("bad data")); err != nil {
		t.Fatalf("FailPermanent: %v", err)
	}

	// Should NOT be claimable — it's dead, not retried.
	entry, err = q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim after FailPermanent: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil — permanently failed entry should not be claimable")
	}

	// Verify status is 'failed' in DB.
	var status string
	err = q.store.DB().QueryRowContext(ctx, `SELECT status FROM queue_entries WHERE session_id = 'fp1'`).Scan(&status)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "failed" {
		t.Fatalf("expected status 'failed', got %q", status)
	}
}

func TestDoubleComplete(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("dc1"), []byte("log")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	entry, err := q.Claim(ctx, "w1")
	if err != nil || entry == nil {
		t.Fatalf("Claim: %v / %v", err, entry)
	}

	result := &model.ExtractionResult{Status: model.StatusExtracted}
	if err := q.Complete(ctx, "dc1", result); err != nil {
		t.Fatalf("Complete (1st): %v", err)
	}

	// Second complete — should not error (idempotent, just updates same row again).
	if err := q.Complete(ctx, "dc1", result); err != nil {
		t.Fatalf("Complete (2nd): %v", err)
	}

	// Entry should still not be claimable.
	entry, err = q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim after double complete: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil after double complete")
	}
}

func TestDuplicateEnqueue(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("dup1"), []byte("log")); err != nil {
		t.Fatalf("Enqueue (1st): %v", err)
	}

	// Second enqueue with same session ID should fail (PK violation).
	err := q.Enqueue(ctx, makeSession("dup1"), []byte("log2"))
	if err == nil {
		t.Fatal("expected error on duplicate enqueue, got nil")
	}

	// Depth should still be 1.
	depth, err := q.Depth(ctx)
	if err != nil {
		t.Fatalf("Depth: %v", err)
	}
	if depth != 1 {
		t.Fatalf("expected depth 1, got %d", depth)
	}
}

func TestFailAfterComplete(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	if err := q.Enqueue(ctx, makeSession("fac1"), []byte("log")); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	entry, err := q.Claim(ctx, "w1")
	if err != nil || entry == nil {
		t.Fatalf("Claim: %v / %v", err, entry)
	}

	result := &model.ExtractionResult{Status: model.StatusExtracted}
	if err := q.Complete(ctx, "fac1", result); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// Fail after complete — should not error (UPDATE hits the row but status changes).
	// The key invariant: it should NOT become claimable again.
	if err := q.Fail(ctx, "fac1", fmt.Errorf("late failure")); err != nil {
		t.Fatalf("Fail after Complete: %v", err)
	}

	// Fail should be a no-op on completed entries (WHERE status='pending' guard).
	var status string
	err = q.store.DB().QueryRowContext(ctx, `SELECT status FROM queue_entries WHERE session_id = 'fac1'`).Scan(&status)
	if err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "extracted" {
		t.Fatalf("expected status 'extracted' after fail-on-complete, got %q", status)
	}
}

func TestClaimFromEmptyQueue(t *testing.T) {
	_, q, _ := setup(t)
	ctx := context.Background()

	entry, err := q.Claim(ctx, "w1")
	if err != nil {
		t.Fatalf("Claim from empty queue: %v", err)
	}
	if entry != nil {
		t.Fatal("expected nil from empty queue")
	}
}

func TestSessionMetadataNotFound(t *testing.T) {
	store, _, _ := setup(t)
	ctx := context.Background()

	_, err := GetSessionMetadata(ctx, store, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}
