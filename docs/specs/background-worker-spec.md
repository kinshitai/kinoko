# Background Worker Spec

**Authors:** Hal (CTO), Luka Jensen  
**Date:** 2026-02-15  
**Status:** Ready for implementation  
**Module:** `github.com/mycelium-dev/mycelium`

---

## Overview

Decouple session ingestion from extraction processing. The session-end hook becomes a fast write (<10ms). A background worker pool processes pending sessions asynchronously.

```
OnSessionEnd → queue.Enqueue()  → return           [<10ms]
Worker pool  → queue.Claim()    → pipeline.Extract() [async, 2-10s]
```

---

## 1. Data Model Changes

### Session Statuses

```go
const (
    StatusQueued    ExtractionStatus = "queued"     // ingested, awaiting worker pickup
    StatusPending   ExtractionStatus = "pending"    // worker claimed, processing in progress
    StatusExtracted ExtractionStatus = "extracted"  // skill created
    StatusRejected  ExtractionStatus = "rejected"   // rejected at stage 1/2/3
    StatusError     ExtractionStatus = "error"      // transient failure, will retry
    StatusFailed    ExtractionStatus = "failed"     // permanent failure, needs human attention
)
```

`StatusQueued` is new — distinguishes "in the queue" from "being processed." If a worker crashes mid-processing, `StatusPending` + stale timeout → requeue to `StatusQueued`.

### Schema Migration

```sql
ALTER TABLE sessions ADD COLUMN log_content_path TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN retry_count      INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sessions ADD COLUMN last_error       TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN next_retry_at    TIMESTAMP;
ALTER TABLE sessions ADD COLUMN claimed_by       TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN claimed_at       TIMESTAMP;

CREATE INDEX idx_sessions_queue ON sessions(extraction_status, next_retry_at);
```

---

## 2. Session Ingestion (Fast Path)

### Interface

```go
package worker

// SessionQueue manages the extraction work queue.
type SessionQueue interface {
    // Enqueue stores a session + log content for async processing.
    // Writes log to disk, inserts session with StatusQueued. Must complete in <10ms.
    Enqueue(ctx context.Context, session extraction.SessionRecord, logContent []byte) error

    // Claim atomically picks the next queued session.
    // Sets StatusPending, claimed_by, claimed_at. Returns nil if empty.
    Claim(ctx context.Context, workerID string) (*QueueEntry, error)

    // Complete marks a session as extracted/rejected.
    Complete(ctx context.Context, sessionID string, result *extraction.ExtractionResult) error

    // Fail marks as errored with retry scheduling.
    Fail(ctx context.Context, sessionID string, err error) error

    // FailPermanent marks as permanently failed (dead letter).
    FailPermanent(ctx context.Context, sessionID string, err error) error

    // Depth returns count of StatusQueued sessions.
    Depth(ctx context.Context) (int, error)

    // RequeueStale reclaims sessions stuck in StatusPending beyond timeout.
    RequeueStale(ctx context.Context, staleDuration time.Duration) (int, error)
}

type QueueEntry struct {
    SessionID      string
    LogContentPath string
    RetryCount     int
    LibraryID      string
}
```

### Claim Query (atomic under WAL)

```sql
UPDATE sessions
SET extraction_status = 'pending',
    claimed_by = ?,
    claimed_at = CURRENT_TIMESTAMP
WHERE id = (
    SELECT id FROM sessions
    WHERE (extraction_status = 'queued')
       OR (extraction_status = 'error' AND next_retry_at <= CURRENT_TIMESTAMP)
    ORDER BY created_at ASC
    LIMIT 1
)
RETURNING id, log_content_path, retry_count, library_id;
```

### Backpressure

Enqueue checks `Depth()` before writing. Warning at configurable threshold (default 100), hard reject at critical threshold (default 10000). No in-memory buffering — queue is DB rows.

### Hook Replacement in serve.go

```go
// Before (sync, 2-10s):
hooks.OnSessionEnd = func(...) { return pipeline.Extract(ctx, session, content) }

// After (async, <10ms):
hooks.OnSessionEnd = func(...) { return queue.Enqueue(ctx, session, content), nil }
```

---

## 3. Worker Pool

### Config

```go
type Config struct {
    Concurrency       int           `yaml:"concurrency"`         // default: 2
    PollInterval      time.Duration `yaml:"poll_interval"`       // default: 5s
    MaxRetries        int           `yaml:"max_retries"`         // default: 3
    InitialBackoff    time.Duration `yaml:"initial_backoff"`     // default: 30s
    MaxBackoff        time.Duration `yaml:"max_backoff"`         // default: 30m
    QueueDepthWarning int           `yaml:"queue_depth_warning"` // default: 100
    QueueDepthCritical int          `yaml:"queue_depth_critical"`// default: 10000
    StaleClaimTimeout time.Duration `yaml:"stale_claim_timeout"` // default: 10m
}
```

### Interface

```go
type Pool interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error   // waits for in-flight work, 30s timeout
    Stats() PoolStats
}

type PoolStats struct {
    ActiveWorkers  int
    IdleWorkers    int
    QueueDepth     int
    TotalProcessed int64
    TotalExtracted int64
    TotalRejected  int64
    TotalErrors    int64
    TotalFailed    int64
}
```

### Worker Loop (per goroutine)

```
loop:
    select ctx.Done → return
    entry = queue.Claim(workerID)
    if nil → sleep PollInterval → continue
    content = readFile(entry.LogContentPath)
    if read error → FailPermanent (file missing = not retryable)
    result = pipeline.Extract(ctx, session, content)
    if error → Fail (with retry) or FailPermanent (if max retries hit)
    if success → Complete
```

### Retry Schedule

`next_retry_at = now + initial_backoff × 2^(retry_count - 1)`, capped at `max_backoff`.

| Retry | Default delay |
|-------|--------------|
| 1st   | 30s          |
| 2nd   | 60s          |
| 3rd   | 120s         |
| After 3rd | StatusFailed |

### Graceful Shutdown

1. Cancel worker context → workers finish current extraction, don't claim new
2. `wg.Wait()` with 30s timeout
3. Sessions stuck in StatusPending are recovered by stale claim sweep on next startup

---

## 4. Scheduled Tasks

Run inside the worker process.

### Decay Scheduler
- Cron expression (default `"0 3 * * *"`)
- Calls existing `decay.Runner.RunCycle()` per library

### Stale Claim Sweep
- Every 2 minutes, requeue sessions in `StatusPending` where `claimed_at` is older than `StaleClaimTimeout`
- Protects against worker crashes

### Stats Logger
- Every hour, log: queue depth, status counts, extraction rate, active workers

### Config

```go
type SchedulerConfig struct {
    DecayCron          string        `yaml:"decay_cron"`           // default: "0 3 * * *"
    RetrySweepInterval time.Duration `yaml:"retry_sweep_interval"` // default: 5m
    StatsInterval      time.Duration `yaml:"stats_interval"`       // default: 1h
    StaleSweepInterval time.Duration `yaml:"stale_sweep_interval"` // default: 2m
}
```

---

## 5. CLI

### `mycelium serve`
Starts git server + worker pool + scheduler. Production mode.

### `mycelium worker`
Standalone worker pool + scheduler, no git server. For scaling workers separately.

### `mycelium import <path>`
Batch-imports session logs into queue. `--dir` for directories. Does NOT start workers — sessions wait for `serve` or `worker`.

### `mycelium queue`
Queue inspection: `queue stats`, `queue list`, `queue retry <id>`, `queue flush`.

---

## 6. Package Layout

```
internal/worker/
    config.go        // Config, SchedulerConfig, defaults
    queue.go         // SessionQueue interface + SQLiteQueue
    pool.go          // Pool + worker goroutines
    scheduler.go     // Decay, retry sweep, stale sweep, stats
    queue_test.go
    pool_test.go
    scheduler_test.go
cmd/mycelium/
    serve.go         // modified: starts pool + scheduler
    worker.go        // new: standalone mode
    import.go        // new: batch import
    queue.go         // new: inspection
```

---

## 7. Implementation Phases

### Phase A: Queue Layer
Schema migration + `SessionQueue` interface + SQLite implementation + unit tests. `Enqueue`, `Claim`, `Complete`, `Fail`, `FailPermanent`, `Depth`, `RequeueStale`.

### Phase B: Worker Pool
`internal/worker/pool.go` — goroutine pool with claim/process loop, retry scheduling, graceful shutdown. Mock queue + mock pipeline for tests.

### Phase C: Scheduler
Decay cron, stale claim sweep, retry sweep, stats logger. Wire into pool lifecycle.

### Phase D: CLI + Serve Integration
Replace sync hook with `Enqueue`. Start pool + scheduler in `serve`. Add `worker`, `import`, `queue` commands.

---

## 8. Failure Modes

| Failure | Impact | Recovery |
|---------|--------|----------|
| Worker crashes mid-extraction | Session stuck in StatusPending | Stale sweep requeues after 10m |
| LLM API down | Extract returns error | Retry with backoff (3 attempts) |
| Disk full | Enqueue fails | Hook returns error, session lost |
| SQLite locked | Claim/Complete retried via busy_timeout | Worker skips poll cycle if still locked |
| All workers busy | Queue grows | Warning at 100, reject at 10000 |
| Process killed (SIGKILL) | In-flight stuck | Stale sweep on next startup |

---

## 9. Integration Notes

- **Pipeline unchanged** — `extraction.Pipeline` stays synchronous. Workers just call `Extract()`.
- **Storage minimal changes** — add `GetSession(ctx, id)` method + schema migration for new columns.
- **Config extension** — add `worker` and `scheduler` sections to `config.Config`.
