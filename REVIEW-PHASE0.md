# Phase 0 Storage Layer Review

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade: C+**

Decent scaffolding. The bones are there, the SQL is mostly correct, tests exist and pass the sniff test. But there are real bugs, spec deviations, and design choices that'll bite you in later phases. Let me count the ways.

---

## Critical Issues (Fix Before Merge)

### 1. `SELECT *` is a Ticking Time Bomb
**File:** `store.go`, lines ~147, ~160, ~175, ~215  
**What:** Every query uses `SELECT * FROM skills` and relies on column ordering matching the `Scan()` call.  
**Why it's bad:** Add a column, reorder the schema, or forget to update one of the TWO duplicate scan functions — silent data corruption. I've seen this kill production systems.  
**Fix:** Enumerate columns explicitly:
```sql
SELECT id, name, version, parent_id, library_id, category, ... FROM skills WHERE ...
```

### 2. `body []byte` Parameter in `Put()` Is Completely Ignored
**File:** `store.go`, `Put()` method  
**Spec says (§2.1):** "Put stores a new skill. Computes embedding, writes SKILL.md to git, inserts DB record."  
**What happens:** The `body` parameter is accepted and thrown on the floor. No file write, no git interaction.  
**Impact:** The entire file-persistence half of `Put()` is missing. Either implement it or document that it's deferred — but don't accept a parameter you ignore. That's a lie in the API.

### 3. `SessionRecord` and `ExtractionResult` Types Missing from `types.go`
**File:** `internal/extraction/types.go`  
**Spec §1.2, §1.3:** Defines `SessionRecord`, `ExtractionStatus`, `ExtractionResult`, `Stage1Result`, `Stage2Result`, `Stage3Result` — all part of the data model.  
**What's there:** Only `SkillRecord`, `QualityScores`, and `SkillCategory`.  
**Impact:** The `sessions` table exists in `schema.sql` with no corresponding Go types. Phases 2-5 will need these. They should ship with Phase 0 since they're the data model, not pipeline logic.

### 4. Spec vs Schema Mismatch: `message_count` vs `total_calls`
**File:** `schema.sql`, sessions table  
**Spec §1.2:** `SessionRecord` has field `MessageCount int` with db tag `message_count` — "total user+assistant message turns"  
**Schema:** Has `total_calls INTEGER NOT NULL` instead  
**These are different things.** `message_count` is conversation turns; `total_calls` is... what? Tool calls? That's already `tool_call_count`. This column is either misnamed or wrong.

---

## High Priority Issues

### 5. N+1 Query Problem in `Query()`
**File:** `store.go`, `Query()` method (~line 175-240)  
**What:** Fetches ALL candidate skills, then loops through each one calling `loadPatterns()` and `loadEmbedding()` individually. That's 1 + 2N queries.  
**With 1000 skills:** 2001 queries. SQLite is fast but this is embarrassing.  
**Fix:** JOIN patterns in the main query, or at minimum batch-load with `WHERE skill_id IN (...)`. Embeddings can be loaded in a single query too.

### 6. `UpdateUsage()` Recomputes Correlation But Nothing Creates `injection_events`
**File:** `store.go`, `UpdateUsage()` method  
**What:** The success_correlation subquery reads from `injection_events`, but `SkillStore` has no method to insert injection events. The correlation will always be 0.0.  
**Fix:** Either add `RecordInjection(ctx, event)` to the interface, or note this as a known gap. The subquery is correct SQL but dead code without the insert path.

### 7. Error Handling: String Errors Instead of Sentinel Types
**File:** `store.go`, `scanSkill()` function  
**What:** `return nil, fmt.Errorf("skill not found")` — a plain string error.  
**Why it's bad:** Callers can't reliably check for "not found" without string matching. Every Go developer over the age of 12 knows to use sentinel errors.  
**Fix:**
```go
var ErrNotFound = errors.New("skill not found")
// ...
return nil, ErrNotFound
```
Then callers use `errors.Is(err, storage.ErrNotFound)`.

### 8. Hardcoded Embedding Model in `Put()`
**File:** `store.go`, `Put()`, line ~125  
**What:** `"text-embedding-3-small"` is hardcoded when inserting into `skill_embeddings`.  
**Spec §2.2:** The `Embedder` interface returns model info. The `skill_embeddings` table has a `model` column specifically to track which model produced the embedding.  
**Fix:** Either accept the model name as a parameter, add it to `SkillRecord`, or accept an `Embedder` dependency. Don't hardcode it — when someone switches to `text-embedding-3-large`, you'll have mismatched rows with no way to tell.

---

## Medium Priority Issues

### 9. Duplicate Scan Functions
**File:** `store.go`, `scanSkill()` and `scanSkillRows()`  
**What:** Two nearly identical functions — one for `*sql.Row`, one for `*sql.Rows`. Same 24-field scan list duplicated.  
**Why it's bad:** Change one, forget the other. Classic copy-paste drift.  
**Fix:** Use a scanner interface or have `scanSkillRows` delegate. The `sql.Row` type implements a compatible `Scan` method — use a shared helper:
```go
type scanner interface { Scan(dest ...any) error }
func scanSkillFrom(s scanner) (*extraction.SkillRecord, error) { ... }
```

### 10. `ExtractionConfig` in `config.go` Is a Stub
**File:** `internal/config/config.go`, `ExtractionConfig` struct  
**Spec Appendix A:** Defines ~20 fields (stage thresholds, retry config, circuit breaker, A/B test, etc.)  
**What's there:** 3 fields (`AutoExtract`, `MinConfidence`, `RequireValidation`).  
**Impact:** When later phases need config, someone will either bloat this struct or create a parallel config — both are bad. Plan the extension now.

### 11. No `context.Context` Timeout/Cancellation Handling
**File:** `store.go`, all methods  
**What:** Context is passed through to DB calls (good), but `NewSQLiteStore()` runs PRAGMAs and schema migration without any context. If the DB is on a slow/hung NFS mount, this blocks forever.  
**Fix:** Accept a context in `NewSQLiteStore()` or at minimum set a connection timeout.

### 12. Query Doesn't Filter to Latest Version Only
**File:** `store.go`, `Query()` method  
**Spec §1.5:** "Injection always selects the latest version."  
**What:** `Query()` returns ALL versions of every skill. If `fix-db-conn` has v1 and v2, both appear in results.  
**Fix:** Add a subquery or window function to filter to latest version per name+library.

---

## Low Priority / Nits

### 13. Insertion Sort Comment Says "Small" But Has No Guard
**File:** `store.go`, `sortScoredSkills()`  
**What:** Uses insertion sort with a comment "result sets are small." They are today. They won't always be.  
**Fix:** Use `slices.SortFunc()` from the standard library. It's one line and always correct.

### 14. `TestQueryLimit` Uses `string(rune('a'+i))`
**File:** `store_test.go`, `TestQueryLimit` and `TestListByDecayLimit`  
**What:** Generates IDs like `"id-a"`, `"id-b"`, etc. Works for 5 items but breaks at i=26 and produces invisible Unicode chars for smaller i values... actually no, it works fine for single chars. But it's a weird idiom — use `fmt.Sprintf("id-%d", i)`.

### 15. No Test for Concurrent Access
**What:** SQLite + WAL mode is claimed but never tested under concurrency. A simple test with 2 goroutines doing Put + Get would validate the WAL setup actually works.

### 16. No Test for `UpdateUsage` Correlation Computation
**File:** `store_test.go`, `TestUpdateUsage`  
**What:** Only tests that `injection_count` increments. Doesn't test the correlation subquery because there are no `injection_events` rows. This is the most complex SQL in the file and it's completely untested.

### 17. `Put()` Doesn't Validate Input
**File:** `store.go`, `Put()`  
**What:** No validation of skill ID (should be UUIDv7), name (should be kebab-case per spec), category (trusts the caller — DB CHECK catches it but gives a garbage error), or quality score ranges.  
**The existing `pkg/skill/skill.go`** validates kebab-case names. The storage layer should too, or at least the types should have a `Validate()` method.

### 18. Missing `MessageCount` Field on `SessionRecord` (If It Existed)
**File:** `schema.sql`  
**Related to issue #4.** Beyond the naming problem, the sessions schema is missing the `message_count` column entirely. And `tokens_used`, `agent_model`, `user_id` defaults don't match spec behavior.

---

## What's Actually Good (Grudgingly)

- **WAL mode + integrity check on startup** — correct, matches spec §5.3. Most people forget one or both.
- **Transaction usage in `Put()`** — patterns and embeddings in same tx with proper rollback. Correct.
- **`IF NOT EXISTS` in DDL** — safe for re-runs. Fine.
- **Cosine similarity implementation** — mathematically correct, handles edge cases (empty, zero-norm). I checked the math. It's right.
- **`nullString`/`nullTime` helpers** — clean handling of nullable columns. Good Go.
- **Test coverage breadth** — 18 tests covering Put, Get, GetLatestByName, Query (patterns, cosine, quality filter, decay filter, ordering, limit), UpdateUsage, UpdateDecay, ListByDecay (ordering, limit, library filter), edge cases (not found, unique constraint, no embedding, timestamps). That's respectable for a first pass.
- **Embedded schema DDL** — `//go:embed schema.sql` keeps schema next to code. Smart.

---

## Summary

| Category | Grade | Notes |
|----------|-------|-------|
| Spec compliance | C | Missing types, schema mismatch, ignored body param |
| Go idioms | B- | Good error wrapping, but string errors, no sentinels, duplicate scan |
| SQL correctness | B | Schema matches spec (mostly), parameterized queries, good constraints |
| Test quality | B- | Good breadth, missing depth (correlation, concurrency, error paths) |
| Bugs | C | N+1 query, dead correlation code, no version filtering |
| Security | A- | All queries parameterized, no string interpolation of user input |
| Consistency | B | Follows slog, fmt.Errorf wrapping, but config diverges from spec |

**Overall: C+**

The storage layer works for a demo. It does not work for production. Fix issues 1-4 before merging. Issues 5-8 should be addressed before Phase 1 builds on top of this. The rest can wait but shouldn't be forgotten.

— Jazz
