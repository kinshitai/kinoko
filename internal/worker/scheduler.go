package worker

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/mycelium-dev/mycelium/internal/decay"
)

// Scheduler manages periodic background tasks.
type Scheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// scheduler implements Scheduler.
type scheduler struct {
	queue      SessionQueue
	pool       Pool
	decay      *decay.Runner
	libraryIDs []string
	cfg        SchedulerConfig
	log        *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup

	// nowFunc and nextDailyFunc are injectable for testing.
	nowFunc       func() time.Time
	nextDailyFunc func(now time.Time, hour, minute int) time.Duration
}

// NewScheduler creates a new Scheduler.
func NewScheduler(
	queue SessionQueue,
	pool Pool,
	decayRunner *decay.Runner,
	libraryIDs []string,
	cfg SchedulerConfig,
	log *slog.Logger,
) Scheduler {
	return &scheduler{
		queue:         queue,
		pool:          pool,
		decay:         decayRunner,
		libraryIDs:    libraryIDs,
		cfg:           cfg,
		log:           log,
		nowFunc:       time.Now,
		nextDailyFunc: nextDailyDelay,
	}
}

func (s *scheduler) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(3)
	go s.runDecay(ctx)
	go s.runStaleSweep(ctx)
	go s.runStatsLogger(ctx)

	s.log.Info("scheduler started",
		"decay_cron", s.cfg.DecayCron,
		"stale_sweep_interval", s.cfg.StaleSweepInterval,
		"stats_interval", s.cfg.StatsInterval,
	)
	return nil
}

func (s *scheduler) Stop(ctx context.Context) error {
	s.log.Info("scheduler stopping")
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.log.Info("scheduler stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("scheduler stop timed out: %w", ctx.Err())
	}
}

// runDecay schedules decay cycles based on the cron expression.
func (s *scheduler) runDecay(ctx context.Context) {
	defer s.wg.Done()

	hour, minute, ok := parseDailyCron(s.cfg.DecayCron)
	if !ok {
		s.log.Warn("unrecognized cron expression, falling back to 24h ticker", "cron", s.cfg.DecayCron)
	}

	for {
		var delay time.Duration
		if ok {
			delay = s.nextDailyFunc(s.nowFunc(), hour, minute)
		} else {
			delay = 24 * time.Hour
		}

		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}

		s.executeDecay(ctx)
	}
}

func (s *scheduler) executeDecay(ctx context.Context) {
	for _, libID := range s.libraryIDs {
		if ctx.Err() != nil {
			return
		}
		result, err := s.decay.RunCycle(ctx, libID)
		if err != nil {
			s.log.Error("decay cycle failed", "library_id", libID, "error", err)
			continue
		}
		s.log.Info("decay cycle completed",
			"library_id", libID,
			"processed", result.Processed,
			"demoted", result.Demoted,
			"deprecated", result.Deprecated,
		)
	}
}

// runStaleSweep periodically requeues stale claimed sessions.
func (s *scheduler) runStaleSweep(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.StaleSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.queue.RequeueStale(ctx, DefaultConfig().StaleClaimTimeout)
			if err != nil {
				s.log.Error("stale sweep failed", "error", err)
				continue
			}
			if n > 0 {
				s.log.Info("stale sessions requeued", "count", n)
			}
		}
	}
}

// runStatsLogger periodically logs pool and queue stats.
func (s *scheduler) runStatsLogger(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.cfg.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := s.pool.Stats()
			depth, err := s.queue.Depth(ctx)
			if err != nil {
				s.log.Error("stats: queue depth failed", "error", err)
				depth = -1
			}
			s.log.Info("worker stats",
				"active_workers", stats.ActiveWorkers,
				"idle_workers", stats.IdleWorkers,
				"total_processed", stats.TotalProcessed,
				"total_extracted", stats.TotalExtracted,
				"total_rejected", stats.TotalRejected,
				"total_errors", stats.TotalErrors,
				"total_failed", stats.TotalFailed,
				"queue_depth", depth,
			)
		}
	}
}

// cronPattern matches "M H * * *" daily cron expressions.
var cronPattern = regexp.MustCompile(`^\s*(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*\s*$`)

// parseDailyCron parses a "M H * * *" cron expression.
// Returns hour, minute, and whether parsing succeeded.
func parseDailyCron(expr string) (hour, minute int, ok bool) {
	m := cronPattern.FindStringSubmatch(expr)
	if m == nil {
		return 0, 0, false
	}
	minute, _ = strconv.Atoi(m[1])
	hour, _ = strconv.Atoi(m[2])
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}

// nextDailyDelay computes the duration until the next occurrence of hour:minute UTC.
func nextDailyDelay(now time.Time, hour, minute int) time.Duration {
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}
