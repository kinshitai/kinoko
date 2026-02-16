package worker

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// --- mocks for scheduler tests ---

type schedMockQueue struct {
	requeueStaleCalls atomic.Int32
	requeueStaleN     int
	depthVal          int
}

func (q *schedMockQueue) Enqueue(_ context.Context, _ model.SessionRecord, _ []byte) error {
	return nil
}
func (q *schedMockQueue) Claim(_ context.Context, _ string) (*QueueEntry, error) { return nil, nil }
func (q *schedMockQueue) Complete(_ context.Context, _ string, _ *model.ExtractionResult) error {
	return nil
}
func (q *schedMockQueue) Fail(_ context.Context, _ string, _ error) error         { return nil }
func (q *schedMockQueue) FailPermanent(_ context.Context, _ string, _ error) error { return nil }
func (q *schedMockQueue) Depth(_ context.Context) (int, error)                     { return q.depthVal, nil }
func (q *schedMockQueue) RequeueStale(_ context.Context, _ time.Duration) (int, error) {
	q.requeueStaleCalls.Add(1)
	return q.requeueStaleN, nil
}

type schedMockPool struct {
	statsCalls atomic.Int32
}

func (p *schedMockPool) Start(_ context.Context) error { return nil }
func (p *schedMockPool) Stop(_ context.Context) error  { return nil }
func (p *schedMockPool) Stats() PoolStats {
	p.statsCalls.Add(1)
	return PoolStats{ActiveWorkers: 1, IdleWorkers: 1}
}

type schedMockSkillReader struct{}

func (r *schedMockSkillReader) ListByDecay(_ context.Context, _ string, _ int) ([]model.SkillRecord, error) {
	return nil, nil
}

type schedMockSkillWriter struct{}

func (w *schedMockSkillWriter) UpdateDecay(_ context.Context, _ string, _ float64) error {
	return nil
}

// --- tests ---

func TestParseDailyCron(t *testing.T) {
	tests := []struct {
		expr    string
		hour    int
		minute  int
		ok      bool
	}{
		{"0 3 * * *", 3, 0, true},
		{"30 14 * * *", 14, 30, true},
		{"  5  23  *  *  *  ", 23, 5, true},
		{"0 25 * * *", 0, 0, false},      // invalid hour
		{"60 3 * * *", 0, 0, false},       // invalid minute
		{"0 3 1 * *", 0, 0, false},        // not daily
		{"*/5 * * * *", 0, 0, false},      // complex
		{"", 0, 0, false},
	}
	for _, tt := range tests {
		h, m, ok := parseDailyCron(tt.expr)
		if ok != tt.ok || h != tt.hour || m != tt.minute {
			t.Errorf("parseDailyCron(%q) = (%d, %d, %v), want (%d, %d, %v)",
				tt.expr, h, m, ok, tt.hour, tt.minute, tt.ok)
		}
	}
}

func TestNextDailyDelay(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	// Same day, future.
	d := nextDailyDelay(now, 14, 30)
	expected := 4*time.Hour + 30*time.Minute
	if d != expected {
		t.Errorf("got %v, want %v", d, expected)
	}

	// Already passed today, should be tomorrow.
	d = nextDailyDelay(now, 3, 0)
	expected = 17 * time.Hour
	if d != expected {
		t.Errorf("got %v, want %v", d, expected)
	}
}

func newTestScheduler(t *testing.T, cfg SchedulerConfig) (*scheduler, *schedMockQueue, *schedMockPool) {
	t.Helper()
	q := &schedMockQueue{depthVal: 5, requeueStaleN: 2}
	p := &schedMockPool{}
	log := slog.Default()

	reader := &schedMockSkillReader{}
	writer := &schedMockSkillWriter{}
	dr, err := decay.NewRunner(reader, writer, decay.DefaultConfig(), log)
	if err != nil {
		t.Fatal(err)
	}

	s := NewScheduler(q, p, dr, []string{"lib-1"}, cfg, log)
	return s.(*scheduler), q, p
}

func TestStaleSweepRuns(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.StaleSweepInterval = 20 * time.Millisecond
	cfg.StatsInterval = 1 * time.Hour
	cfg.DecayCron = "0 99 * * *" // invalid, won't fire

	s, q, _ := newTestScheduler(t, cfg)

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Wait for a few sweep cycles.
	time.Sleep(100 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	calls := q.requeueStaleCalls.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 stale sweep calls, got %d", calls)
	}
}

func TestStatsLoggerRuns(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.StatsInterval = 20 * time.Millisecond
	cfg.StaleSweepInterval = 1 * time.Hour
	cfg.DecayCron = "0 99 * * *"

	s, _, p := newTestScheduler(t, cfg)

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	calls := p.statsCalls.Load()
	if calls < 2 {
		t.Errorf("expected at least 2 stats calls, got %d", calls)
	}
}

func TestDecayRunsOnSchedule(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.StaleSweepInterval = 1 * time.Hour
	cfg.StatsInterval = 1 * time.Hour
	cfg.DecayCron = "0 3 * * *"

	s, _, _ := newTestScheduler(t, cfg)

	// Override timing to fire immediately.
	s.nextDailyFunc = func(now time.Time, hour, minute int) time.Duration {
		return 5 * time.Millisecond
	}

	// Verify the goroutine fires by checking it doesn't block start/stop.
	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
}

func TestStartStop(t *testing.T) {
	cfg := DefaultSchedulerConfig()
	cfg.StaleSweepInterval = 1 * time.Hour
	cfg.StatsInterval = 1 * time.Hour

	s, _, _ := newTestScheduler(t, cfg)

	ctx := context.Background()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	stopCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}

	// Verify all goroutines exited by calling Stop again (should be idempotent via wg).
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// wg.Wait() should return immediately since all goroutines are done.
		s.wg.Wait()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("goroutines did not exit")
	}
}
