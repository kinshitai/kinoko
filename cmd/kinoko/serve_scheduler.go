package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

// decayScheduler runs periodic decay cycles inside kinoko serve.
type decayScheduler struct {
	runner    *decay.Runner
	libraries []string
	interval  time.Duration
	log       *slog.Logger
	cancel    context.CancelFunc
	done      chan struct{}
}

// newDecayScheduler creates a scheduler that runs decay on a cron.
// The store satisfies both decay.SkillReader and decay.SkillWriter.
func newDecayScheduler(store *storage.SQLiteStore, cfg *config.Config, logger *slog.Logger) (*decayScheduler, error) {
	decayCfg := decayConfigFromYAML(cfg.Decay)
	runner, err := decay.NewRunner(store, store, decayCfg, logger)
	if err != nil {
		return nil, err
	}

	interval := 6 * time.Hour
	if cfg.Decay.IntervalHours > 0 {
		interval = time.Duration(cfg.Decay.IntervalHours) * time.Hour
	}

	return &decayScheduler{
		runner:    runner,
		libraries: libraryIDs(cfg),
		interval:  interval,
		log:       logger,
		done:      make(chan struct{}),
	}, nil
}

// Start begins the periodic decay loop.
func (ds *decayScheduler) Start(ctx context.Context) {
	ctx, ds.cancel = context.WithCancel(ctx)
	go ds.loop(ctx)
}

// Stop cancels the loop and waits for it to finish.
func (ds *decayScheduler) Stop() {
	if ds.cancel != nil {
		ds.cancel()
	}
	<-ds.done
}

func (ds *decayScheduler) loop(ctx context.Context) {
	defer close(ds.done)

	// Run once at startup.
	ds.runAll(ctx)

	ticker := time.NewTicker(ds.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ds.runAll(ctx)
		}
	}
}

func (ds *decayScheduler) runAll(ctx context.Context) {
	for _, libID := range ds.libraries {
		if ctx.Err() != nil {
			return
		}
		result, err := ds.runner.RunCycle(ctx, libID)
		if err != nil {
			ds.log.Error("decay cycle failed", "library", libID, "error", err)
			continue
		}
		ds.log.Info("decay cycle complete",
			"library", libID,
			"processed", result.Processed,
			"demoted", result.Demoted,
			"deprecated", result.Deprecated,
			"rescued", result.Rescued,
		)
	}
}
