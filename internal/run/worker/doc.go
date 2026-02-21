// Package worker provides a background worker pool and periodic task scheduler
// for the Kinoko run agent. The Pool claims queued sessions and runs them through
// the extraction pipeline with configurable concurrency. The Scheduler manages
// recurring jobs such as skill decay sweeps on a cron-like interval.
package worker
