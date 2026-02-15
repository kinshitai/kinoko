package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/model"
)

// --- Mocks ---

type mockQueue struct {
	mu       sync.Mutex
	entries  []*QueueEntry
	results  map[string]*model.ExtractionResult
	failures map[string]struct{ err error; permanent bool }
	claimCh  chan struct{} // signaled on each claim attempt
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		results:  make(map[string]*model.ExtractionResult),
		failures: make(map[string]struct{ err error; permanent bool }),
		claimCh:  make(chan struct{}, 100),
	}
}

func (q *mockQueue) Enqueue(_ context.Context, _ model.SessionRecord, _ []byte) error {
	return nil
}

func (q *mockQueue) Claim(_ context.Context, _ string) (*QueueEntry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	defer func() { q.claimCh <- struct{}{} }()
	if len(q.entries) == 0 {
		return nil, nil
	}
	e := q.entries[0]
	q.entries = q.entries[1:]
	return e, nil
}

func (q *mockQueue) Complete(_ context.Context, sessionID string, result *model.ExtractionResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.results[sessionID] = result
	return nil
}

func (q *mockQueue) Fail(_ context.Context, sessionID string, err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failures[sessionID] = struct{ err error; permanent bool }{err, false}
	return nil
}

func (q *mockQueue) FailPermanent(_ context.Context, sessionID string, err error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failures[sessionID] = struct{ err error; permanent bool }{err, true}
	return nil
}

func (q *mockQueue) Depth(_ context.Context) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.entries), nil
}

func (q *mockQueue) RequeueStale(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}

func (q *mockQueue) addEntry(id, logPath string, retryCount int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.entries = append(q.entries, &QueueEntry{
		SessionID:      id,
		LogContentPath: logPath,
		RetryCount:     retryCount,
		LibraryID:      "lib-1",
	})
}

type mockExtractor struct {
	fn func(ctx context.Context, session model.SessionRecord, content []byte) (*model.ExtractionResult, error)
}

func (m *mockExtractor) Extract(ctx context.Context, session model.SessionRecord, content []byte) (*model.ExtractionResult, error) {
	return m.fn(ctx, session, content)
}

func testConfig() Config {
	return Config{
		Concurrency:        2,
		PollInterval:       10 * time.Millisecond,
		MaxRetries:         3,
		InitialBackoff:     time.Second,
		MaxBackoff:         time.Minute,
		QueueDepthWarning:  100,
		QueueDepthCritical: 10000,
		StaleClaimTimeout:  time.Minute,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func dummySessionGetter(_ context.Context, id string) (*model.SessionRecord, error) {
	return &model.SessionRecord{ID: id, LibraryID: "lib-1"}, nil
}

func writeLogFile(t *testing.T, dir, id, content string) string {
	t.Helper()
	path := filepath.Join(dir, id+".log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- Tests ---

func TestPool_StartStop(t *testing.T) {
	q := newMockQueue()
	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return nil, nil
	}}

	p := NewPool(q, ext, dummySessionGetter, testConfig(), testLogger())
	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}

	stats := p.Stats()
	if stats.IdleWorkers+stats.ActiveWorkers != 2 {
		t.Fatalf("expected 2 total workers, got active=%d idle=%d", stats.ActiveWorkers, stats.IdleWorkers)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := p.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestPool_ProcessSessions(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()

	path1 := writeLogFile(t, dir, "s1", "log content 1")
	path2 := writeLogFile(t, dir, "s2", "log content 2")
	q.addEntry("s1", path1, 0)
	q.addEntry("s2", path2, 0)

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return &model.ExtractionResult{Status: model.StatusExtracted}, nil
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait for processing.
	deadline := time.After(2 * time.Second)
	for {
		stats := p.Stats()
		if stats.TotalProcessed >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for processing, stats=%+v", p.Stats())
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	stats := p.Stats()
	if stats.TotalProcessed != 2 {
		t.Errorf("expected 2 processed, got %d", stats.TotalProcessed)
	}
	if stats.TotalExtracted != 2 {
		t.Errorf("expected 2 extracted, got %d", stats.TotalExtracted)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.results) != 2 {
		t.Errorf("expected 2 results, got %d", len(q.results))
	}
}

func TestPool_RetryOnPipelineError(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()
	path := writeLogFile(t, dir, "s1", "log")
	q.addEntry("s1", path, 0) // retry_count=0, under max

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return nil, errors.New("llm down")
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		stats := p.Stats()
		if stats.TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	q.mu.Lock()
	f, ok := q.failures["s1"]
	q.mu.Unlock()
	if !ok {
		t.Fatal("expected failure for s1")
	}
	if f.permanent {
		t.Error("expected non-permanent failure (retry)")
	}

	stats := p.Stats()
	if stats.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", stats.TotalErrors)
	}
}

func TestPool_FailPermanentAfterMaxRetries(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()
	path := writeLogFile(t, dir, "s1", "log")
	q.addEntry("s1", path, 2) // retry_count=2, max_retries=3 → 2+1 >= 3 → permanent

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return nil, errors.New("still broken")
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if p.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	q.mu.Lock()
	f, ok := q.failures["s1"]
	q.mu.Unlock()
	if !ok {
		t.Fatal("expected failure for s1")
	}
	if !f.permanent {
		t.Error("expected permanent failure")
	}

	if p.Stats().TotalFailed != 1 {
		t.Errorf("expected 1 failed, got %d", p.Stats().TotalFailed)
	}
}

func TestPool_FileReadFailIsPermanent(t *testing.T) {
	q := newMockQueue()
	q.addEntry("s1", "/nonexistent/path.log", 0)

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		t.Error("extractor should not be called")
		return nil, nil
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if p.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	q.mu.Lock()
	f, ok := q.failures["s1"]
	q.mu.Unlock()
	if !ok {
		t.Fatal("expected failure")
	}
	if !f.permanent {
		t.Error("file read failure should be permanent")
	}
}

func TestPool_GracefulShutdown_InFlightCompletes(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()
	path := writeLogFile(t, dir, "s1", "log")
	q.addEntry("s1", path, 0)

	extractStarted := make(chan struct{})
	extractContinue := make(chan struct{})

	ext := &mockExtractor{fn: func(ctx context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		close(extractStarted)
		// Extract receives a detached context, so ctx.Done() should NOT fire
		// when the pool is stopped. We wait on extractContinue only.
		<-extractContinue
		// Verify the extract context is still valid (not cancelled by pool shutdown).
		if ctx.Err() != nil {
			return nil, fmt.Errorf("extract context should not be cancelled: %w", ctx.Err())
		}
		return &model.ExtractionResult{Status: model.StatusExtracted}, nil
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	<-extractStarted // Worker is in Extract

	// Stop should wait for in-flight.
	stopDone := make(chan error, 1)
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go func() { stopDone <- p.Stop(stopCtx) }()

	// Let extraction finish.
	time.Sleep(50 * time.Millisecond)
	close(extractContinue)

	if err := <-stopDone; err != nil {
		t.Fatalf("stop error: %v", err)
	}

	q.mu.Lock()
	_, ok := q.results["s1"]
	q.mu.Unlock()
	if !ok {
		t.Error("in-flight session should have completed")
	}
}

func TestPool_StatsCountRejected(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()
	path := writeLogFile(t, dir, "s1", "log")
	q.addEntry("s1", path, 0)

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return &model.ExtractionResult{
			Status: model.StatusRejected,
			Stage1: &model.Stage1Result{Passed: false, Reason: "too short"},
		}, nil
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if p.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	stats := p.Stats()
	if stats.TotalRejected != 1 {
		t.Errorf("expected 1 rejected, got %d", stats.TotalRejected)
	}
}

func TestPool_GetSessionFailureIsTransient(t *testing.T) {
	dir := t.TempDir()
	q := newMockQueue()
	path := writeLogFile(t, dir, "s1", "log")
	q.addEntry("s1", path, 0)

	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		t.Error("extractor should not be called when getSession fails")
		return nil, nil
	}}

	failingSessionGetter := func(_ context.Context, _ string) (*model.SessionRecord, error) {
		return nil, errors.New("db connection refused")
	}

	cfg := testConfig()
	cfg.Concurrency = 1
	p := NewPool(q, ext, failingSessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	deadline := time.After(2 * time.Second)
	for {
		if p.Stats().TotalProcessed >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	q.mu.Lock()
	f, ok := q.failures["s1"]
	q.mu.Unlock()
	if !ok {
		t.Fatal("expected failure for s1")
	}
	if f.permanent {
		t.Error("getSession failure should be transient (non-permanent)")
	}

	stats := p.Stats()
	if stats.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", stats.TotalErrors)
	}
}

func TestPool_EmptyQueue_NoSpin(t *testing.T) {
	q := newMockQueue()
	ext := &mockExtractor{fn: func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return nil, nil
	}}

	cfg := testConfig()
	cfg.Concurrency = 1
	cfg.PollInterval = 50 * time.Millisecond

	p := NewPool(q, ext, dummySessionGetter, cfg, testLogger())

	ctx := context.Background()
	p.Start(ctx)

	// Count claim attempts over 200ms. With 50ms poll, should be ~4-5, not hundreds.
	var claimCount atomic.Int32
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-q.claimCh:
				claimCount.Add(1)
			case <-done:
				return
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(done)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	p.Stop(stopCtx)

	count := claimCount.Load()
	// With 50ms poll and 200ms window: expect roughly 4-5 claims. Allow up to 10.
	if count > 10 {
		t.Errorf("too many claim attempts (%d), workers may be spinning", count)
	}
	fmt.Printf("claim attempts in 200ms: %d\n", count)
}
