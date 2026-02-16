package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

func TestEnqueue_Backpressure(t *testing.T) {
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test-bp")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := DefaultConfig()
	cfg.QueueDepthCritical = 2
	cfg.QueueDepthWarning = 1

	q := NewSQLiteQueue(store, t.TempDir(), cfg, slog.Default())

	ctx := context.Background()
	now := time.Now().UTC()
	s := model.SessionRecord{
		ID:        "bp-1",
		StartedAt: now, EndedAt: now,
		LibraryID: "test",
	}

	// Fill to critical
	if err := q.Enqueue(ctx, s, []byte("log1")); err != nil {
		t.Fatal(err)
	}
	s.ID = "bp-2"
	if err := q.Enqueue(ctx, s, []byte("log2")); err != nil {
		t.Fatal(err)
	}

	// Third should hit backpressure
	s.ID = "bp-3"
	err = q.Enqueue(ctx, s, []byte("log3"))
	if err == nil {
		t.Fatal("expected backpressure error")
	}
}

func TestComplete_Extracted(t *testing.T) {
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test-comp")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := DefaultConfig()
	q := NewSQLiteQueue(store, t.TempDir(), cfg, slog.Default())
	ctx := context.Background()

	now := time.Now().UTC()
	s := model.SessionRecord{ID: "comp-1", StartedAt: now, EndedAt: now, LibraryID: "test"}
	q.Enqueue(ctx, s, []byte("log"))

	// Complete with extracted status
	result := &model.ExtractionResult{
		SessionID: "comp-1",
		Status:    model.StatusExtracted,
		Skill:     &model.SkillRecord{ID: "skill-1"},
	}
	if err := q.Complete(ctx, "comp-1", result); err != nil {
		t.Fatal(err)
	}
}

func TestComplete_Rejected_Stage1(t *testing.T) {
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test-rej")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := DefaultConfig()
	q := NewSQLiteQueue(store, t.TempDir(), cfg, slog.Default())
	ctx := context.Background()

	now := time.Now().UTC()
	s := model.SessionRecord{ID: "rej-1", StartedAt: now, EndedAt: now, LibraryID: "test"}
	q.Enqueue(ctx, s, []byte("log"))

	result := &model.ExtractionResult{
		SessionID: "rej-1",
		Status:    model.StatusRejected,
		Stage1:    &model.Stage1Result{Passed: false, Reason: "too short"},
	}
	if err := q.Complete(ctx, "rej-1", result); err != nil {
		t.Fatal(err)
	}
}

func TestComplete_Rejected_Stage2(t *testing.T) {
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test-rej2")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := DefaultConfig()
	q := NewSQLiteQueue(store, t.TempDir(), cfg, slog.Default())
	ctx := context.Background()

	now := time.Now().UTC()
	s := model.SessionRecord{ID: "rej2-1", StartedAt: now, EndedAt: now, LibraryID: "test"}
	q.Enqueue(ctx, s, []byte("log"))

	result := &model.ExtractionResult{
		SessionID: "rej2-1",
		Status:    model.StatusRejected,
		Stage1:    &model.Stage1Result{Passed: true},
		Stage2:    &model.Stage2Result{Passed: false, Reason: "not novel"},
	}
	if err := q.Complete(ctx, "rej2-1", result); err != nil {
		t.Fatal(err)
	}
}

func TestComplete_Rejected_Stage3(t *testing.T) {
	store, err := storage.NewSQLiteStore("file::memory:?cache=shared&_txlock=immediate", "test-rej3")
	if err != nil {
		t.Fatal(err)
	}
	store.DB().Exec("PRAGMA foreign_keys=OFF")
	t.Cleanup(func() { store.Close() })

	cfg := DefaultConfig()
	q := NewSQLiteQueue(store, t.TempDir(), cfg, slog.Default())
	ctx := context.Background()

	now := time.Now().UTC()
	s := model.SessionRecord{ID: "rej3-1", StartedAt: now, EndedAt: now, LibraryID: "test"}
	q.Enqueue(ctx, s, []byte("log"))

	result := &model.ExtractionResult{
		SessionID: "rej3-1",
		Status:    model.StatusRejected,
		Stage1:    &model.Stage1Result{Passed: true},
		Stage2:    &model.Stage2Result{Passed: true},
		Stage3:    &model.Stage3Result{Passed: false, CriticReasoning: "not useful"},
	}
	if err := q.Complete(ctx, "rej3-1", result); err != nil {
		t.Fatal(err)
	}
}
