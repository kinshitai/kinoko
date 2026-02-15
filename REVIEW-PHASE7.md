# Phase 7 Review — Decay Runner

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade: C+**

Decent structure, clean code, but a potential math bug that could silently corrupt every decay score in the database. Tests look thorough at first glance but miss the cases that actually matter.

---

## Critical

### 1. Decay formula correctness depends on an undocumented contract (runner.go:82-84)

```go
newDecay := oldDecay * math.Pow(0.5, daysSince/float64(halfLife))
```

This multiplies the *already-decayed* `oldDecay` by a factor computed from `daysSince` = time since `UpdatedAt`. If `UpdateDecay` (the writer) does **not** update `updated_at` to `now`, the next cycle will use the same large `daysSince` and apply it to the already-reduced score — double-counting decay. After two daily cycles, a tactical skill would effectively decay as if `0.5^(2/90)` happened twice (= `0.5^(4/90)`) instead of the correct `0.5^(2/90)`.

The math is only correct if the `SkillWriter.UpdateDecay` implementation also bumps `updated_at`. This is an **implicit contract** that is:
- Not documented on the `SkillWriter` interface
- Not enforced by the runner
- Not tested

If someone writes a `SkillWriter` that only updates `decay_score`, every skill in the library silently decays at an accelerating rate.

**Fix:** Either (a) change the interface to `UpdateDecay(ctx, id, decayScore, updatedAt)` making the contract explicit, (b) document it in a godoc comment on `SkillWriter`, or (c) rethink the formula to be absolute: store an anchor score/time and compute from that.

### 2. Division by zero on zero half-life (runner.go:107-117)

```go
func (r *Runner) halfLifeDays(cat extraction.SkillCategory) int {
    switch cat {
    case extraction.CategoryFoundational:
        return r.cfg.FoundationalHalfLifeDays
```

If any half-life config value is 0 (misconfiguration, zero-value struct, bad YAML), `float64(halfLife)` is 0, and `daysSince/0.0` = `+Inf`. Then `math.Pow(0.5, +Inf)` = 0, and the skill is instantly deprecated. No panic, but silently kills every skill in that category.

No validation on `Config`. No test for this.

**Fix:** Validate config on construction. `NewRunner` should return an error if any half-life is ≤ 0.

---

## High

### 3. Spec mismatch: `RescueWindowDays` doesn't exist in spec (runner.go:22)

The spec's `DecayConfig` (Appendix A) has no `RescueWindowDays` field. The code adds it with a default of 30. This is an undocumented extension. Either update the spec or remove it and hardcode (with a comment explaining why).

### 4. Spec mismatch: return type naming (runner.go:46)

Spec defines `DecayCycleResult`. Code defines `CycleResult`. The `DecayRunner` interface in the spec returns `*DecayCycleResult`. Pick one — preferably the spec's name since this is a public interface.

### 5. Rescue boost of 0.3 is hardcoded magic number (runner.go:89)

```go
newDecay = math.Min(1.0, newDecay+0.3)
```

Not configurable. Not in the spec. Not documented why 0.3. This will be the first thing someone cargo-cults or accidentally changes. Make it a config field or at least a named constant with a comment.

### 6. `math.MaxInt32` as "give me everything" (runner.go:74)

```go
skills, err := r.reader.ListByDecay(ctx, libraryID, math.MaxInt32)
```

Code smell. If the interface takes a limit, either pass 0 to mean "no limit" (document that convention on the interface) or don't have a limit parameter at all for this use case. `MaxInt32` is a hack that leaks implementation assumptions.

---

## Medium

### 7. No error propagation tests

No test for `reader.ListByDecay` returning an error. No test for `writer.UpdateDecay` returning an error mid-batch. The mocks support it (`err` field) but nobody exercises them. The partial-failure case (3 of 10 skills updated, then writer error) is particularly interesting — the function returns an error but has already written some updates. Is that OK? Document it or handle it.

### 8. Rescue logic: `SuccessCorrelation > 0` threshold (runner.go:124)

A skill with `SuccessCorrelation = 0.001` (essentially noise) triggers a full 0.3 rescue boost. This feels wrong. Should there be a minimum correlation threshold? The spec doesn't say, but a near-zero correlation shouldn't have the same rescue effect as 0.9.

### 9. `shouldRescue` doesn't check `InjectionCount` (runner.go:119-125)

A skill with 1 injection and `SuccessCorrelation = 1.0` gets rescued. That's a sample size of 1. The spec mentions flagging skills with `success_correlation < -0.2` only after ≥10 injections (§3.5). There should be a symmetric floor for rescue — don't boost based on insufficient data.

### 10. Demoted count is misleading (runner.go:95)

```go
} else if newDecay < oldDecay {
    result.Demoted++
}
```

A rescued skill that still ends up lower than `oldDecay` counts as both rescued AND demoted. That's confusing. A skill with `oldDecay=0.8`, computed decay `0.4`, rescued to `0.7` is counted as rescued (correct) and demoted (technically true but misleading). Should rescued skills be excluded from the demoted count?

---

## Tests

The test file is solid in structure — table-driven, good mocks, fixed clock. But:

- **Missing:** zero half-life config
- **Missing:** reader/writer error paths
- **Missing:** partial failure (writer fails on 3rd of 5 skills)
- **Missing:** rescue + deprecation interaction (rescued but still below threshold)
- **Missing:** skill with `UpdatedAt` in the future (clock skew)
- **Missing:** very large `daysSince` (overflow/underflow of float64)
- `TestRescueLogic` doesn't assert the actual score values, only the rescued count. You can get `rescued=1` with a completely wrong boost calculation.

---

## Summary

The core structure is right. Interfaces are clean, the clock injection is good, the code reads well. But the decay math has a hidden dependency that will bite someone, zero-value configs can silently destroy a library, and the tests don't cover the failure modes that matter in production. Fix the critical items before merging.
