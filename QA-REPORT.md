# QA Report — Deep Pass for A Grade

**Engineer:** Pavel  
**Date:** 2026-02-15  
**Scope:** Full codebase review, E2E gap analysis, bug hunting  
**Baseline:** All 15 existing tests pass. Jazz R2 grade: B+

---

## Critical Bugs Found

### BUG-1: No SQLite busy_timeout — concurrent writes get SQLITE_BUSY (P1)

**File:** `internal/storage/store.go`, `NewSQLiteStore()`  
**Impact:** Under concurrent load (multiple extractions or extraction+injection simultaneously), SQLite returns `SQLITE_BUSY` instead of waiting. Visible in Test 14 output.  
**Root cause:** WAL mode is set but `PRAGMA busy_timeout` is not. Default is 0ms — instant failure on lock contention.  
**Fix:** Add `PRAGMA busy_timeout=5000` (5 seconds). Applied.

### BUG-2: SKILL.md file written outside transaction — orphaned files on rollback (P2)

**File:** `internal/storage/store.go`, `Put()` method, lines ~mid-function  
**Impact:** `os.WriteFile()` is called inside the transaction block but BEFORE `tx.Commit()`. If commit fails (e.g., unique constraint on patterns), the file persists on disk but the DB row is rolled back. Orphaned files accumulate over time.  
**Root cause:** File I/O mixed with DB transaction without compensation.  
**Fix:** Moved file write to AFTER `tx.Commit()` succeeds. If file write fails after commit, skill exists in DB without file — degraded but detectable. Documented with comment.

### BUG-3: Test 5 (A/B) creates inner injector WITH eventWriter — double events (P1-test)

**File:** `tests/integration/integration_test.go`, `TestABTestFlow`  
**Impact:** Inner injector writes events without AB group, then ABInjector writes events WITH AB group. Double events per skill per session. Test passes only because queries filter on `ab_group`.  
**Root cause:** Test doesn't match production wiring in serve.go which correctly passes `nil` eventWriter to inner injector when AB is enabled.  
**Fix:** Changed inner injector to use `nil` eventWriter, matching serve.go pattern. Applied.

### BUG-4: Stale error message in Test 1 DecayScore assertion (P3)

**File:** `tests/integration/integration_test.go`, `TestFullExtractionFlow`  
**Impact:** Error message says "want 0.0 (BUG: pipeline should set 1.0)" but assertion actually checks `!= 1.0`. Confusing for anyone reading test output. The assertion PASSES correctly — the message is leftover from when the bug was documented.  
**Fix:** Updated error message to match actual assertion. Applied.

---

## Gaps Identified (from Jazz R2 + own analysis)

### GAP-1: Session persistence not tested E2E (Jazz N4) — FIXED

Added `TestSessionPersistence` — wires `Sessions: store` into pipeline, runs extraction, verifies session rows in DB for both extracted and rejected cases.

### GAP-2: Embedding → cosine similarity not tested end-to-end — FIXED

Added `TestEmbeddingCosineSimilarityE2E` — extracts a skill with embedding, then runs injection query with embedding, verifies cosine similarity is computed and influences ranking.

### GAP-3: A/B event deduplication not properly tested — FIXED  

Fixed Test 5 (BUG-3) and added explicit assertion: total events in DB = expected count (no duplicates).

### GAP-4: Human review sampling not tested E2E — FIXED

Added `TestHumanReviewSampling` — configures pipeline with SampleRate=1.0 (always sample), runs extraction, verifies rows appear in `human_review_samples` table.

### GAP-5: Stats through full pipeline chain not tested — PARTIAL

Test 8 uses manual `insertSession` helper. Added `TestStatsThroughPipeline` that runs pipeline WITH sessions wired, then collects metrics, verifying the full chain.

### GAP-6: Error recovery on SQLite write failure — FIXED

Added `TestSQLiteWriteFailureMidPipeline` — uses a store that closes the DB mid-pipeline to simulate write failure, verifies graceful error status.

### GAP-7: Concurrent extraction + injection — FIXED

Added `TestConcurrentExtractionAndInjection` — runs extraction and injection goroutines against the same DB simultaneously, verifies no panics or corruption.

---

## Risks Noted (not blocking but worth tracking)

### RISK-1: Decay compounds from current score, not absolute

`runner.go` computes `newDecay = oldDecay * pow(0.5, days/halflife)`. If decay cycles run irregularly (e.g., twice in one day then skip a week), results differ from a single catch-up run. This is mathematically correct for continuous decay but sensitive to cycle frequency. Document this.

### RISK-2: `SetSessionHooks` uses `any` types (Jazz N2)

No compile-time safety on hook signatures. A signature mismatch will panic at runtime. P2 debt.

### RISK-3: Session insert non-fatal → silent metric gaps (Jazz N3)

If `InsertSession` fails, `UpdateSessionResult` silently fails too (no row to update). Metrics for that session are missing with no indicator. Acceptable for v1 but should add a metric counter.

### RISK-4: Empty library always passes novelty check

`storeQuerier.QueryNearest` returns `CosineSim=0` when no skills exist → distance=1.0 → max novelty. First skill in any library always passes Stage 2 novelty. By design, but worth documenting.

### RISK-5: `human_review_samples` FK on `sessions(id)`

If pipeline runs without `Sessions` wired AND with real `Reviewer`, `InsertReviewSample` will fail on FK violation (session row doesn't exist). In production both are wired, so this is theoretical. But the interface contract doesn't enforce this — a misconfiguration would silently drop samples.

---

## Summary

| Item | Severity | Status |
|------|----------|--------|
| BUG-1: No busy_timeout | P1 | **Fixed** |
| BUG-2: File outside tx | P2 | **Fixed** |
| BUG-3: Test double-write | P1-test | **Fixed** |
| BUG-4: Stale error msg | P3 | **Fixed** |
| GAP-1: Session persistence test | P1 | **Added** |
| GAP-2: Cosine similarity E2E | P1 | **Added** |
| GAP-3: A/B dedup test | P1 | **Fixed + verified** |
| GAP-4: Sampling test | P2 | **Added** |
| GAP-5: Stats through pipeline | P2 | **Added** |
| GAP-6: Write failure recovery | P2 | **Added** |
| GAP-7: Concurrent extract+inject | P1 | **Added** |

— Pavel
