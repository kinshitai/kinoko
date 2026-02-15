# Review: R2 — Extract `internal/llm` Package

**Reviewer:** Jazz
**Date:** 2026-02-15
**Grade:** C+

Builds. Tests pass. The move happened. But there are real issues.

---

## Summary

The `internal/llm` package was created with four files: `client.go`, `errors.go`, `openai.go`, `llm_test.go`. LLM interfaces and error types moved out of `extraction/stage3.go`. The OpenAI HTTP client moved out of `cmd/mycelium/extract.go`. `helpers.go` was removed (confirmed gone). Both `extract.go` and `serve.go` now import `internal/llm`. Build passes, all unit tests pass. The one integration test failure (`TestWorkerPoolE2E`) is a pre-existing SQLite BUSY race — unrelated.

So far so good. Now the problems.

---

## Issues

### 🔴 P0: `OpenAIClient` returns `fmt.Errorf` instead of `*LLMError` on non-200

**File:** `internal/llm/openai.go:77`

```go
return "", fmt.Errorf("openai API %d: %s", resp.StatusCode, string(body[:n]))
```

This is a **bug**. You built an entire `LLMError` type with `StatusCode` field. You built `IsRetryable`, `IsRateLimit`, `IsTimeout` functions that do `errors.As(err, &llmErr)` to check the status code. And then the *only concrete LLM client in the codebase* doesn't return `*LLMError`. It returns a plain `fmt.Errorf`.

Result: structured error classification never fires for HTTP errors. A 429 from OpenAI gets caught by the fallback `strings.Contains(msg, "rate limit")` — maybe. A 503 might match `strings.Contains(msg, "unavailable")` — maybe not. You're relying on OpenAI's error message text instead of the status code you already have.

**Fix:** Line 77 should be:
```go
return "", &LLMError{StatusCode: resp.StatusCode, Message: string(body[:n])}
```

This is the entire point of having `LLMError`. Use it.

---

### 🟡 P1: Type aliases left in `extraction/stage2.go` and `stage3.go` — unjustified

**Files:** `internal/extraction/stage2.go:53`, `internal/extraction/stage3.go:22-28`

```go
type LLMClient = llm.LLMClient       // stage2.go
type LLMError = llm.LLMError         // stage3.go
type LLMCompleteResult = llm.LLMCompleteResult  // stage3.go
type LLMClientV2 = llm.LLMClientV2   // stage3.go
```

Four type aliases. Zero external consumers reference `extraction.LLMClient` or `extraction.LLMError` — I checked. These aliases exist solely for internal convenience within stage2 and stage3 files so they don't have to write `llm.LLMClient`.

That's not what type aliases are for. Type aliases are for backward compatibility during migrations where external callers use the old path. Nobody does here. These are laziness aliases.

**Fix:** Delete all four aliases. Use `llm.LLMClient`, `llm.LLMError`, etc. directly. The import is already there.

---

### 🟡 P2: Delegation wrappers for `isRetryable`/`isRateLimit`/`isTimeout` in stage3.go — pointless

**File:** `internal/extraction/stage3.go:285-292`

```go
func isRetryable(err error) bool { return llm.IsRetryable(err) }
func isTimeout(err error) bool { return llm.IsTimeout(err) }
func isRateLimit(err error) bool { return llm.IsRateLimit(err) }
```

Three unexported wrappers that do nothing except delegate. They exist so the call sites in `callWithRetry` don't have to change from `isRetryable(err)` to `llm.IsRetryable(err)`. That's 3 lines of code to avoid changing 3 other lines of code.

There's also a test in `stage3_test.go` (line 889) that tests `isRetryable` — which now tests a wrapper that calls `llm.IsRetryable`. The real tests are in `llm_test.go`. So you have duplicate test coverage of the same function through a transparent wrapper.

**Fix:** Delete the wrappers. Call `llm.IsRetryable(err)` etc. directly. Delete or update the redundant test.

---

### 🟡 P3: `OpenAIClient` doesn't implement `LLMClientV2`

**File:** `internal/llm/openai.go`

The plan said to move `openAILLMClient` + `openAIComplete` from `cmd/`. You did. But `OpenAIClient` only implements `LLMClient.Complete()`. It doesn't implement `LLMClientV2.CompleteWithTimeout()`.

In `stage3.go:89`, the constructor does a type assertion:
```go
if v2, ok := llm.(LLMClientV2); ok {
    c.llmV2 = v2
}
```

Since `OpenAIClient` doesn't implement V2, this always falls through to the basic `Complete()` path with a manual `context.WithTimeout` wrapper. You lose token usage tracking — `estimateTokens` guesses from string length instead.

This isn't a regression (the old `openAIComplete` didn't implement V2 either), but R2 was the time to fix it. The response struct from OpenAI includes `usage.prompt_tokens` and `usage.completion_tokens`. You already decode the response body. Add the fields.

**Fix:** Add `CompleteWithTimeout` to `OpenAIClient` that reads token counts from the OpenAI response `usage` field.

---

### 🟢 P4: Dead comment in `extract.go`

**File:** `cmd/mycelium/extract.go:257`

```go
// openAILLMClient and openAIComplete moved to internal/llm package.
```

Tombstone comments are not documentation. They're litter. In six months nobody will care where something was moved from. The git log knows.

**Fix:** Delete the comment.

---

### 🟢 P5: No validation in `NewOpenAIClient`

**File:** `internal/llm/openai.go:30-37`

The test `TestNewOpenAIClient_EmptyAPIKey` has a comment: "Constructor doesn't validate — caller's responsibility." That's a choice, but a bad one. An empty API key will produce a confusing 401 error at call time instead of a clear error at construction time. Same for empty model.

**Fix:** Return an error (or at minimum panic) if `apiKey` is empty. Yes, this changes the constructor signature. Do it now while there are exactly 3 call sites.

---

### 🟢 P6: `max_tokens` hardcoded to 2048

**File:** `internal/llm/openai.go:54`

```go
"max_tokens": 2048,
```

Not configurable. The critic prompt in stage3 could easily need more for verbose responses. Should be an option or at minimum a field on `OpenAIClient` set via `Option`.

---

## What Was Done Right

- Clean file separation: interfaces in `client.go`, errors in `errors.go`, implementation in `openai.go`
- Functional options pattern for `OpenAIClient` (`WithHTTPClient`)
- No global HTTP client (each `OpenAIClient` has its own with a 60s default timeout)
- Error classification functions properly exported (`IsRetryable`, `IsRateLimit`, `IsTimeout`)
- Test coverage for error classification is thorough (14 test cases for `IsRetryable` alone)
- `helpers.go` removed as planned
- Both `extract.go` and `serve.go` properly import and use `llm.NewOpenAIClient`

## Verdict

The structural move is correct. The package boundaries are right. But the flagship feature — structured error handling via `LLMError` — is broken because the only client doesn't use it. That's a P0 in a package whose reason for existence is "unify LLM error handling." The type aliases and delegation wrappers are cleanup debt that should have been resolved in this PR, not deferred.

**Grade: C+** — Correct direction, incomplete execution.

---

*The code moved. The bugs moved with it. — Jazz, Feb 2026*
