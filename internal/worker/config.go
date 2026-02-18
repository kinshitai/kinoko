// Package worker provides a concurrent worker pool and cron scheduler for
// processing queued extraction sessions. The pool claims sessions from a
// queue, runs the extraction pipeline, and records results. The scheduler
// orchestrates periodic decay cycles and stale-session sweeps.
package worker

import "time"

// Config holds worker pool settings.
type Config struct {
	Concurrency        int           `yaml:"concurrency"`
	PollInterval       time.Duration `yaml:"poll_interval"`
	MaxRetries         int           `yaml:"max_retries"`
	InitialBackoff     time.Duration `yaml:"initial_backoff"`
	MaxBackoff         time.Duration `yaml:"max_backoff"`
	QueueDepthWarning  int           `yaml:"queue_depth_warning"`
	QueueDepthCritical int           `yaml:"queue_depth_critical"`
	StaleClaimTimeout  time.Duration `yaml:"stale_claim_timeout"`
	ProcessTimeout     time.Duration `yaml:"process_timeout"` // timeout for each extraction; 0 = 300s default
}

// DefaultConfig returns Config with spec defaults.
func DefaultConfig() Config {
	return Config{
		Concurrency:        2,
		PollInterval:       5 * time.Second,
		MaxRetries:         3,
		InitialBackoff:     30 * time.Second,
		MaxBackoff:         30 * time.Minute,
		QueueDepthWarning:  100,
		QueueDepthCritical: 10000,
		StaleClaimTimeout:  10 * time.Minute,
		ProcessTimeout:     300 * time.Second,
	}
}

// SchedulerConfig holds scheduled task settings.
type SchedulerConfig struct {
	DecayCron          string        `yaml:"decay_cron"`
	RetrySweepInterval time.Duration `yaml:"retry_sweep_interval"`
	StatsInterval      time.Duration `yaml:"stats_interval"`
	StaleSweepInterval time.Duration `yaml:"stale_sweep_interval"`
	StaleClaimTimeout  time.Duration `yaml:"stale_claim_timeout"`
}

// DefaultSchedulerConfig returns SchedulerConfig with spec defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		DecayCron:          "0 3 * * *",
		RetrySweepInterval: 5 * time.Minute,
		StatsInterval:      1 * time.Hour,
		StaleSweepInterval: 2 * time.Minute,
		StaleClaimTimeout:  DefaultConfig().StaleClaimTimeout,
	}
}
