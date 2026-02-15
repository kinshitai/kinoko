# Phase A — Queue Layer Review (Round 2)

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Previous Grade:** B-  
**Updated Grade:** B+

---

## Fixed Issues ✓

### #1 Claim atomicity (was CRITICAL)
Added documentation comment explaining reliance on SQLite single-writer + `busy_timeout`. Added `rowid ASC` tiebreaker (also fixes #10 and #13 partially). Still not wrapped in `BEGIN IMMEDIATE` but the comment makes the contract explicit. Acceptable.

### #2 TOCTOU race on backpressure (was HIGH)
**Properly fixed.** Depth check and INSERT now live in a single transaction. Log file written *before* the transaction and cleaned up on any failure path. This is correct.

### #4 Fail() non-atomic read-modify-write (was MEDIUM)
**Properly fixed.** Single `UPDATE` computes backoff inline with `retry_count + 1` and `1 << retry_count`. Clean. The `MIN(... , ?)` cap on max backoff is correct.

### #6 Migration swallows all errors (was MEDIUM)
**Properly fixed.** Now checks `strings.Contains(err.Error(), "duplicate column")` before ignoring. Non-duplicate errors propagate and close the DB. Good.

### #10 FIFO tiebreaker (was LOW)
Fixed via `ORDER BY created_at ASC, rowid ASC` in the Claim query.

### #11 String comparison for ErrNoRows (was LOW)
Fixed. Uses `errors.Is(err, sql.ErrNoRows)` now.

---

## Still Open

### #5 No log file cleanup on Complete/FailPermanent (MEDIUM)
Still no `os.Remove(logContentPath)` in `Complete()` or `FailPermanent()`. The queue dir will accumulate dead log files indefinitely. This isn't a correctness bug but it's a resource leak. Add cleanup or document the omission.

### #7 Complete() clears claimed_by/claimed_at (MEDIUM)
Still erasing audit trail on completion. Same issue as R1 — you lose which worker processed the session. Not blocking, but don't pretend this is fine for debugging.

### #8 StatusQueued vs 'pending' default (LOW)
Unchanged, still confusing. Tracking.

### #9 Depth undercounts error sessions (LOW)
Unchanged. Tracking.

---

## New Observations

### Enqueue transaction doesn't use IMMEDIATE mode (LOW-MEDIUM)
The `BeginTx` in `Enqueue` doesn't pass `&sql.TxOptions{}` to force an immediate/write lock. SQLite upgrades a deferred transaction to a write lock on the first write statement, but between the `SELECT COUNT(*)` (read) and the `INSERT` (write), another writer could sneak in. In practice `busy_timeout` saves you again, but since you went to the trouble of putting this in a transaction specifically to prevent TOCTOU, you should use `_txlock=immediate` on the DSN or `BEGIN IMMEDIATE`. 

I see the test DSN has `_txlock=immediate` — good. Confirm the production DSN does too, or this fix is test-only theater.

### Enqueue cleanup path is thorough (GOOD)
Every error branch after writing the log file calls `os.Remove(logPath)`. Checked all five paths — correct. Nice.

### Test coverage unchanged
Still missing the tests I called out in R1 (duplicate session ID, Fail on nonexistent session, Complete rejection paths). Not blocking but don't forget them.

---

## Summary

The three most important fixes (#2, #4, #6) are done correctly. The claim atomicity documentation (#1) is an acceptable compromise. The FIFO and ErrNoRows fixes (#10, #11) are clean. Two medium issues (#5, #7) remain — neither is blocking but both should be tracked. The `_txlock=immediate` question is new and worth confirming before deploy.

Bumped from B- to B+. Fix #5 and confirm the DSN issue and this is an A-.
