# Phase 0 Storage Layer Review — Round 2

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Previous Grade:** C+  
**Updated Grade: B+**

---

## Original Issues — Verification

### Issue #1: `SELECT *` — ✅ FIXED
`skillColumns` constant enumerates all 24 columns explicitly. Used consistently in `Get`, `GetLatestByName`, `Query`, and `ListByDecay`. Single source of truth. This is correct.

### Issue #2: `body []byte` Ignored in `Put()` — ✅ FIXED
`Put()` now writes body to `skill.FilePath` via `os.MkdirAll` + `os.WriteFile`. Guarded by `len(body) > 0 && skill.FilePath != ""`. New test `TestPutWritesBodyToDisk` validates it. Git wiring noted as deferred — that's acceptable for Phase 0.

One nit: the file write happens *between* the INSERT and the tx.Commit(). If the file write succeeds but tx.Commit() fails, you have an orphaned file on disk with no DB record. If the file write fails, the deferred `tx.Rollback()` cleans up the DB — good. But the reverse isn't handled. Minor for now, flag it for Phase 1.

### Issue #3: Missing `SessionRecord` and Extraction Types — ✅ FIXED
`types.go` now has: `SessionRecord`, `ExtractionStatus` (with all 7 constants), `ExtractionResult`, `Stage1Result`, `Stage2Result`, `Stage3Result`, `InjectionRequest`, `InjectionResponse`, `ScoredSkill`, `PromptClassification`. Comprehensive. Fields match the schema. I'll grudgingly say this is well done.

### Issue #4: `message_count` vs `total_calls` Mismatch — ✅ FIXED
Schema now has `message_count INTEGER NOT NULL` in the sessions table. `total_calls` is gone. `SessionRecord.MessageCount` maps correctly. `tokens_used`, `agent_model`, `user_id` all present with sensible defaults.

### Issue #5: N+1 Query in `Query()` — ✅ FIXED
New `loadPatternsMulti()` and `loadEmbeddingsMulti()` methods do batch `WHERE skill_id IN (...)` queries. Query flow is now 3 queries total (skills + patterns + embeddings) regardless of result set size. Correct.

### Issue #6: `UpdateUsage()` / Missing `injection_events` Insert — ✅ FIXED (documented)
Comment now explicitly states: "injection_events rows are inserted by the injection pipeline (Phase 6). Until that phase is implemented, this subquery returns 0.0." The correlation update is also gated by `outcome == "success" || outcome == "failure"`, so it doesn't run needlessly on empty outcomes. Acceptable — the dead code is now *documented* dead code.

### Issue #7: String Errors Instead of Sentinels — ✅ FIXED
`ErrNotFound` and `ErrDuplicate` are proper sentinel errors. `scanSkillFrom` returns `ErrNotFound` via `errors.Is(err, sql.ErrNoRows)` check. `Put()` wraps duplicate constraint as `fmt.Errorf("%w: ...", ErrDuplicate)`. New `TestSentinelErrors` validates both. Textbook.

### Issue #8: Hardcoded Embedding Model — ✅ FIXED
`embeddingModel` is now a field on `SQLiteStore`, set via `NewSQLiteStore(dsn, embeddingModel)`. Falls back to `"text-embedding-3-small"` only when empty string passed. Tests validate both configurable and default paths. Correct.

---

## New Issues Introduced by Fixes

### N1. File Write Outside Transaction Scope (Minor)
**File:** `store.go`, `Put()`, ~line 138-144  
As noted above, `os.WriteFile` happens inside the transaction block but *isn't* transactional. DB rollback doesn't unlink the file. This is a known limitation of mixing filesystem + DB ops, and not easily fixable without a WAL-style approach. Just be aware.

### N2. `loadPatternsMulti` / `loadEmbeddingsMulti` — Unbounded IN Clause
If `Query()` returns 10,000 candidate skills, you'll build a `WHERE skill_id IN (?, ?, ?, ... x10000)` clause. SQLite has a limit of 999 bound parameters by default (though modernc/sqlite may differ). Consider chunking at ~500 for safety. Not a problem today with small data sets, but it's a latent bomb.

### N3. `Query()` Still Doesn't Filter to Latest Version
**Original issue #12 (medium priority).** Not in my top 8, but I did flag it. `Query()` can still return v1 and v2 of the same skill. The spec says injection picks latest version. This should be addressed before Phase 6 wires up injection.

---

## Previously Flagged Medium/Low Issues — Status

| # | Issue | Status |
|---|-------|--------|
| 9 | Duplicate scan functions | ✅ Fixed — unified `scanner` interface + `scanSkillFrom` |
| 10 | `ExtractionConfig` stub | Not addressed — still 3 fields. Acceptable for Phase 0. |
| 11 | No context in `NewSQLiteStore` | Not addressed. Acceptable for now. |
| 12 | No version filtering in Query | Not addressed. See N3 above. |
| 13 | Insertion sort | ✅ Fixed — uses `slices.SortFunc` |
| 14 | Weird string(rune) IDs | ✅ Fixed — uses `fmt.Sprintf("id-%d", i)` |
| 15 | No concurrency test | Not addressed. Acceptable for Phase 0. |
| 16 | No correlation test | Not addressed, but documented as deferred to Phase 6. |
| 17 | No input validation in Put | Not addressed. Should happen before Phase 1. |

---

## Summary

All 8 critical/high issues are properly fixed. The fixes are clean — no sloppy hacks, no regressions. The new batch-loading methods are correct. The sentinel errors are idiomatic. The types are comprehensive. The test coverage expanded to match (body write test, embedding model tests, sentinel error tests).

New issues are minor (file-outside-tx, unbounded IN clause, version filtering). None are blockers.

| Category | R1 | R2 | Notes |
|----------|----|----|-------|
| Spec compliance | C | B+ | Types complete, schema aligned, body writes |
| Go idioms | B- | A- | Sentinels, scanner interface, slices.SortFunc |
| SQL correctness | B | B+ | Batch queries, explicit columns, documented gaps |
| Test quality | B- | B | New tests for fixes, still missing concurrency/validation |
| Bugs | C | B+ | N+1 gone, sentinels work, file writes work |
| Security | A- | A- | Unchanged — still solid |

**Overall: B+**

You listened, you fixed everything I asked for, and you didn't break anything doing it. The code is now production-adjacent rather than demo-grade. I'd still want input validation and version filtering before Phase 1 closes, but this is a clean merge for Phase 0.

Don't let it go to your head.

— Jazz
