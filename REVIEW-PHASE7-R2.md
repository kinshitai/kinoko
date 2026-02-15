# Phase 7 Re-Review (Round 2) — Decay Runner

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Previous Grade:** C+  
**Grade: B+**

Significant improvement. All critical and high issues addressed. Code reads like someone actually read the review instead of skimming it.

---

## R1 Critical — Status: FIXED

### 1. Double-counting decay via UpdatedAt ✅

Switched time anchor to `LastInjectedAt`. Godoc on `SkillWriter` now documents the contract explicitly. Fallback to `UpdatedAt` only when `LastInjectedAt.IsZero()` — reasonable. Added `TestDecayUsesLastInjectedAt` that specifically validates the distinction. This is the right fix.

### 2. Division by zero on zero half-life ✅

`ValidateConfig` rejects `<= 0` half-lives at construction time. `NewRunner` returns error. Defensive guard in `RunCycle` (line ~118: `if halfLife <= 0`) is belt-and-suspenders — fine, though now unreachable since `NewRunner` blocks it. Tests cover zero, negative, and zero-struct configs.

---

## R1 High — Status: FIXED

### 3. RescueWindowDays documented ✅

Comment on the struct field explains it's an implementation extension. Good enough.

### 4. DecayCycleResult naming ✅

Matches spec now.

### 5. Rescue boost configurable ✅

`RescueBoost` is a config field with validation (`[0,1]`). `TestCustomRescueBoost` exercises a non-default value. Clean.

### 6. ListByDecay limit=0 convention ✅

Passes `0` instead of `math.MaxInt32`. Documented on the `SkillReader` interface. Correct.

---

## Remaining Issues

### Medium: SuccessCorrelation > 0 still has no minimum threshold (carried from R1 #8)

`shouldRescue` still triggers on `SuccessCorrelation = 0.001`. No injection count check either (R1 #9). These are design decisions more than bugs — flagging again but not blocking. If the team is OK with it, add a one-line comment explaining the deliberate choice so the next person doesn't file the same concern.

### Medium: Demoted count still includes rescued skills (carried from R1 #10)

A rescued skill whose final score is still below `oldDecay` increments both `Rescued` and `Demoted`. Same logic as before. Minor — the counts are informational, not control flow. Still misleading if anyone builds dashboards on these numbers.

### Low: Defensive half-life guard is dead code (new)

```go
if halfLife <= 0 {
    newDecay = oldDecay
}
```

`ValidateConfig` already rejects `<= 0`, so this branch is unreachable in production. Not harmful — it's defensive — but it'll show as uncovered in coverage reports. Either remove it or add a comment explaining it's intentional defense-in-depth.

### Low: `TestReaderError` has a no-op assertion (new)

```go
if !errors.Is(err, errors.Unwrap(err)) {
    // Just verify it wraps properly
}
```

This asserts nothing. `errors.Is(err, errors.Unwrap(err))` is always true when there's a wrapped error. Either verify the error message contains `"decay: list skills"` or remove the dead check.

---

## Tests

Much improved from R1. Now covers:

- ✅ Config validation (zero, negative, bad boost, zero-struct)
- ✅ `LastInjectedAt` vs `UpdatedAt` distinction
- ✅ Reader error propagation
- ✅ Writer error propagation
- ✅ Partial write failure (3 skills, fail on 3rd)
- ✅ Rescue score values (not just counts)
- ✅ Rescue boundary capping at 1.0
- ✅ Custom rescue boost value

Still missing but non-blocking:
- Future `LastInjectedAt` (clock skew) — `daysSince` goes negative, `math.Pow(0.5, negative)` > 1, decay score *increases*. Probably wrong but edge-case.
- Very large `daysSince` — float64 underflow to 0.0, which hits deprecation. Acceptable behavior but worth a comment.

---

## Summary

Every critical and high item is resolved. The `LastInjectedAt` refactor is clean and well-tested. Config validation closes the zero-value trap. Remaining items are medium/low — polish, not correctness. Ship it.
