# Review: Phase D R2 — CLI + Serve Integration (Re-review)

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Round:** R2 (verifying fixes from R1 B- grade)  
**Files:** `serve.go`, `worker.go`, `importcmd.go`, `queuecmd.go`, `scheduler.go`

---

## Targeted Fix Verification

### S1 — `scheduler.go` hardcoded `DefaultConfig().StaleClaimTimeout` → ✅ FIXED

`runStaleSweep` now uses `s.cfg.StaleClaimTimeout`. `startWorkerSystem` in `serve.go` wires it: `schedCfg.StaleClaimTimeout = workerCfg.StaleClaimTimeout`. Clean fix.

### D1 — Silent data loss when worker system fails → ✅ FIXED

`startWorkerSystem` error is now fatal in `runServe` — returns error instead of logging a warning. No more sessions silently getting `StatusRejected`. Good.

### D5 — `ignoreNil` nil-pointered interface bug → ✅ FIXED

Now uses `reflect.ValueOf(&v).Elem()` with kind-switch for Ptr/Interface/Map/Slice/Chan/Func before calling `IsNil()`. Handles the typed-nil case correctly. The `reflect` import is justified here.

### D8 — Unbounded `os.ReadFile` in import → ✅ FIXED

`os.Stat` + 50MB size check before read. Reasonable limit. Error message includes actual size and limit. Good.

### D12 — Missing `queue flush` → ✅ FIXED

`queueFlushCmd` implemented with confirmation prompt and `--force` flag. Counts before deleting, handles zero-queue case. Solid implementation.

---

## Bonus Fixes (not requested but noticed)

### D6 — `worker.go` discards shutdown errors → ✅ FIXED
Now logs errors from `sched.Stop()` and `pool.Stop()`.

### D11 — Import returns 0 on partial failure → ✅ FIXED
Now returns `fmt.Errorf("%d file(s) failed to import", errCount)`.

### D16 — Unused logger in `runQueueRetry` → ✅ FIXED
Removed entirely.

---

## Still Open (not in scope for this fix round, carrying forward)

| ID | Severity | Status | Description |
|----|----------|--------|-------------|
| D2 | Low | Open | Duplicate embedder/LLM client between `buildSessionHooks` and `buildPipeline` |
| D3 | Low | Open | Shadowed context in `waitForShutdown` — works but muddled |
| D7 | Low | Open | Duplicate signal handling between `serve.go` and `worker.go` |
| D9 | Medium | Open | `--dir` extension filter (`.log`, `.txt`, `.json`) silent and undocumented in help text |
| D13 | Low | Open | `queue list` hardcoded LIMIT 20, no `--limit` flag |
| D14 | Low | Open | `queue retry` resets `retry_count` to 0 without documentation |
| D15 | Low | Open | `queue stats`/`list` bypass `SessionQueue` interface, hit raw SQL |

---

## Grade: **B+**

All five targeted items (S1, D1, D5, D8, D12) are properly fixed. Three bonus fixes on top. The critical paths — worker failure handling, stale claim timeout, import safety — are all correct now. The remaining opens are low-severity structural debt, not correctness issues. D9 (undocumented extension filter) is the most annoying survivor — users will lose files silently — but it's not a data corruption risk.

Solid fix round. Moved from B- to B+.
