# Phase 4 Review — Round 2

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `stage3.go`, `stage3_test.go`, `stage2.go` (LLMClientV2 consistency check)  
**Previous Grade:** C+  
**Grade:** B+

---

## P1 Fixes from R1 — Status

### ✅ #1: `isRetryable` no longer matches any string with "5"

Fixed properly. Now uses `errors.As` for `*LLMError` with explicit status code checks (429, 500-599), plus `isTimeout()` helper, plus narrow string fallback (`"unavailable"` only). The old `strings.Contains(msg, "5")` is gone. Test coverage is thorough — 15 cases including wrapped errors, plain strings with "5", and non-retryable status codes.

### ✅ #2: TokensUsed no longer always 0

Two-path solution: `LLMClientV2` interface with `CompleteWithTimeout` returns actual token counts; fallback `LLMClient` path uses `estimateTokens()` (~4 chars/token). Not perfect but not zero. Test verifies V2 returns exact counts (280) and basic path returns nonzero. AC-10 satisfied.

### ✅ #3: AC-7 timeout escalation implemented

`callWithRetry` now tracks whether the last error was a timeout via `isTimeout()`. First attempt uses 30s, retry after timeout uses 60s. `callLLM` accepts a timeout parameter and either uses `LLMClientV2.CompleteWithTimeout` or wraps the basic client in `context.WithTimeout`. Test captures actual timeout values passed and verifies 30s→60s escalation. Spec §5.1 satisfied.

---

## P2 Fixes from R1 — Status

### ✅ #4: Delimiter injection addressed

Nonce-based delimiters: `---BEGIN SESSION {hex}---` / `---END SESSION {hex}---`. `sanitizeDelimiters` replaces any occurrence of the actual nonce-based delimiters in content with `[SANITIZED_DELIMITER]`. Test verifies exactly 1 begin + 1 end delimiter in the final prompt.

### ✅ #5: Half-open failure re-open test added

`TestStage3Critic_HalfOpenFailureDoublesDuration` — opens circuit, advances past 5 min, fails probe, verifies still open at +5 min (doubled to 10 min), verifies succeeds after +10 min. Clean.

### ✅ #6: Concurrent circuit breaker test added

`TestStage3Critic_ConcurrentHalfOpen` — two goroutines hit half-open simultaneously with `sync.WaitGroup`. Comment acknowledges the mutex serializes the check not the full call. Designed to catch races under `-race`. Acceptable.

### ✅ #7: Stage2Result edge cases added

`TestStage3Critic_Stage2InputEdges` — zero novelty, empty patterns, max scores, min viable scores. All four cases from Pavel §4.2.

### ✅ #8: Contradiction detection improved

Replaced exact `allScoresAre(q, 1)` with `averageScore(q) < 1.5` for the extract-with-low-scores case. Added `allScoresAbove(q, 4)` for the reject-with-high-scores case (catches 4s and 5s, not just exact 5). `TestStage3Critic_ContradictionEdgeCases` covers 6 cases including the previously-missing "one score of 2" scenario and "reject with all 4s". Good.

---

## R1 Minor Issues — Status

### ✅ #9: Rate limit retry refactored

`maxRetriesFor()` function extracted. Still mutates `maxRetries` in the loop for the 429 escalation case, but the structure is cleaner.

### ✅ #10: Truncation uses `DecodeLastRune`

Fixed. Backs off at most 3 bytes using a bounded loop with `utf8.DecodeLastRune`. O(1) per iteration.

### ✅ #11: Dead code in test cleaned

`TestStage3Critic_CircuitBreaker/success resets failure counter` no longer has unused variables.

### ✅ #12: `buildCriticPrompt` handles marshal error

```go
stage2JSON, err := json.Marshal(stage2)
if err != nil {
    stage2JSON = []byte("{}")
}
```

Good enough.

### ✅ #13: Non-retryable error test added

`TestNonRetryableErrorSkipsRetry` — 401 error, verifies exactly 1 call. Clean.

---

## New Issues Found in R2

### 🟡 1. `sanitizeDelimiters` only sanitizes the nonce-based delimiters, not generic patterns

The `sanitizeDelimiters` function replaces exact matches of `beginDelim` and `endDelim` (which contain the random nonce). If content contains `---BEGIN SESSION abc123---` and the generated nonce is `def456`, the content's delimiters pass through unsanitized. In practice this is fine because the nonce makes collisions astronomically unlikely — but `sanitizeDelimiters` as written will literally never trigger because the nonce is generated *after* content exists. The function is dead code. Not a security issue (nonces protect you), but dead code pretending to be a security measure is misleading.

### 🟡 2. `estimateTokens` is a rough approximation with no documentation of its limitations

4 chars/token is a crude heuristic that varies wildly by language, code vs prose, tokenizer version. It's fine as a fallback, but should have a doc comment noting it's estimation-only and consumers should prefer `LLMClientV2` for accurate counts. Minor, but if someone reads `TokensUsed` downstream and makes billing decisions on it, they'll be in for a surprise.

### 🟡 3. Circuit breaker `recordFailure` is called once per `Evaluate`, but each `Evaluate` may have 4+ internal retry attempts

If the LLM is genuinely down, one `Evaluate` call does 4 retries internally, then increments `consecutiveFail` by 1. So it takes 5 `Evaluate` calls (= 20 actual LLM calls) to trip the breaker. This is probably intentional (circuit breaker operates at the caller level, not the transport level), but it's undocumented and could surprise someone expecting 5-failure trip to mean 5 actual failed requests.

### 🟢 4. `maxRetriesFor` always returns 3

```go
func maxRetriesFor(_ error) int {
    return 3
}
```

Takes an error parameter it ignores. Leftover from a refactor? Either use the parameter or make it a constant. Nit.

### 🟢 5. Concurrent half-open test is weak

The test acknowledges both goroutines may get through `checkCircuit`. It only verifies "at least one probe call" and "no panics." That's a race detector test, not a behavior test. The comment says "The important thing is no panics or data races" — fair, but Pavel asked for proof that two goroutines in half-open don't *both* become probes. This test doesn't prove that. It proves they don't crash. Half credit.

---

## stage2.go — LLMClientV2 Consistency Check

Stage2 still uses `LLMClient` (the basic interface). It does not use `LLMClientV2`. This is **fine** — stage2's rubric call doesn't need timeout escalation or token tracking (those are stage3-specific ACs). The `LLMClientV2` interface extends `LLMClient`, so any V2 implementation passed to stage3 can also serve stage2 through the base interface. No inconsistency.

---

## Acceptance Criteria Checklist (Updated)

| AC | Status | Notes |
|----|--------|-------|
| AC-1 | ✅ | Interface matches spec §2.1 |
| AC-2 | ✅ | Happy path covered |
| AC-3 | ✅ | Score validation, composite recomputation, confidence clamping |
| AC-4 | ✅ | `isRetryable` fixed, proper status code checking |
| AC-5 | ✅ | Opens at 5, half-open works, re-open duration tested |
| AC-6 | ✅ | Malformed → rejection with `critic_parse_error` |
| AC-7 | ✅ | Timeout escalation 30s→60s implemented and tested |
| AC-8 | ✅ | 429 gets 5 retries, clean separation from other errors |
| AC-9 | ✅ | Context cancellation tested |
| AC-10 | ✅ | TokensUsed populated (exact via V2, estimated via fallback) |
| AC-11 | ✅ | Structured logging at decision points |
| AC-12 | ✅ | Secrets not in logs at INFO level |
| AC-13 | ✅ | Passed/verdict consistency tested |

**Score: 13/13 pass.**

---

## Test Quality

~65 test cases (up from 46). All P1/P2 gaps from R1 addressed. Table-driven where appropriate. Good fixture helpers. Clock/sleep injection is clean. Contradiction edge cases are thorough.

**Remaining gaps:**
- Concurrent half-open test proves safety (no races) but not exclusivity (only one probe)
- `sanitizeDelimiters` effectively dead code — no test can trigger it because nonce is unique
- No test for `estimateTokens` accuracy bounds

---

## Summary

All three critical issues fixed. All significant issues addressed. All minor nits cleaned up. The `isRetryable` rewrite using typed `LLMError` is the right approach — should've been there from the start. Timeout escalation is clean. Contradiction detection with `averageScore` threshold is more robust than exact equality.

New issues are all yellow/green — no blockers. The `sanitizeDelimiters` dead code and the weak concurrency test are the most notable, neither is merge-blocking.

From C+ to B+. The fixes are correct and the test coverage is materially better. Not an A because the concurrent half-open test doesn't actually prove what Pavel asked for, and there's dead code masquerading as security logic.
