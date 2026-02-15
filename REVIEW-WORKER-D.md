# Review: Phase D — CLI + Serve Integration

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `cmd/mycelium/serve.go`, `worker.go`, `importcmd.go`, `queuecmd.go`, `root.go`

---

## S1 from Phase C: NOT FIXED

`scheduler.go:154` still calls `DefaultConfig().StaleClaimTimeout`. Nobody touched it. Carrying forward.

---

## serve.go

### D1 (Medium) — Hook is a dummy until worker system succeeds.

`buildSessionHooks` sets `OnSessionEnd` to a stub returning `StatusRejected`. Then *after* `startWorkerSystem`, it's conditionally replaced with the enqueue hook. If `startWorkerSystem` fails (which is logged as a warning, not an error), every session silently gets `StatusRejected` with no extraction and no error surfaced to the caller. The git server happily accepts sessions that vanish into nothing.

This is a silent data-loss path. If the worker system can't start, either fail hard or make the hook return an error so callers know extraction isn't happening.

### D2 (Low) — `buildSessionHooks` creates an embedder and LLM client, then `buildPipeline` creates *another* embedder and LLM client.

Two separate `embedding.New()` calls, two separate `openAILLMClient` instances. Not a bug but wasteful. Should share them via a struct or pass them in.

### D3 (Low) — `waitForShutdown` shadowed context.

```go
ctx, cancel := context.WithCancel(ctx)
```

The parent `ctx` is from `cmd.Context()`. Shadowing it and creating a new cancel means the goroutine's `cancel()` only cancels the derived context, which is fine — but the original context's cancellation also propagates. The double `select` on `done` and `ctx.Done()` is redundant since the goroutine closes `done` on *either* signal or context cancel. Not broken, just muddled. Clean it up.

### D4 (Nit) — Shutdown order comment says "scheduler → pool → git server → store" but store isn't explicitly stopped in `waitForShutdown`.

The comment in the function says it handles store, but store.Close() is deferred in `runServe`. The separation is fine but the comment lies.

### What's correct:
- Shutdown order (scheduler → pool → server) is correct per spec.
- 30s shutdown timeout is correct.
- Hook replacement from sync to async enqueue is clean.
- `startWorkerSystem` is well-factored and reused by `worker.go`.

---

## worker.go

### D5 (Medium) — `ignoreNil` generic helper has a subtle interface-nil bug.

```go
func ignoreNil[T any](v T, fn func(T) error) error {
    if any(v) == nil {
        return nil
    }
    return fn(v)
}
```

If `T` is `worker.Pool` (an interface) and the value is a non-nil interface wrapping a nil pointer, `any(v) == nil` is `false` and you call `fn` on a nil-pointered interface. Classic Go trap. In practice `startWorkerSystem` won't return a nil-pointered interface, but this helper is generic and exported-adjacent. Either document the limitation or use `reflect.ValueOf(v).IsNil()`.

### D6 (Low) — No shutdown error logging.

`serve.go` logs errors from `sched.Stop()` and `pool.Stop()`. `worker.go` discards them with `_ = ignoreNil(...)`. If shutdown fails, you'll never know. Log the errors.

### D7 (Low) — Duplicate signal handling code.

`worker.go` reimplements the signal wait that `waitForShutdown` already does in `serve.go`. Factor out a `waitForSignal` helper. Copy-paste is how shutdown bugs breed.

### What's correct:
- Reuses `startWorkerSystem` — good, no drift between serve and worker modes.
- No git server started — matches spec §5 exactly.

---

## importcmd.go

### D8 (Medium) — No validation of file size before `os.ReadFile`.

```go
content, err := os.ReadFile(p)
```

If someone does `mycelium import /dev/zero` or passes a 10GB log, this OOMs. Add a size check or use `io.LimitReader`.

### D9 (Medium) — File extension filter is too restrictive and undocumented.

`--dir` only picks up `.log`, `.txt`, `.json`. If someone has `.md` or `.yaml` session files, they're silently skipped. No log, no warning. Either document the filter in the help text or make it configurable with `--ext`.

### D10 (Low) — `parseSessionFromLog` is defined in `extract.go`, not in `importcmd.go`.

Cross-file dependency within the same package is fine, but the function generates a session ID (presumably random). If import is re-run on the same file, you get duplicates. No idempotency. The spec doesn't mention dedup for import, but it's a footgun.

### D11 (Nit) — Exit code is 0 even when some files fail.

`errCount > 0` prints to stderr but returns `nil`. Should return an error or use `os.Exit(1)` so scripts can detect partial failure.

### What's correct:
- Doesn't start workers — matches spec §5.
- Library fallback to first configured — sensible default.
- Handles both positional args and `--dir` — good UX.

---

## queuecmd.go

### D12 (Medium) — Spec says `queue flush`, not implemented.

Spec §5: "`queue stats`, `queue list`, `queue retry <id>`, `queue flush`." There's no `flush` subcommand. That's a missing feature, not a nit.

### D13 (Low) — `queue list` hardcoded LIMIT 20, no `--limit` flag.

If someone has 500 queued sessions, they see 20 and have no way to see the rest. Add a `--limit` flag.

### D14 (Low) — `queue retry` resets `retry_count` to 0.

This means a session that already failed 3 times gets 3 more retries. That might be intentional (manual override), but it should be documented or have a `--keep-count` flag. If it keeps hitting the same transient error, you've just created an infinite retry loop for anyone who scripts `queue retry` in a cron.

### D15 (Low) — `queue stats` and `queue list` bypass the `SessionQueue` interface, hitting raw SQL.

The `SessionQueue` interface has `Depth()` — why not use it? Direct SQL couples the CLI to SQLite schema. If the storage ever changes, these commands break independently of the interface.

### D16 (Nit) — Unused logger in `runQueueRetry`.

```go
logger := slog.New(...)
_ = logger
```

Delete it.

---

## root.go

Clean. All three new commands registered. No issues.

---

## Grade: **B-**

The core integration works: hook replacement is correct, shutdown ordering is correct, `startWorkerSystem` is properly shared between serve and worker modes. But there's a silent data-loss path when workers fail to start (D1), a missing spec feature (D12), no file size protection on import (D8), and the Phase C stale timeout bug (S1) is still open. The queue CLI feels like it was written in a rush — raw SQL instead of using the queue interface, missing `flush`, hardcoded limits.

---

## Action Items

| ID | Severity | Description |
|----|----------|-------------|
| S1 | Medium | **STILL OPEN from Phase C.** `scheduler.go:154` uses `DefaultConfig().StaleClaimTimeout` instead of configured value |
| D1 | Medium | Silent data loss when worker system fails to start — hook returns `StatusRejected` with no error |
| D5 | Medium | `ignoreNil` generic helper doesn't catch nil-pointered interfaces |
| D8 | Medium | `import` does unbounded `ReadFile` — add size limit |
| D9 | Medium | `--dir` file extension filter is silent and undocumented |
| D12 | Medium | `queue flush` subcommand missing per spec §5 |
| D6 | Low | `worker.go` discards shutdown errors |
| D7 | Low | Duplicate signal handling between `serve.go` and `worker.go` |
| D10 | Low | Import is not idempotent — re-import creates duplicates |
| D11 | Low | Import returns success exit code on partial failure |
| D13 | Low | `queue list` has no `--limit` flag, hardcoded to 20 |
| D14 | Low | `queue retry` silently resets retry count |
| D15 | Low | Queue CLI uses raw SQL instead of `SessionQueue` interface |
| D2 | Low | Duplicate embedder/LLM client construction in serve.go |
| D3 | Low | Shadowed context in `waitForShutdown` |
| D4 | Nit | Shutdown order comment mentions store but doesn't handle it |
| D16 | Nit | Unused logger in `runQueueRetry` |
