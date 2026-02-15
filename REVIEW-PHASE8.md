# Phase 8 Review — CLI Integration

**Reviewer:** Jazz
**Date:** 2026-02-15
**Grade: C+**

Not terrible, not good. The wiring mostly works but there are real bugs and several lazy shortcuts that will bite you in production.

---

## Critical Issues

### 1. `ListByDecay` with limit=0 returns zero rows, not all rows (BUG)
**File:** `decay.go` → `runner.go` → `store.go`

The decay runner calls `ListByDecay(ctx, libraryID, 0)` expecting "no limit" per the `SkillReader` contract comment. But `store.go:ListByDecay` passes `limit` directly to SQL `LIMIT ?`. In SQLite, `LIMIT 0` returns **zero rows**. The decay command will silently process nothing and report `Processed: 0`. Completely broken.

**Fix:** In `ListByDecay`, check `if limit <= 0` and either omit the LIMIT clause or use `LIMIT -1` (SQLite's "no limit" value).

**Severity:** 🔴 Data corruption class — decay never runs, skills never expire.

### 2. `extract` calls `os.Exit()` after returning from `RunE` (BUG)
**File:** `extract.go`, lines ~97-102

```go
if result.Status == extraction.StatusRejected {
    os.Exit(2)
}
```

Cobra's `RunE` should return errors for non-zero exits. Calling `os.Exit()` inside `RunE` skips deferred `store.Close()`, skips Cobra's cleanup, and prevents testing. The `main.go` already handles `os.Exit(1)` on error.

**Fix:** Return a typed error and handle exit codes in `main.go`, or use `cmd.SilenceErrors` + `cobra.Command.SetExitCode` pattern.

**Severity:** 🟡 Resource leak + untestable exit paths.

### 3. `--dry-run` ignores the passed context (BUG)
**File:** `decay.go`, `runDecayDryRun`

```go
func runDecayDryRun(_ context.Context, store *storage.SQLiteStore, ...) error {
    runner.RunCycle(context.Background(), libraryID)
```

The function signature accepts a context but discards it, then creates `context.Background()`. If the user hits Ctrl+C, the dry run won't cancel. Same issue in the non-dry-run path of `runDecay`.

**Severity:** 🟡 Unresponsive to cancellation signals.

### 4. `noopDecayWriter` doesn't implement full interface contract
**File:** `decay.go`

`noopDecayWriter` only implements `UpdateDecay`. But `decay.NewRunner` takes separate `SkillReader` and `SkillWriter` args. The store is passed as reader, noop as writer — this works because `SkillWriter` only has `UpdateDecay`. Fine for now, but fragile. No compile-time check.

**Fix:** Add `var _ decay.SkillWriter = (*noopDecayWriter)(nil)` like you did for `storeQuerier`.

**Severity:** 🟢 Minor, but your test file already does compile-time checks for other adapters — be consistent.

---

## Significant Issues

### 5. `stats.go` — SQL queries have no error checking
**File:** `stats.go`

Every `db.QueryRow(...).Scan(...)` call ignores the error return:

```go
row := db.QueryRow(`SELECT COUNT(*) FROM sessions`)
row.Scan(&totalSessions)  // error silently dropped
```

If the `sessions` table doesn't exist (e.g., schema not migrated), you get zero values with no indication anything is wrong. At minimum check the first query and bail if the schema is missing.

**Severity:** 🟡 Silent failures, misleading output.

### 6. `stats.go` — `rows.Scan` error ignored in skills-by-category loop
**File:** `stats.go`

```go
rows.Scan(&cat, &count)  // error dropped
```

Also `rows.Close()` is called manually but should be `defer rows.Close()` immediately after error check.

### 7. Session log parser — false positives everywhere
**File:** `extract.go`, `parseSessionFromLog`

The error pattern `(?i)(error|failed|exception|traceback)` matches normal log lines like `"error handling improved"`, `"failed over to backup (success)"`, or any line containing the word "exception" in prose. The exec pattern matches `"execute"` and `"run"` which appear in literally any English text.

`HasSuccessfulExec` really means "the log contains the word 'run' somewhere" — that's not useful signal.

**Severity:** 🟡 Garbage-in data for Stage 1 filtering. Will inflate `ErrorCount` and `HasSuccessfulExec`.

### 8. `extract.go` — no `--dry-run` flag
**File:** `extract.go`

The spec says `kinoko extract <session-log>` for manual extraction. But there's no way to run the pipeline without writing to the store. The decay command has `--dry-run` — extract should too. Extraction writes a skill to disk AND to the database.

**Severity:** 🟡 Missing spec requirement for safe manual testing.

### 9. `serve.go` — extraction/injection integration is just TODOs
**File:** `serve.go`

The spec says Phase 8 should "Wire extraction into `kinoko serve` as a post-session hook" and "Wire injection into session startup." These are comments, not code. The entire integration part of Phase 8 is unimplemented.

**Severity:** 🔴 Spec non-compliance. The CLI commands work standalone, but the actual *integration* into serve is missing.

---

## Minor Issues

### 10. `extract.go` — hardcoded `gpt-4o-mini` model
Should come from config's `CriticModel` field, not hardcoded.

### 11. `extract.go` — `openAIComplete` reads error body incorrectly
```go
body := make([]byte, 512)
n, _ := resp.Body.Read(body)
```
Single `Read` call may return partial data. Use `io.ReadAll` (or `io.LimitReader`).

### 12. `extract.go` — no timeout/retry on OpenAI calls
The spec defines `RetryConfig` and `CircuitBreakerConfig` in the extraction config. CLI extract ignores both.

### 13. `extract.go` — `estimateTokens` is naive
4 chars/token is a GPT-3 estimate. For code-heavy logs the ratio is worse. Not a bug, but the comment should say "rough estimate" — oh wait, it does. Fine.

### 14. Tests are shallow
`commands_test.go` only tests flag existence and argument validation. Zero integration tests. No test for `parseSessionFromLog` edge cases (empty file, no timestamps, malformed lines). The one parser test exists but doesn't cover negative cases.

---

## What's Actually Fine

- Dependency wiring for extract/decay/stats follows correct patterns (store as reader+writer, adapter structs)
- `storeQuerier` adapter cleanly bridges `storage.Query` → `extraction.SkillQuerier`
- Subcommand registration in `root.go` is complete
- Config loading pattern is consistent across all commands
- `decay.go` correctly separates reader (store) and writer (noop) for dry-run — the design is right even if context handling is wrong

---

## Summary

| Area | Grade |
|------|-------|
| Dependency wiring | B |
| Error handling | D |
| Session log parsing | D+ |
| Stats queries | C- |
| `--dry-run` | C |
| Exit codes | D |
| Spec compliance | D (serve integration missing) |
| Tests | D+ |
| **Overall** | **C+** |

The standalone CLI commands (`extract`, `decay`, `stats`) are structurally sound but have a showstopper bug (#1 — decay processes zero skills), a resource leak (#2), and the actual point of Phase 8 — wiring into `serve` — is TODO comments.

Fix #1, #2, #3, #5, and #9 before merging. The rest can be tracked as follow-ups.
