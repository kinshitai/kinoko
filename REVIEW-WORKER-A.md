# Phase A — Queue Layer Review

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade:** B-

Solid foundation, but several issues need fixing before this goes to Phase B. The happy path works; the edge cases and concurrency corners have gaps.

---

## Critical Issues

### 1. Claim atomicity is not guaranteed under concurrent writers (CRITICAL)

The `UPDATE ... WHERE id = (SELECT ...)` pattern in `Claim()` is **not atomic across connections** in SQLite. Two concurrent `Claim()` calls can both execute the inner `SELECT`, get the same row ID, and then one `UPDATE` silently affects zero rows while still returning no error — the `RETURNING` clause just returns nothing and you treat it as "empty queue" instead of "lost race, retry."

SQLite's WAL mode serializes *writers*, so with a single `*sql.DB` and default `SQLITE_BUSY` handling this likely works in practice because only one write transaction executes at a time. But `database/sql` connection pooling means two goroutines can open separate connections. The `busy_timeout=5000` saves you — one will block — but this is **relying on implicit locking behavior, not explicit atomicity**.

Either:
- Wrap the claim in an `IMMEDIATE` transaction (`BEGIN IMMEDIATE; UPDATE ... RETURNING ...; COMMIT;`), or
- Document clearly that this depends on SQLite's single-writer guarantee and `busy_timeout`.

The test `TestClaimAtomic` passes because of this implicit serialization, not because the SQL itself is atomic. Fragile.

### 2. Enqueue has a TOCTOU race on backpressure (HIGH)

```go
depth, err := q.Depth(ctx)  // read count
// ... another goroutine enqueues here ...
if depth >= q.cfg.QueueDepthCritical {
    return ErrBackpressure
}
// ... insert proceeds
```

`Depth()` and the `INSERT` are not in the same transaction. Under concurrent enqueues, you can blow past the critical threshold. With a critical limit of 10,000 this is unlikely to matter in practice, but it's still a design flaw. At minimum, document it. Better: check-and-insert in a single transaction.

### 3. Log file written but no cleanup on **Enqueue backpressure path** (HIGH)

If `Depth()` succeeds, you write the log file to disk, *then* check backpressure... wait, no — you check backpressure first. Good. But re-read the code:

```go
depth, err := q.Depth(ctx)  // ✓ check first
if depth >= q.cfg.QueueDepthCritical { return ErrBackpressure }  // ✓ reject before file write
// write file
// insert DB
// cleanup on DB failure ✓
```

Actually this ordering is correct. The file is only written after the backpressure check passes. And there's cleanup on DB insert failure. Fine — withdrawing this one. But see issue #5.

---

## Medium Issues

### 4. `Fail()` does two queries non-atomically (MEDIUM)

```go
err := db.QueryRowContext(ctx, `SELECT retry_count FROM sessions WHERE id = ?`, sessionID).Scan(&retryCount)
// ... compute backoff ...
_, err = db.ExecContext(ctx, `UPDATE sessions SET ... retry_count = retry_count + 1 ...`)
```

The `SELECT` and `UPDATE` are separate statements. Another caller could `Fail()` the same session between them, incrementing `retry_count` twice but computing backoff based on the stale count. Unlikely (why would two workers fail the same session?), but wrap in a transaction or compute backoff in SQL:

```sql
UPDATE sessions SET
    retry_count = retry_count + 1,
    next_retry_at = datetime('now', '+' || (? * (1 << retry_count)) || ' seconds')
WHERE id = ?
```

### 5. No log file cleanup on `Complete` or `FailPermanent` (MEDIUM)

After extraction finishes (success or permanent failure), the log file at `log_content_path` sits on disk forever. The spec says nothing about cleanup, but you're writing potentially large session logs into `queue/`. Over time this is a disk leak. Add cleanup to `Complete()` and `FailPermanent()`, or document that a separate janitor handles it.

### 6. Schema migration silently swallows ALL errors, not just "duplicate column" (MEDIUM)

```go
_, _ = db.Exec(col.ddl) // ignore "duplicate column" errors
```

This ignores *every* error: disk full, corrupted DB, permission denied. Check whether the error contains "duplicate column" before ignoring:

```go
if _, err := db.Exec(col.ddl); err != nil {
    if !strings.Contains(err.Error(), "duplicate column") {
        return fmt.Errorf("migrate %s: %w", col.name, err)
    }
}
```

### 7. `Complete()` clears `claimed_by`/`claimed_at` — loses audit trail (MEDIUM)

Setting `claimed_by = ''` and `claimed_at = NULL` on completion erases which worker processed the session. That's useful debugging info. Keep it, or add a separate `completed_by`/`completed_at` column.

---

## Low Issues

### 8. `extraction_status` default in schema.sql is `'pending'`, not `'queued'` (LOW)

```sql
extraction_status TEXT NOT NULL DEFAULT 'pending',
```

The spec introduces `StatusQueued` as the initial state for the worker pipeline. Existing code (non-worker path via `InsertSession`) uses `'pending'` as the default. This is technically correct for backward compat, but confusing — the word "pending" now means two different things depending on the code path. Consider whether `InsertSession` (the old sync path) should even exist alongside the queue.

### 9. `Depth()` only counts `'queued'`, not `'error'` sessions awaiting retry (LOW)

Spec says `Depth()` returns "count of StatusQueued sessions." But for backpressure purposes, `error` sessions with `next_retry_at` in the past are *also* pending work. The backpressure check undercounts actual queue load. Probably fine at default thresholds, but worth a comment.

### 10. FIFO test is timing-dependent (LOW)

`TestFIFOOrdering` enqueues three sessions rapidly. They'll likely get the same `created_at` (SQLite `CURRENT_TIMESTAMP` has second granularity). FIFO ordering relies on `ORDER BY created_at ASC` — if two rows share the same timestamp, ordering is undefined. The test passes by luck (insertion order within same second). Add a tiebreaker: `ORDER BY created_at ASC, rowid ASC`.

### 11. `err.Error() == "sql: no rows in result set"` is string comparison (LOW)

```go
if err.Error() == "sql: no rows in result set" {
```

Use `errors.Is(err, sql.ErrNoRows)` like everywhere else in the codebase. The string comparison works today but is fragile.

### 12. No SQL injection risk (GOOD)

All queries use parameterized `?` placeholders. No string interpolation in SQL. Clean.

### 13. Missing index tiebreaker in Claim query (LOW)

The `idx_sessions_queue` index covers `(extraction_status, next_retry_at)` but the `ORDER BY created_at ASC` in the Claim subselect isn't covered. For large tables this means a filesort on every claim. Add `created_at` to the index or accept the perf hit.

---

## Test Coverage Assessment

Tests cover: enqueue+claim, empty queue, concurrent claim, fail+retry, permanent fail, depth, backpressure, stale requeue, FIFO, complete. That's good breadth.

**Missing tests:**
- Enqueue with duplicate session ID (what happens? silent overwrite? error?)
- `Fail()` when session doesn't exist
- `Complete()` with rejected result (stage 1/2/3 rejection paths)
- Log file content verification after `Complete` 
- Concurrent enqueue backpressure (race the threshold)

---

## Summary

The queue layer is structurally sound and follows the spec closely. Main concerns: claim atomicity relies on implicit SQLite behavior rather than explicit transactions, `Fail()` has a non-atomic read-modify-write, and the migration error handling is too permissive. Fix #1, #4, #6, and #11 before merging. The rest are worth tracking.
