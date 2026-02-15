# Review: Phase B — Worker Pool

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `internal/worker/pool.go`, `internal/worker/pool_test.go`  
**Grade: B+**

Solid work. The goroutine lifecycle is clean, stats are thread-safe via atomics, and the shutdown path is correct. But I found real issues.

---

## Issues

### 1. Context cancellation propagates into Extract — spec says finish current work (Medium)

**File:** `pool.go` — `run()` passes the pool's cancellable `ctx` directly to `p.extractor.Extract(ctx, ...)` (via `p.process(ctx, ...)`). When `Stop()` calls `cancel()`, that context is cancelled *immediately*, which means in-flight `Extract()` calls receive a cancelled context. The spec says: "Cancel worker context → workers finish current extraction, don't claim new."

The intent is that workers stop *claiming* on cancel, but in-flight extractions should complete. Right now the extraction sees `ctx.Done()` and may abort early depending on how `Extract` handles context. The graceful shutdown test even acknowledges this: `case <-ctx.Done(): // Context cancelled but we still complete.` — the mock ignores cancellation, but a real LLM client won't.

**Fix:** Use a separate context or `context.WithoutCancel()` for the extraction call, and only use the pool context for the claim loop and sleep.

### 2. `processed` counter incremented even on file-read/session-get failures (Low-Medium)

**File:** `pool.go:113` — `p.processed.Add(1)` is called unconditionally after `process()` returns, including when the file read fails at the top of `process()`. This inflates `TotalProcessed`. Whether that's intentional is debatable, but it means `TotalProcessed != TotalExtracted + TotalRejected + TotalErrors + TotalFailed` is NOT guaranteed — it can equal it, but only if every path increments exactly one of those counters.

Actually, looking closer: file read failure increments `failed`, extraction error increments `errors` or `failed`, success increments `extracted` or `rejected`. So the sum *does* add up. But "processed" counting file-read failures as processed work is misleading. A session where the file was missing wasn't really "processed."

### 3. No `QueueDepth` in `Stats()` (Low)

**File:** `pool.go` — `PoolStats` has `QueueDepth int` but `Stats()` never populates it. It's always 0. The spec's Stats Logger (section 4) says to log queue depth. You'd need a context to call `queue.Depth(ctx)`, which `Stats()` doesn't accept. Either add a context param or drop the field — a perpetually-zero field is worse than no field.

### 4. `getSession` failure is FailPermanent — is it always? (Medium)

**File:** `pool.go:131-137` — If `getSession` fails, it's treated as permanent. But `getSession` could fail transiently (DB locked, network timeout). Only `os.ReadFile` failures are definitively non-retryable (file won't magically appear). A DB error might resolve on retry.

**Fix:** Use `Fail()` for `getSession` errors unless max retries exceeded, same as extraction errors.

### 5. Shutdown test has a subtle race (Low)

**File:** `pool_test.go` — `TestPool_GracefulShutdown_InFlightCompletes`: after `<-extractStarted`, it calls `Stop()` in a goroutine, sleeps 50ms, then closes `extractContinue`. There's no guarantee `Stop()` has called `cancel()` before `extractContinue` is closed. If the scheduler is slow, extraction might complete *before* cancellation, making this test not actually test graceful shutdown under cancellation. It works in practice because of timing, but it's not deterministic.

### 6. Double-Stop is a panic (Low)

**File:** `pool.go` — Calling `Stop()` twice will call `p.cancel()` twice. The second call is a no-op for context cancellation, and `p.wg.Wait()` will return immediately. Not a panic actually — but calling `Start()` twice *will* overwrite `p.cancel` and `p.totalWorkers`, leaking the first batch of goroutines. No guard against double-start.

### 7. Mock queue's `Claim` doesn't pass context cancellation (Nit)

**File:** `pool_test.go` — The mock `Claim` ignores context. In the real queue, a cancelled context would make the DB call return an error. The mock always succeeds or returns nil. This means tests don't verify the `ctx.Err()` check in the claim error path realistically. Not blocking, but worth noting.

---

## What's Correct

- Atomic counters for stats — no mutex contention, correct usage
- `sleep()` properly selects on `ctx.Done()` — no leaked goroutines on shutdown
- File read → FailPermanent matches spec ("file missing = not retryable")
- Retry threshold `entry.RetryCount+1 >= p.cfg.MaxRetries` is consistent with queue's `Fail` incrementing `retry_count` before checking
- `wg.Add(1)` before `go` — correct pattern, no race
- Timer cleanup with `defer t.Stop()` in sleep

---

## Summary

The pool is well-structured and the happy path works. The biggest real issue is #1 — context propagation into Extract undermines the graceful shutdown guarantee the spec requires. Issue #4 is a correctness concern for production. The rest are minor. Fix #1 and #4, and this is an A.
