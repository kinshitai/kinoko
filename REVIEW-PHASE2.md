# Phase 2 Review — Stage 1 Filter

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `stage1.go`, `stage1_test.go`, cross-ref `types.go`  
**Grade: B-**

---

## stage1.go

### 1. Interface defined in the wrong place

`Stage1Filter` interface is declared in `stage1.go`. Every other shared type — `Stage1Result`, `SessionRecord`, `ExtractionStatus`, etc. — lives in `types.go`. The interface belongs there too. When Stage 2 and 3 land, are their interfaces going in their own files too? Pick a convention and stick with it. Move it to `types.go`.

### 2. Redundant zero-tool-calls special case

```go
if session.ToolCallCount == 0 {
    result.ErrorRateOK = true
} else {
    result.ErrorRateOK = session.ErrorRate <= f.cfg.MaxErrorRate
}
```

The spec says ErrorRate is `0 if no tool calls`. If the caller obeys the spec, `session.ErrorRate` is already 0, and `0 <= 0.7` is true. This branch does nothing except paper over a caller bug — silently. If you want to be defensive, *validate and log the inconsistency*. If you trust the caller, delete the branch. Right now it's the worst of both worlds: it hides bad data.

### 3. ErrorRate not validated against ErrorCount/ToolCallCount

The filter blindly trusts `session.ErrorRate`. The spec defines it as `error_count / tool_call_count`. A caller could pass `ErrorCount: 10, ToolCallCount: 10, ErrorRate: 0.1` and the filter would happily accept it. For a "cheap, no I/O" filter, a sanity check costs nothing:

```go
if session.ToolCallCount > 0 {
    expected := float64(session.ErrorCount) / float64(session.ToolCallCount)
    if math.Abs(expected - session.ErrorRate) > 0.001 {
        slog.Warn("stage1: ErrorRate inconsistent", ...)
    }
}
```

### 4. Stage1Config vs ExtractionConfig

Appendix A defines `ExtractionConfig` with the exact same four threshold fields. `Stage1Config` is a parallel struct that someone has to manually map from. This will drift. Either embed `Stage1Config` inside `ExtractionConfig`, or take `ExtractionConfig` directly and read the fields you need. Don't make two sources of truth for the same four numbers.

### 5. No logger injection

Hardcoded `slog.Info` calls. Can't suppress in tests, can't redirect in benchmarks, can't verify log output. Pass `*slog.Logger` in the constructor. This is Go 101.

---

## stage1_test.go

### 6. Not table-driven

Your own standards doc says: "Tests: table-driven, test both positive and negative cases." These are 13 individual functions that each construct a filter, mutate one field, and check one thing. Classic table-driven candidate:

```go
tests := []struct {
    name    string
    mutate  func(*SessionRecord)
    passed  bool
    checks  map[string]bool
}{...}
```

Half the test file is boilerplate that a 5-line table would eliminate.

### 7. Missing edge cases

- **Negative duration**: `DurationMinutes: -5` — does DurationOK become false? Probably yes by accident (−5 < 2), but it's untested.
- **Negative tool calls**: `ToolCallCount: -1` — passes the `>= 3` check? No. But what about ErrorRate with negative ErrorCount?
- **ErrorRate exactly 0.0**: Only tested implicitly via the zero-tool-calls test.
- **NaN / Inf floats**: `DurationMinutes: math.NaN()` — `NaN >= 2` is false in Go, so it rejects. But `NaN <= 180` is also false. Good. Untested though.
- **MaxDuration < MinDuration config**: What happens with `Stage1Config{MinDurationMinutes: 100, MaxDurationMinutes: 10}`? Nothing passes. Should the constructor validate this? At minimum, test the behavior.

### 8. Reason string content not verified

Most tests check `r.Passed` and individual booleans but don't assert on `r.Reason` content. The reason string is the only diagnostic output for rejected sessions. Test that it contains the expected substring — "duration", "tool_calls", "error_rate", etc. The `MultipleFailures` test should verify all four reasons appear.

### 9. validSession() hides MessageCount

`validSession()` doesn't set `MessageCount`, `TokensUsed`, `AgentModel`, `UserID`, or `LibraryID`. These are zero-valued. That's fine for *now* since Stage 1 doesn't check them, but it makes the helper misleading — it's not actually a "valid session" by any realistic standard. Name it `minimalPassingSession()` or populate the fields.

---

## Spec Conformance

- ✅ `Stage1Filter` interface signature matches §2.1 exactly
- ✅ `Stage1Result` fields match §1.3
- ✅ Default thresholds match Appendix A
- ✅ "Synchronous, cheap, no I/O" — correct, no external calls
- ⚠️ Config struct doesn't align with `ExtractionConfig` from Appendix A (parallel definition)
- ⚠️ Interface location inconsistent with types.go pattern

---

## Summary

The logic is correct. The code works. But this is the *simplest phase in the entire pipeline* — four comparisons and an AND. The bar for "clean" is correspondingly higher. Interface in the wrong file, redundant defensive code that hides bugs instead of surfacing them, non-table-driven tests despite your own standards requiring it, and missing edge cases on a function that literally just compares numbers. There's no excuse for not testing negative inputs and NaN on a numeric filter.

Not broken. Just sloppy for what it is.

**Grade: B-**
