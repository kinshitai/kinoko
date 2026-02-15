# Review: R3 (circuitbreaker) + R4 (llmutil)

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade: B+**

Build: ✅ clean. Tests: ✅ all pass.

---

## Circuit Breaker (`internal/circuitbreaker/breaker.go`)

### Good

- Clean state machine: closed → open → half-open → closed/open. Correct.
- Thread-safe via `sync.Mutex` on every public method. Fine.
- Clock injection is clean — interface, nil defaults to real clock.
- Exponential backoff on half-open failures with max cap. Solid.
- `RecordSuccess` resets backoff to base. Correct.
- Half-open rejects second caller (single-probe). Good design choice.

### Issues

1. **No config validation.** `New()` happily accepts `Threshold: 0` (breaker trips on first call before any failure — `consecutiveFail` starts at 0, first `RecordFailure` sets it to 1 which is `>= 0`). Negative threshold, zero durations, `MaxDuration < BaseDuration` — all silently accepted. Add a `Validate() error` or at least clamp in `New()`. **Severity: medium.**

2. **`BaseDuration: 0` means open state instantly transitions to half-open.** Combined with `Threshold: 0`, you get a breaker that's basically a no-op but burns cycles. Not a bug per se, but a foot-gun. **Severity: low.**

3. **`RecordSuccess` unconditionally sets state to `stateClosed`.** If called while already closed (normal path), this also resets `consecutiveFail` and `openDuration`. That means a success after 2 failures (below threshold) resets the failure counter. This is *probably* intentional (success = healthy), but it's undocumented. I'd want a comment. **Severity: nit.**

4. **`openDuration` set twice on first trip.** In `RecordFailure` for `stateClosed`, you set `b.openDuration = b.cfg.BaseDuration`. But `New()` already initializes `openDuration` to `BaseDuration`. When `RecordSuccess` resets it to `BaseDuration`, and then we trip again, we set it again in `RecordFailure`. Harmless redundancy, but the initialization in `New()` is misleading — it suggests `openDuration` is live state, but for a fresh breaker it's never read before being overwritten. **Severity: nit.**

### Tests

Good coverage: closed→open, open→half-open→closed, escalation, max cap, backoff reset, half-open rejection, concurrent access. The `fakeClock` is properly mutex-guarded. **No test for zero/invalid config** (see issue #1).

---

## JSON Parser (`internal/llmutil/json.go`)

### Good

- All 4 strategies preserved: direct parse, ```json fence, generic fence, first-`{`-to-last-`}`.
- Generic constraint `[T any]` is correct — caller picks the type.
- Empty/whitespace input returns early with clear error.
- Cascade ordering is correct (most specific first).

### Issues

5. **Strategy 2 and 3 overlap.** If the input has ` ```json ... ``` `, strategy 2 runs. But if strategy 2's inner content is malformed JSON, we fall through to strategy 3, which will match the *same* ` ```json ``` block (because ` ```json ` starts with ` ``` `). Strategy 3 will then try to parse `json\n{malformed...}` which also fails. This is harmless (we fall through to strategy 4), but it's wasted work. Not worth fixing — just noting for posterity. **Severity: nit.**

6. **No array support.** Strategy 4 uses `{` and `}`. If an LLM returns a JSON array `[...]`, strategies 2/3 would catch it in a fence, but bare `[1,2,3]` outside fences won't be caught by strategy 4. If this is intentional (the function is named `ExtractJSON` with constraint `any` suggesting objects), document it. **Severity: low.**

7. **Multiple JSON blocks.** If the response has two fenced blocks, only the first is tried. Fine for current use cases, but worth a doc comment. **Severity: nit.**

### Tests

10 test cases covering all 4 strategies, nested objects, empty, whitespace, malformed, no-JSON, different types. Decent. **Missing: array extraction test, multiple-fence test.** Consistent with issue #6/#7.

---

## Integration: How Consumers Use the New Packages

### `embedding/embedding.go`

- Uses `circuitbreaker.New` correctly. Maps its own `CircuitBreakerConfig` to `circuitbreaker.Config`. Clean.
- `MaxDuration` hardcoded to `30 * time.Minute` — not configurable. Mildly annoying but acceptable for now.

### `extraction/stage3.go`

- Uses both `circuitbreaker` and `llmutil.ExtractJSON`. Clean integration.
- `parseAndValidate` calls `llmutil.ExtractJSON[criticResponse]` — correct generic usage.

### `extraction/stage2.go`

- Uses `llmutil.ExtractJSON[rubricResponse]` — correct.

### `injection/injector.go`

- Uses `llmutil.ExtractJSON[classificationResponse]` — correct.

---

## Debt: Aliased Sentinels

8. **`ErrCircuitOpen` aliases in `embedding.go` and `stage3.go`.** Both define `var ErrCircuitOpen = circuitbreaker.ErrOpen` "for backward compatibility." This works (`errors.Is` follows the pointer), but it's debt. There are **18 references** across test files. These should migrate to `circuitbreaker.ErrOpen` directly and the aliases should be deleted. The aliases are a crutch — they'll confuse anyone who doesn't know the history. **Severity: medium (tech debt).**

9. **`HalfOpenMax` config field in `embedding.CircuitBreakerConfig` is defined, defaulted, and… never used.** The `circuitbreaker.Config` struct has no `HalfOpenMax` field — the breaker hardcodes single-probe half-open. This is dead config. Remove it or implement it. **Severity: low (dead code/config).**

---

## Old Duplicate Code

10. **No old `parseJSON`/`extractJSON`/`ParseLLMJSON` functions found outside `llmutil`.** ✅ Old duplicate parse functions appear to be deleted.

11. **No old circuit breaker implementations found outside `circuitbreaker` package.** ✅ The embedding and extraction packages both import from the shared package.

---

## Summary

| # | Issue | Severity |
|---|-------|----------|
| 1 | No config validation on Breaker | Medium |
| 2 | Zero BaseDuration foot-gun | Low |
| 3 | RecordSuccess resets failure count unconditionally (undocumented) | Nit |
| 4 | Redundant openDuration init | Nit |
| 5 | Strategy 2/3 overlap on malformed fence | Nit |
| 6 | No bare array support in strategy 4 | Low |
| 7 | Only first fence block tried | Nit |
| 8 | ErrCircuitOpen aliases = tech debt (18 refs) | Medium |
| 9 | HalfOpenMax dead config | Low |

**Bottom line:** Solid extraction. The shared packages are clean, well-tested, and correctly integrated. The state machine is right, the generics are right, old duplicates are gone. Main gripes: validate your damn config inputs (#1), and schedule a follow-up to kill those sentinel aliases (#8). The dead `HalfOpenMax` config (#9) is sloppy — either wire it up or rip it out.

Ship it with #1 fixed. The rest can be follow-ups.
