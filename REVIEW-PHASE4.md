# Phase 4 Review — Stage 3 Critic

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `stage3.go`, `stage3_test.go`  
**Grade:** C+

Decent structure, tests exist and mostly test what they claim, but there's a critical bug in retry logic, missing spec requirements, and gaps in security coverage that Pavel specifically asked for.

---

## 🔴 Critical (must fix before merge)

### 1. `isRetryable` matches any error containing "5"

```go
strings.Contains(msg, "5") // 5xx
```

This matches `"stage2 did not pass"`, `"invalid score: 47 > 5"`, `"5 retries exhausted"`, your grandmother's phone number — literally any error string with a `5` in it. Every non-retryable error that happens to contain the digit 5 gets retried 3 times for free.

**Fix:** Match `"500"`, `"502"`, `"503"`, `"504"` explicitly, or better: use typed errors / status code checking instead of string matching.

### 2. TokensUsed is always 0 (AC-10 failed)

`LLMClient.Complete` returns `(string, error)`. There's no token count in the response. `TokensUsed` is never set anywhere in the code. Pavel called this in paranoia item #7. Either extend the `LLMClient` interface to return token metadata or document that it's always 0 — but don't leave AC-10 silently unfulfilled.

### 3. AC-7 not implemented — timeout escalation

Spec §5.1: *"Timeout (>30s) → Retry once with 60s timeout."* The implementation does generic exponential backoff (1s, 2s, 4s). There's no timeout detection, no escalated timeout on retry. The retry logic doesn't distinguish timeout from other retryable errors at all.

---

## 🟡 Significant (fix before next phase)

### 4. Prompt delimiter injection not tested (Pavel S-4)

Content containing `---END SESSION---` would break the prompt sandboxing. Pavel specifically listed this in §7.1 S-4. No test exists, no escaping exists. Either escape the delimiters in content or use delimiters that can't appear in natural text (XML-style with random nonces, etc.).

### 5. Half-open failure re-open test missing

Pavel's §3.3 has a specific scenario: half-open probe fails → circuit re-opens for *doubled* duration (10 min). The *code* implements this (`openDuration * 2`), but there's no test verifying the re-open duration is actually 10 minutes, not 5. You wrote the code but didn't prove it works.

### 6. No concurrent circuit breaker test (Pavel paranoia #6)

The `sync.Mutex` is there, which is good. But Pavel asked for a test with `sync.WaitGroup` proving two goroutines in half-open don't both become probes. Missing entirely.

### 7. Stage2Result edge case tests missing

Pavel's §4.2 `TestStage3Critic_Stage2InputEdges` — zero novelty score, empty patterns, max scores, min viable scores. None implemented.

### 8. Contradiction detection is half-baked

You catch `extract + all scores = 1` and override to reject. Good. But `reject + all scores = 5` passes through untouched — the LLM says "reject" with perfect scores and you just shrug. The spec test says to at least verify this case is handled consistently. More importantly: what about `extract` with scores like `1,1,1,1,1,1,2`? Your `allScoresAre` check is exact equality — one score of 2 bypasses the entire contradiction check. A composite threshold would be more robust.

---

## 🟢 Minor (nits, cleanup)

### 9. Rate limit retry logic is fragile

```go
if isRateLimit(err) && attempt == maxRetries && maxRetries < 5 {
    maxRetries = 5
}
```

Mutating the loop bound mid-iteration. Works but one refactor away from breaking. Extract to a `maxRetriesFor(err)` function computed upfront, or at least add a comment explaining the trick.

### 10. Truncation backs off byte-by-byte

```go
for len(truncated) > 0 && !utf8.Valid(truncated) {
    truncated = truncated[:len(truncated)-1]
}
```

`utf8.Valid` scans the entire slice each iteration. At the boundary you'll back off at most 3 bytes (max UTF-8 rune is 4 bytes), so it's fine in practice, but `utf8.DecodeLastRune` is the idiomatic approach and O(1).

### 11. Test for "success resets failure counter" is messy

The test creates `llm` and `_ = newTestCritic(llm)` (unused), then creates `llm2` and `critic2`. Dead code in a test file. Clean it up.

### 12. `buildCriticPrompt` marshals Stage2Result without error handling

```go
stage2JSON, _ := json.Marshal(stage2)
```

Stage2Result will always marshal fine, but swallowing errors is a bad habit. At minimum, if it somehow fails, the prompt will contain `null` and the LLM will hallucinate scores.

### 13. No test for non-retryable errors skipping retry

An auth error (`"401 unauthorized"`) should fail immediately without retries. The code does this (falls through `isRetryable` check), but there's no test proving it. Easy to add.

---

## Acceptance Criteria Checklist

| AC | Status | Notes |
|----|--------|-------|
| AC-1 | ✅ | Interface matches spec §2.1 |
| AC-2 | ✅ | Happy path covered |
| AC-3 | ✅ | Score validation, composite recomputation, confidence clamping |
| AC-4 | ⚠️ | Backoff timing correct, but `isRetryable` bug (issue #1) means wrong errors get retried |
| AC-5 | ✅ | Opens at 5, half-open works, but missing re-open duration test |
| AC-6 | ✅ | Malformed → rejection with `critic_parse_error` |
| AC-7 | ❌ | Timeout escalation not implemented |
| AC-8 | ⚠️ | 429 gets 5 retries, but `isRetryable` bug means non-429s also match |
| AC-9 | ✅ | Context cancellation tested |
| AC-10 | ❌ | TokensUsed always 0 |
| AC-11 | ✅ | Structured logging at decision points |
| AC-12 | ✅ | Secrets not in logs at INFO level |
| AC-13 | ✅ | Passed/verdict consistency tested |

**Score: 9/13 pass, 2 partial, 2 fail.**

---

## Test Quality

46 test cases total (exceeds 30 minimum). Table-driven where appropriate. Mocks are clean. Clock injection is good. The tests mostly test what they claim.

**Gaps:**
- No concurrency tests
- No Stage2Result edge cases
- Dead code in circuit breaker reset test
- No delimiter injection test
- No non-retryable error isolation test
- 150KB content test doesn't verify truncation actually happened (just that it didn't error)

---

## Summary

The bones are solid — prompt design, circuit breaker, retry structure, JSON parsing fallbacks are all reasonable. But the `isRetryable` bug is a showstopper that needs fixing before this touches production, TokensUsed is unimplemented, and timeout escalation from the spec is missing entirely. Fix the three reds, add the missing tests Pavel asked for, and this is a B+.
