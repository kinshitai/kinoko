// Package worker provides a concurrent worker pool that polls a session queue
// and runs the extraction pipeline. It handles claim-based concurrency, retries
// with exponential backoff, stale claim recovery, and queue depth alerting.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// SessionGetter loads a full SessionRecord by ID.
type SessionGetter func(ctx context.Context, id string) (*model.SessionRecord, error)

// Pool manages a pool of worker goroutines that claim and process queued sessions.
type Pool interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Stats() PoolStats
}

// PoolStats tracks worker pool state and lifetime counters.
type PoolStats struct {
	ActiveWorkers  int   `json:"active_workers"`
	IdleWorkers    int   `json:"idle_workers"`
	TotalProcessed int64 `json:"total_processed"`
	TotalExtracted int64 `json:"total_extracted"`
	TotalRejected  int64 `json:"total_rejected"`
	TotalErrors    int64 `json:"total_errors"`
	TotalFailed    int64 `json:"total_failed"`
}

// workerPool implements Pool.
type workerPool struct {
	queue      SessionQueue
	extractor  model.Extractor
	getSession SessionGetter
	cfg        Config
	log        *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup

	activeWorkers atomic.Int32
	totalWorkers  int

	processed atomic.Int64
	extracted atomic.Int64
	rejected  atomic.Int64
	errors    atomic.Int64
	failed    atomic.Int64
}

// NewPool creates a new worker pool.
func NewPool(queue SessionQueue, extractor model.Extractor, getSession SessionGetter, cfg Config, log *slog.Logger) Pool {
	return &workerPool{
		queue:      queue,
		extractor:  extractor,
		getSession: getSession,
		cfg:        cfg,
		log:        log,
	}
}

func (p *workerPool) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)
	p.totalWorkers = p.cfg.Concurrency

	for i := 0; i < p.cfg.Concurrency; i++ {
		workerID := fmt.Sprintf("worker-%d", i)
		p.wg.Add(1)
		go p.run(ctx, workerID)
	}

	p.log.Info("worker pool started", "concurrency", p.cfg.Concurrency)
	return nil
}

func (p *workerPool) Stop(ctx context.Context) error {
	p.log.Info("worker pool stopping")
	p.cancel()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.log.Info("worker pool stopped gracefully")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker pool stop timed out: %w", ctx.Err())
	}
}

func (p *workerPool) Stats() PoolStats {
	active := int(p.activeWorkers.Load())
	return PoolStats{
		ActiveWorkers:  active,
		IdleWorkers:    p.totalWorkers - active,
		TotalProcessed: p.processed.Load(),
		TotalExtracted: p.extracted.Load(),
		TotalRejected:  p.rejected.Load(),
		TotalErrors:    p.errors.Load(),
		TotalFailed:    p.failed.Load(),
	}
}

func (p *workerPool) run(ctx context.Context, workerID string) {
	defer p.wg.Done()
	p.log.Info("worker started", "worker_id", workerID)
	defer p.log.Info("worker stopped", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entry, err := p.queue.Claim(ctx, workerID)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			p.log.Error("claim failed", "worker_id", workerID, "error", err)
			p.sleep(ctx)
			continue
		}

		if entry == nil {
			p.sleep(ctx)
			continue
		}

		p.log.Info("claimed session", "worker_id", workerID, "session_id", entry.SessionID)
		p.activeWorkers.Add(1)
		p.process(ctx, workerID, entry)
		p.activeWorkers.Add(-1)
		p.processed.Add(1)
	}
}

func (p *workerPool) process(_ context.Context, workerID string, entry *QueueEntry) {
	// Use a detached context for all DB operations during processing.
	// The pool context (ctx) may be cancelled during graceful shutdown,
	// but in-flight work must still be able to Complete/Fail/read sessions.
	processTimeout := p.cfg.ProcessTimeout
	if processTimeout <= 0 {
		processTimeout = 300 * time.Second
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), processTimeout)
	defer dbCancel()

	// Read log file from disk.
	content, err := os.ReadFile(entry.LogContentPath)
	if err != nil {
		p.log.Error("file read failed", "worker_id", workerID, "session_id", entry.SessionID, "path", entry.LogContentPath, "error", err)
		if failErr := p.queue.FailPermanent(dbCtx, entry.SessionID, fmt.Errorf("read log file: %w", err)); failErr != nil {
			p.log.Error("fail permanent error", "worker_id", workerID, "session_id", entry.SessionID, "error", failErr)
		}
		p.failed.Add(1)
		return
	}

	// Load full session record. Failures are transient (DB/network may recover).
	session, err := p.getSession(dbCtx, entry.SessionID)
	if err != nil {
		p.log.Error("get session failed", "worker_id", workerID, "session_id", entry.SessionID, "error", err)
		if failErr := p.queue.Fail(dbCtx, entry.SessionID, fmt.Errorf("get session: %w", err)); failErr != nil {
			p.log.Error("fail error", "worker_id", workerID, "session_id", entry.SessionID, "error", failErr)
		}
		p.errors.Add(1)
		return
	}

	// Run extraction pipeline with the detached context so that pool
	// cancellation does not abort in-flight extractions.
	result, err := p.extractor.Extract(dbCtx, *session, content)
	if err != nil {
		p.log.Error("extraction failed", "worker_id", workerID, "session_id", entry.SessionID, "error", err)
		// Use a fresh context for failure recording — dbCtx may be the one that timed out.
		failCtx, failCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer failCancel()
		if entry.RetryCount+1 >= p.cfg.MaxRetries {
			if failErr := p.queue.FailPermanent(failCtx, entry.SessionID, err); failErr != nil {
				p.log.Error("fail permanent error", "worker_id", workerID, "session_id", entry.SessionID, "error", failErr)
			}
			p.failed.Add(1)
		} else {
			if failErr := p.queue.Fail(failCtx, entry.SessionID, err); failErr != nil {
				p.log.Error("fail error", "worker_id", workerID, "session_id", entry.SessionID, "error", failErr)
			}
			p.errors.Add(1)
		}
		return
	}

	// Complete.
	if completeErr := p.queue.Complete(dbCtx, entry.SessionID, result); completeErr != nil {
		p.log.Error("complete failed", "worker_id", workerID, "session_id", entry.SessionID, "error", completeErr)
		p.errors.Add(1)
		return
	}

	if result.Status == model.StatusExtracted {
		p.extracted.Add(1)
		p.log.Info("session extracted", "worker_id", workerID, "session_id", entry.SessionID)
	} else {
		p.rejected.Add(1)
		p.log.Info("session rejected", "worker_id", workerID, "session_id", entry.SessionID)
	}
}

func (p *workerPool) sleep(ctx context.Context) {
	t := time.NewTimer(p.cfg.PollInterval)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
