# T3 QA Report — `internal/queue/` package

**Author:** Pavel 🇷🇺 (QA/SDET)  
**Date:** 2026-02-17  
**Branch:** `feat/t3-queue-package-v2`

## Tests Added (5)

| Test | What it verifies | Result |
|------|-----------------|--------|
| `TestFailPermanent` | Enqueue → claim → FailPermanent. Entry gets status `failed`, not claimable. | ✅ PASS |
| `TestDoubleComplete` | Complete same session twice. Idempotent (no error, stays completed). | ✅ PASS |
| `TestDuplicateEnqueue` | Enqueue same session_id twice. PK violation error on second call. Depth stays 1. | ✅ PASS |
| `TestFailAfterComplete` | Complete then Fail. After fix: Fail is a no-op, status remains `extracted`. | ✅ PASS |
| `TestClaimFromEmptyQueue` | Claim with nothing queued. Returns `nil, nil` gracefully. | ✅ PASS |

## Bug Found & Fixed

**`Fail()` and `FailPermanent()` could overwrite terminal statuses.**

Both methods had an unconditional `WHERE session_id = ?`, meaning calling `Fail` on an already-`extracted` entry would flip it back to `error` and make it retryable. This is a state machine violation.

**Fix:** Added `AND status = 'pending'` guard to both `Fail` and `FailPermanent` UPDATE queries. Only entries currently being worked on (status `pending`) can transition to error/failed states.

## Full Suite Results

- `go test -race ./internal/queue/...` — **12/12 PASS**
- `go test -race ./...` — **20/20 packages PASS**
- `go vet ./...` — **clean**

## Test Coverage Summary (queue package)

| Area | Coverage |
|------|----------|
| Enqueue → Claim → Complete lifecycle | ✅ |
| Fail + retry with backoff | ✅ |
| FailPermanent (dead letter) | ✅ (new) |
| Backpressure | ✅ |
| Stale requeue | ✅ |
| Concurrent claims | ✅ |
| Session metadata CRUD | ✅ |
| Double complete (idempotent) | ✅ (new) |
| Duplicate enqueue (PK error) | ✅ (new) |
| Fail after complete (no-op) | ✅ (new) |
| Claim from empty queue | ✅ (new) |
