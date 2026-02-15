# Review: Worker Pool (Phase B fixes) + Scheduler (Phase C)

**Reviewer:** Jazz
**Date:** 2026-02-15
**Files:** `internal/worker/pool.go`, `pool_test.go`, `scheduler.go`, `scheduler_test.go`, `config.go`

---

## Phase B Fix Verification

### Detached context fix ✅
Line `context.WithTimeout(context.Background(), 60*time.Second)` — correct. Extract runs on a background context, pool cancellation won't kill in-flight work. Test `TestPool_GracefulShutdown_InFlightCompletes` explicitly verifies the extract context isn't cancelled. Good.

### getSession transient fix ✅
`getSession` failure calls `queue.Fail()` (transient), not `FailPermanent()`. Test `TestPool_GetSessionFailureIsTransient` covers this path and asserts `f.permanent == false`. Good.

Both B fixes verified and tested.

---

## Phase C: Scheduler Review

### Issues Found

**S1 (Medium) — `runStaleSweep` uses hardcoded config instead of its own field.**
```go
n, err := s.queue.RequeueStale(ctx, DefaultConfig().StaleClaimTimeout)
```
This calls `DefaultConfig()` every tick instead of using a configurable value. If someone changes the default later, existing schedulers silently change behavior. Should accept `StaleClaimTimeout` from the pool's `Config` or from `SchedulerConfig`. Currently the scheduler doesn't even have access to the pool config — it just gets the `Pool` interface. This is a design smell: the scheduler needs a value it can't get from its own config.

**S2 (Low) — `RetrySweepInterval` in `SchedulerConfig` is unused.**
Config defines `RetrySweepInterval: 5 * time.Minute` but no goroutine runs a retry sweep. Dead config field. Either remove it or implement the sweep.

**S3 (Low) — `parseDailyCron` only handles `M H * * *` format.**
That's fine for now but the field is called `DecayCron` implying general cron support. The fallback to 24h ticker on parse failure is reasonable, but the log message should be louder — `Warn` is too quiet for "your cron config is being ignored." Make it `Error`.

**S4 (Low) — `TestDecayRunsOnSchedule` doesn't actually verify decay ran.**
```go
var decayRan atomic.Int32
// ... never incremented, never asserted
_ = decayRan
```
The test just verifies start/stop don't block. That's a start/stop test, not a decay-runs-on-schedule test. The name lies.

**S5 (Nit) — Scheduler `Stop` isn't safe to call twice.**
Second `s.cancel()` call panics if `cancel` is nil (never started) or is the same func (already cancelled — actually fine, cancel is idempotent). But if `Start` was never called, `s.cancel` is nil → panic. Pool has the same issue. Minor, but add a nil guard or document it.

### What's Fine

- Timer-based decay scheduling with `nextDailyFunc` injection — clean, testable.
- `parseDailyCron` with range validation — correct.
- `nextDailyDelay` handles wrap-around to next day — correct.
- Stale sweep and stats logger use proper ticker + select pattern — clean shutdown.
- All three goroutines tracked via `sync.WaitGroup` with timeout on Stop — good.
- Test coverage for cron parsing, delay calculation, sweep calls, stats calls, start/stop lifecycle — adequate.

---

## Pool Test Quality

Tests are solid. Good coverage of: happy path, retry vs permanent failure, file-not-found, graceful shutdown with in-flight work, rejected sessions, transient getSession, no-spin on empty queue. The spin test (`TestPool_EmptyQueue_NoSpin`) is a nice touch.

One nit: `TestPool_ProcessSessions` and others poll with `time.After(10ms)` busy loops. Not a real problem in tests, just ugly. Could use channels but not worth changing.

---

## Combined Grade: **B+**

Pool fixes are correct and well-tested. Scheduler is functional but has the hardcoded `DefaultConfig()` smell (S1), a dead config field (S2), and a test that doesn't test what it claims (S4). None are blocking but S1 should get fixed before it bites someone.

### Action Items
| ID | Severity | Description |
|----|----------|-------------|
| S1 | Medium | Pass `StaleClaimTimeout` into scheduler or its config instead of calling `DefaultConfig()` |
| S2 | Low | Remove `RetrySweepInterval` from config or implement retry sweep |
| S3 | Low | Upgrade cron parse failure log from Warn to Error |
| S4 | Low | Fix `TestDecayRunsOnSchedule` to actually assert decay execution |
| S5 | Nit | Guard against `Stop()` called without `Start()` |
