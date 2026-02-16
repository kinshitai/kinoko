# PR #17 Review — feat: Phase A2 — Novelty API + SQLite vec storage

**Reviewer:** Jazz
**Date:** 2026-02-16
**Grade: B+**

---

## Summary

Clean, focused PR. ~460 lines across 5 files. Adds novelty checking endpoint backed by in-process cosine similarity over stored skill embeddings. Matches spec A2. No SQLite vec extension used (pure Go cosine — fine for now, won't scale past ~10k skills). Tests cover the important paths.

Grudgingly: this is decent work.

---

## File-by-File Findings

### `internal/storage/novelty.go`

**P1 — Full table scan with no pagination guard.**
`FindSimilar` loads ALL embeddings into memory, computes cosine sim in Go, sorts, then truncates. The spec says "SQLite vec extension" — this is brute force. Acceptable for Phase A with small datasets but:
- No comment acknowledging this is temporary
- No upper bound on rows scanned — if someone ingests 50k skills, this OOMs or crawls

**P2 — No context cancellation check inside the loop.**
Long-running scan with many rows won't respect `ctx.Done()`. Add a periodic check.

**P3 — `bytesToFloat32s` assumes little-endian.** Already existed pre-PR, not your bug, but worth noting.

### `internal/api/novelty.go`

**P1 — Request body size not limited.**
`json.NewDecoder(r.Body).Decode(&req)` reads unbounded input. A malicious client can POST gigabytes. Wrap with `http.MaxBytesReader`:
```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
```

**P2 — Trace filename collision.**
`time.Now().UTC().Format("20060102_150405")` has 1-second granularity. Two requests in the same second overwrite each other. Add a counter or use nanoseconds.

**P2 — `truncateForTrace` slices bytes, not runes.**
Will split multi-byte UTF-8 characters. Use `utf8` or `[]rune` for correctness.

**P3 — Hardcoded `searchThreshold := 0.3`.**
Should be configurable or at least a named constant with a comment explaining why 0.3.

### `internal/api/server.go`

**P2 — `noveltyMux` naming is misleading.**
It's the main `http.ServeMux`, not a novelty-specific mux. Call it what it is or just keep a reference to `mux` as a field. Current name implies a separate mux for novelty routes.

**P3 — `SetNoveltyChecker` is post-construction registration.**
This is a mild code smell — the route exists or doesn't depending on call order. If someone calls `Start()` before `SetNoveltyChecker()`, there's a race. Not dangerous with current usage but fragile. A TODO comment would suffice.

### `internal/config/config.go`

Clean. `GetNoveltyThreshold()` defaults correctly. No issues.

### `internal/api/novelty_test.go`

**P2 — No test for high-similarity (novel=false) case.**
All tests run against an empty DB, so novelty is always true. Need a test that:
1. Stores a skill embedding
2. Queries with similar content
3. Asserts `novel=false` and `score > threshold`

This is the most important behavior to test and it's missing.

**P3 — `TestNoveltyHandler_NoEngine` creates checker with nil Store.**
Would panic if the handler somehow got past the engine nil check. Fragile test setup.

---

## Must-Fix (blocking merge)

| # | File | Issue |
|---|------|-------|
| 1 | `api/novelty.go` | **P1**: Add `MaxBytesReader` on request body |
| 2 | `storage/novelty.go` | **P1**: Add comment acknowledging brute-force scan is temporary; add TODO for vec extension or index |
| 3 | `api/novelty_test.go` | **P2→P1**: Add test for `novel=false` path (store embedding, query similar content) |

## Nice-to-Have

| # | File | Issue |
|---|------|-------|
| 4 | `api/novelty.go` | P2: Trace filename collision (use nanos) |
| 5 | `api/novelty.go` | P2: `truncateForTrace` byte vs rune |
| 6 | `api/server.go` | P2: Rename `noveltyMux` to `mux` or similar |
| 7 | `storage/novelty.go` | P2: Check `ctx.Done()` in scan loop |
| 8 | `api/novelty.go` | P3: Named constant for search threshold |

---

## What's Good

- Error handling follows `fmt.Errorf("context: %w", err)` pattern consistently ✓
- Structured logging with slog ✓
- Clean separation: NoveltyChecker is independently testable ✓
- Config defaults are sensible ✓
- Mock engine is well-designed for deterministic testing ✓
- Trace feature is non-intrusive (no-op when disabled) ✓

---

## Verdict: **REVISE**

Fix the 3 must-fix items (MaxBytesReader, brute-force acknowledgment, novel=false test) and this merges. The rest can be follow-up.

Grade after fixes: projected A-.
