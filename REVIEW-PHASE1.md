# Phase 1 Review: Embedding Service

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade: B+**

Grudgingly, this is solid work. The code is clean, matches the spec reasonably well, and the tests actually test meaningful state transitions. But I found issues. I always find issues.

---

## Priority 1 — Bugs & Spec Deviations

### P1-1: No distinction between retryable and non-retryable errors

`EmbedBatch` retries on *every* failure, including 400 Bad Request, 401 Unauthorized, and 403 Forbidden. The spec (§5.2) says retry on "Timeout/5xx". A 401 means your API key is wrong — retrying 3 times with backoff won't fix that, it'll just slow down your failure and potentially trigger rate limits.

**File:** `embedding.go`, `EmbedBatch` loop + `doRequest`

**Fix:** Check `resp.StatusCode` in `doRequest`. Return a typed error (e.g., `permanentError`) for 4xx (except 429). In `EmbedBatch`, don't retry permanent errors. Retry 429 and 5xx only.

### P1-2: Circuit breaker counts non-retryable failures

Related to above: a 401 increments `cbFailures`. Five bad API keys in a row trips the circuit breaker, blocking all requests for 5 minutes. That's insane — the breaker should only trip on infrastructure failures (timeouts, 5xx, 429), not client errors.

**File:** `embedding.go`, `EmbedBatch` — `cbRecordFailure()` is called for ALL errors.

### P1-3: Spec says re-open duration doubles, code doesn't implement it

Spec §5.1 (LLM Critic, same pattern applied to embedding per §5.2): "If it fails, re-open for 10 minutes." The circuit breaker in the code always re-opens with the same `OpenDuration`. The spec expects the open duration to increase on half-open failure (5m → 10m). This is missing.

**File:** `embedding.go`, `cbRecordFailure` in `circuitHalfOpen` case.

---

## Priority 2 — Correctness Concerns

### P2-1: `io.ReadAll` with no size limit

`doRequest` calls `io.ReadAll(resp.Body)` with no cap. A misbehaving server (or a man-in-the-middle) could return gigabytes and OOM the process. Use `io.LimitReader`.

**File:** `embedding.go`, `doRequest`

**Fix:** `io.ReadAll(io.LimitReader(resp.Body, 10<<20))` — 10MB is generous for an embedding response.

### P2-2: Response body potentially logged in error messages

```go
return nil, fmt.Errorf("embedding API returned status %d: %s", resp.StatusCode, string(body))
```

If the API returns error details that include the request (some do), this could echo sensitive data (the input texts) into logs. The `slog.Error` call above it also logs `string(body)`. This is less about the API key (which goes in the *request*) and more about user content leaking into structured logs.

**File:** `embedding.go`, `doRequest`

**Fix:** Truncate body in error messages. Log at most 512 bytes.

### P2-3: `EmbedBatch` doesn't validate individual embedding dimensions

The code checks `len(embResp.Data) != len(texts)` and validates index bounds, but doesn't verify that each returned embedding has the expected number of dimensions. A corrupted response with wrong-sized vectors would silently propagate garbage into the store.

**File:** `embedding.go`, `doRequest`, after the sort-by-index loop.

---

## Priority 3 — Design & Idiom Issues

### P3-1: Config field name mismatch with spec

Spec's `EmbeddingConfig` (Appendix A) uses `Dimensions` as the field name. The code uses `Dims`. Minor, but when someone reads the spec and then reads the code, they'll be confused. Pick one.

**File:** `embedding.go`, `Config` struct

### P3-2: `ErrCircuitOpen` is a `var`, should be a sentinel type or use `errors.New`

```go
var ErrCircuitOpen = fmt.Errorf("circuit breaker is open")
```

`fmt.Errorf` creates a new error every call... wait, no, it's a package-level var, so it's created once. Fine. But `errors.New` is idiomatic for sentinel errors. `fmt.Errorf` is for wrapping. Using it without `%w` for a sentinel is a code smell.

**File:** `embedding.go`

### P3-3: `Client` satisfies `Embedder` interface but there's no compile-time check

Add: `var _ Embedder = (*Client)(nil)` at package level. Cheap insurance.

**File:** `embedding.go`

### P3-4: HTTP client timeout is hardcoded

```go
http:   &http.Client{Timeout: 30 * time.Second},
```

This should be in `Config` or at minimum a constant. The spec doesn't specify a timeout value, but hardcoding it in `New()` means you can't tune it without recompiling.

**File:** `embedding.go`, `New()`

---

## Priority 4 — Test Gaps

### P4-1: No test for context cancellation

There's no test verifying that a cancelled context stops the retry loop. This is important — if the parent context is cancelled, you don't want to sit through 3 retries with exponential backoff.

### P4-2: No test for 429 vs 400 behavior

Once P1-1 is fixed, you'll need tests proving that 400 doesn't retry but 429 does. Currently there are no status-code-specific tests.

### P4-3: No concurrent access test for circuit breaker

The circuit breaker uses a mutex, which is correct, but there's no test with concurrent goroutines hitting `cbAllow`/`cbRecordFailure`/`cbRecordSuccess` simultaneously. Run it with `-race`. The mutex *looks* right, but "looks right" is how you get production data races.

### P4-4: No test for empty API key

What happens if `APIKey` is ""? The code skips the Authorization header. Is that intentional? Should it error at construction time? At minimum there should be a test documenting the expected behavior.

### P4-5: Circuit breaker half-open test relies on `time.Sleep`

The half-open tests use `time.Sleep(60 * time.Millisecond)` to wait for the open duration. This is fragile on slow CI machines. Consider injecting a clock (e.g., `now func() time.Time`) into the Client for deterministic testing.

---

## Priority 5 — Consistency with Phase 0

### P5-1: `ScoredSkill` duplicated between packages

`storage.ScoredSkill` and `extraction.ScoredSkill` are identical structs in different packages. This is a consistency landmine — update one, forget the other. One should be canonical and the other should import it, or they should both live in `extraction/types.go`.

**Files:** `storage/store.go` (`ScoredSkill`), `extraction/types.go` (`ScoredSkill`)

### P5-2: Store embeds model name, embedding client doesn't expose it

`SQLiteStore` takes `embeddingModel` as a constructor param and stores it with embeddings. But `Client` doesn't expose its model name — `Embedder` interface only has `Dimensions()`. When wiring Phase 1 into Phase 0's `Put()`, someone will have to manually pass the model string, creating a consistency risk. Consider adding `Model() string` to the `Embedder` interface.

---

## What's Actually Good (Grudgingly)

- Circuit breaker state machine is correct for the implemented behavior (closed → open → half-open → closed/open).
- Mutex usage is correct — lock, defer unlock, no lock held across I/O.
- `doRequest` properly uses `NewRequestWithContext` for context propagation.
- Response index reordering is a nice touch — handles out-of-order API responses correctly.
- Tests are table-driven-adjacent and use `httptest.NewServer` properly.
- `EmbedBatch` with empty input returns early — good.
- Backoff calculation is correct (multiply then cap).

---

## Summary

| Category | Issues |
|---|---|
| Bugs / Spec Deviations | 3 (P1-1, P1-2, P1-3) |
| Correctness | 3 (P2-1, P2-2, P2-3) |
| Design / Idioms | 4 (P3-1 through P3-4) |
| Test Gaps | 5 (P4-1 through P4-5) |
| Cross-Phase Consistency | 2 (P5-1, P5-2) |

**Grade: B+** — The architecture is sound and the code is readable. The circuit breaker and retry loop work correctly for the happy path. But the lack of retryable/non-retryable error distinction (P1-1, P1-2) is a real bug that will cause problems in production, and the missing escalating open duration (P1-3) is a spec deviation. Fix P1-* before merging. The rest can be follow-up.
