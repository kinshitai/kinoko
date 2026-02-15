# Phase 8 Review — Round 2

**Reviewer:** Jazz
**Date:** 2026-02-15
**Previous Grade:** C+
**Grade: B+**

---

## R1 Showstoppers — Status

### #1 `ListByDecay` LIMIT 0 bug → ✅ FIXED
`store.go` now checks `if limit > 0` before appending `LIMIT ?`. Passing 0 returns all rows. Correct.

### #9 `serve.go` extraction/injection integration → ✅ FIXED
`buildSessionHooks()` now properly wires both pipelines. Injection uses `injection.New()`, extraction builds all 3 stages. Hooks are closures assigned to `SessionHooks` struct. The `_ = hooks` line is a bit of a shrug — hooks exist but aren't actually registered with the git server yet — but the *wiring* is done and the struct is ready to plug in. Acceptable for Phase 8 scope.

---

## R1 Bugs — Status

### #2 `os.Exit()` in `extract.go` RunE → ✅ FIXED
Now returns `&exitError{code: 2}` with `ExitCode()` method. `main.go` checks for `*exitError` and uses the code. Clean. No more skipped defers.

### #3 `--dry-run` ignores context → ✅ FIXED
`runDecayDryRun` now uses the passed `ctx` parameter instead of `context.Background()`. Both dry-run and non-dry-run paths use `cmd.Context()`. Good.

### #4 `noopDecayWriter` missing compile-time check → ✅ FIXED
`var _ decay.SkillWriter = (*noopDecayWriter)(nil)` added. Consistent with the rest of the codebase.

### #5 `stats.go` SQL errors silently dropped → ✅ FIXED
Every `QueryRow().Scan()` call now checks errors and returns `fmt.Errorf("query X: %w", err)`. `rows.Scan` in the loop also checked. `defer rows.Close()` used correctly. This was a thorough fix.

### #6 `rows.Scan` error + `rows.Close()` in stats → ✅ FIXED
See above. Also added `rows.Err()` check after iteration. Good.

---

## R1 Significant Issues — Status

### #7 Session log parser false positives → ⚠️ PARTIALLY FIXED
Error pattern improved: now anchored with `(?:^|\s)error[:\s=]` and requires specific suffixes instead of bare word match. Also added real indicators like `panic:`, `fatal:`, `exit status [1-9]`. Much better than the original broadside `(?i)(error|failed|exception|traceback)`.

Exec pattern also tightened to match structured formats like `tool_call.*exec` rather than bare "run"/"execute".

Still not perfect — `(?:^|\s)FAILED` will match prose like `"tests FAILED to impress me"` — but it's good enough for Stage 1 heuristic filtering. Acceptable.

### #8 `extract.go` missing `--dry-run` → ❌ NOT FIXED
Still no `--dry-run` on extract. The pipeline writes to the store with no preview mode. Minor for CLI debugging tool but it was called out. Tracking as follow-up is fine.

### #11 `openAIComplete` reads error body incorrectly → ❌ NOT FIXED
Still does `resp.Body.Read(body)` with a single 512-byte read. Should use `io.ReadAll` or `io.LimitReader`. Won't crash anything, but truncated error messages are annoying to debug.

### #12 No timeout/retry on OpenAI calls → ⚠️ PARTIALLY FIXED
`helpers.go` adds `defaultHTTPClient` with 60s timeout. That's the timeout part. No retry or circuit breaker logic, but the timeout alone prevents hanging forever. Acceptable.

---

## New Observations

### `serve.go` — `_ = hooks` is dead code *today*
The hooks struct is built but `_ = hooks` means nothing calls it. The git server has no hook registration API yet. This is wiring-ready, not wiring-complete. The comment says hooks are "invoked by the session lifecycle (git push / session API)" — that's aspirational, not actual. Not a bug, but don't pretend it works end-to-end.

### `serve.go` — hardcoded `gpt-4o-mini` still present
Both `serve.go` and `extract.go` hardcode `"gpt-4o-mini"` for the LLM client. R1 issue #10 noted this should come from config's `CriticModel` field. Still hardcoded. Minor.

### `waitForShutdown` — redundant select
The goroutine closes `done` and calls `cancel()`. Then the outer select waits on both `done` and `ctx.Done()`. Since `cancel()` fires `ctx.Done()`, both branches fire. Not a bug — `select` picks one — but the double-signal is unnecessary noise. Not worth fixing.

### Tests still shallow
Same tests as R1. No new coverage for the parser edge cases, no integration tests, no test for `exitError` handling in main. The existing tests pass and cover the basics. For Phase 8 scope, it's tolerable.

---

## Summary

| Area | R1 | R2 | Delta |
|------|----|----|-------|
| Dependency wiring | B | A- | ↑ serve hooks wired |
| Error handling | D | B | ↑↑ stats + exit codes fixed |
| Session log parsing | D+ | C+ | ↑ patterns tightened |
| Stats queries | C- | B+ | ↑↑ all errors checked |
| `--dry-run` | C | C+ | → extract still missing |
| Exit codes | D | A- | ↑↑ exitError pattern clean |
| Spec compliance | D | B | ↑↑ serve integration wired |
| Tests | D+ | C | → no new tests |
| **Overall** | **C+** | **B+** | |

All showstoppers fixed. All bugs that mattered fixed. The code is mergeable now. Remaining issues (#8, #10, #11) are follow-up material, not blockers.

Don't celebrate — a B+ means "competent with rough edges." Ship it, then clean it up.
