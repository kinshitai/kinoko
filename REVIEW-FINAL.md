# Final Review — Refactoring R7, R8, R9 + Circuitbreaker Fix + Alias Cleanup

**Reviewer:** Jazz
**Date:** 2026-02-15
**Verdict:** Pass. Grudgingly.

---

## Build & Test

```
go build ./...    ✅ Clean
go test ./... -short -count=1    ✅ All 16 packages pass
```

No `ErrCircuitOpen` references found anywhere outside `circuitbreaker` package. Good — the old alias is dead.

---

## Per-Ticket Review

### R7: Pipeline Split (`pipeline.go` → `pipeline.go` + `skillmd.go` + `sampling.go`)

**Grade: A-**

| File | Lines | Concern |
|------|-------|---------|
| `pipeline.go` | 331 | Pipeline struct, constructor, `Extract`, `updateSessionStatus` |
| `skillmd.go` | 146 | `buildSkillMD`, `skillNameFromClassification`, `kebab`, `titleCase` |
| `sampling.go` | 85 | `maybeSample`, `cryptoRandIntn`, `RandIntn` type |

Clean separation. Each file has exactly one reason to change. `pipeline.go` went from 553 → 331 lines, which is right where it should be.

**Nits:**
- `skillmd.go` line counts include the `buildSkillMD` function which uses `fmt.Fprintf` 20+ times in a row. Not a bug, but a `text/template` or `strings.Builder` with fewer `Fprintf` calls would be cleaner. Not worth blocking on.
- `sampling.go`: The stratified sampling logic is correct but the `underrepresented`/`overrepresented` boolean dance is hard to read at a glance. A comment explaining the three cases (under/over/equal) in plain English would help the next person. Minor.

---

### R8: Stage3 Split (`stage3.go` → `stage3.go` + `stage3_prompt.go`)

**Grade: A**

| File | Lines | Concern |
|------|-------|---------|
| `stage3.go` | 315 | Critic struct, `Evaluate`, retry, parsing, score helpers |
| `stage3_prompt.go` | 86 | `buildCriticPrompt`, `truncateContent`, `sanitizeDelimiters`, `generateNonce` |

Prompt construction is cleanly separated. `stage3.go` went from 540 → 315 lines. The `truncateContent` function correctly handles UTF-8 rune boundaries — I checked. `sanitizeDelimiters` with nonce-based delimiters is a proper injection defense.

No complaints. This is exactly the split the plan called for.

---

### R9: Move `logparser` and `querier` out of `cmd/`

**Grade: A-**

| File | Lines | Location |
|------|-------|----------|
| `logparser.go` | 106 | `internal/extraction/` |
| `querier.go` | 31 | `internal/storage/` |

Both files are in the right packages. `ParseSessionFromLog` is exported and used by `cmd/mycelium/extract.go` and `cmd/mycelium/importcmd.go` — verified. `storage.NewSkillQuerier` returns `extraction.SkillQuerier` and is used by both `extract.go` and `serve.go` — verified.

**Nits:**
- `logparser.go` still uses `bufio.Scanner` on content that was already `strings.Split` into `lines` (line 38 vs line 42). The `lines` variable from the Split is only used for `msgCount := len(lines)`. Wasteful — scan once, count lines during the scan. Not a correctness issue but it's the kind of sloppy that accumulates.
- `querier.go` is 31 lines. Tiny but correct. The adapter pattern is clean. `NewSkillQuerier(nil)` doesn't panic at construction time (verified by test), which is fine since it'll fail on use.

**Stale reference check:** `cmd/mycelium/extract_test.go` has comments referencing "R9 area" and "Must exist BEFORE moving to internal/extraction/logparser.go" — these comments are now outdated since the move is done. The tests themselves correctly import `extraction.ParseSessionFromLog`. The comments should be cleaned up but aren't blocking.

---

### Circuitbreaker Validation Fix

**Grade: A**

`internal/circuitbreaker/breaker.go` (132 lines):
- `Threshold <= 0` → error ✅
- `BaseDuration <= 0` → error ✅
- `MaxDuration < BaseDuration` → error ✅
- `clock == nil` → default to `realClock{}` ✅
- Error messages include the actual values ✅
- `mustNewBreaker` in `stage3.go` panics on invalid config with a clear message ✅

The validation is correct and defensive. The `New` constructor returns `(*Breaker, error)` which is the right API — callers choose whether to panic or propagate. The exponential backoff in `RecordFailure` correctly caps at `MaxDuration`. The half-open state correctly allows only one probe (subsequent calls return `ErrOpen`).

One thing I like: renaming `ErrCircuitOpen` → `ErrOpen` within the package. The package name already provides context: `circuitbreaker.ErrOpen` reads better than `circuitbreaker.ErrCircuitOpen`. Grep confirms zero remaining references to the old name.

---

### Alias Cleanup

**Grade: A**

No stale type aliases, no re-exports, no compatibility shims found. `grep -r ErrCircuitOpen` returns nothing. Clean.

---

## Overall Refactoring Assessment (R1–R9)

### What the plan set out to do

REFACTOR-PLAN.md identified 5 structural problems:
1. `extraction` package as implicit god types package → **Fixed by R1** (`internal/model`)
2. Copy-pasted circuit breaker → **Fixed by R3** (`internal/circuitbreaker`)
3. LLM infrastructure in `cmd/` → **Fixed by R2** (`internal/llm`)
4. Big files that nobody split → **Fixed by R6, R7, R8**
5. Shared logic in `cmd/` → **Fixed by R9** (logparser, querier)

Plus R4 (llmutil JSON parser) and R5 (taxonomy package) for deduplication.

### Did it work?

**Yes.** The dependency graph is cleaner. `model` is the base types package instead of `extraction`. No package imports `extraction` just for types anymore. The circuit breaker is shared, not duplicated. LLM client code lives where it should.

### File size audit (post-refactoring)

| File | Before | After | Status |
|------|--------|-------|--------|
| `extraction/pipeline.go` | 553 | 331 | ✅ Under 350 |
| `extraction/stage3.go` | 540 | 315 | ✅ Under 350 |
| `storage/store.go` | 765 | Split into 5 files | ✅ |
| `cmd/mycelium/extract.go` | 314 | Trimmed (logparser+querier moved) | ✅ |

### What's left from the plan

R10–R16 (Phase 3-4 polish) are not done yet. These are:
- R10: Deduplicate embedder/LLM client construction in serve.go
- R11: Deduplicate decayConfigFromYAML bridging
- R12: Deduplicate session status update logic
- R13: Split metrics/collector.go:Collect
- R14: Remove unused `db` struct tags
- R15: Remove reflect-based ignoreNil
- R16: Move hook types to model package

None of these are urgent. They're polish. The structural work (R1–R9) was the important part and it's done.

### Codebase state

The codebase is in materially better shape. Package boundaries make sense. Files are focused. Shared code is shared. The dependency graph flows downward without weird cross-cutting imports.

Build is clean. All tests pass. No dead code detected in the reviewed files.

---

## Overall Grade: **A-**

The structural refactoring achieved its goals. The code reads better, the package layout makes sense, and nothing was broken in the process. The minus is for minor sloppiness (stale comments in test files, the double-scan in logparser, sampling logic readability). These are nits, not defects.

I've seen far worse refactorings from people with half the ambition. This one was planned well and executed cleanly. Don't let it go to your head.

---

*— Jazz, still grumpy, but running out of things to complain about*
