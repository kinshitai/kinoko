# Phase 9 Review — A/B Testing & Metrics

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `internal/injection/ab.go`, `internal/metrics/collector.go`, `cmd/mycelium/stats.go`  
**Grade: B-**

Not bad structurally, but there are real bugs hiding in the metric queries and a spec mismatch that'll silently produce wrong data. The A/B injector is clean but has a thread-safety gap and a subtle randomization concern.

---

## Critical Issues

### 1. Human Review Verdict Mismatch (collector.go) — BUG
The collector queries `WHERE verdict = 'useful'`, but the spec §3.4 defines verdicts as `'agree' | 'disagree_should_extract' | 'disagree_should_reject'`. There is no `'useful'` verdict in the schema or spec. This query will always return 0, making extraction precision permanently 0%.

**Fix:** Query `WHERE verdict = 'agree'` to match the spec, or define what "useful" actually means and update both the spec and schema.

### 2. Stage Pass Rate Counts Error Sessions as Passes (collector.go) — BUG
The Stage 1 passed query uses `rejected_at_stage > 1 OR rejected_at_stage = 0`. Sessions with `extraction_status = 'error'` have `rejected_at_stage = 0` (they weren't rejected, they errored). These get counted as Stage 1 passes, Stage 2 passes, and Stage 3 passes — inflating all pass rates.

**Fix:** Add `AND extraction_status != 'error'` to the pass-rate queries, or better yet, explicitly enumerate:
```sql
WHERE extraction_status IN ('stage2','stage3','extracted') -- for stage1 passed
```

### 3. A/B Event ID Collisions (ab.go) — BUG
The event ID is `fmt.Sprintf("%s-%s-%d-ab", req.SessionID, sk.SkillID, i)`. If the same session somehow gets injected twice (retry, race), you get a PRIMARY KEY conflict and silent data loss. Use a UUIDv7 like the rest of the system.

---

## Significant Issues

### 4. ABInjector Is Not Thread-Safe
`ABInjector` stores no mutable state itself (good), but `randFunc` field is set publicly in tests via `ab.randFunc = ...`. In production, `rand.Float64` from `math/rand/v2` is safe for concurrent use, so this works *in practice*. But the exported `randFunc` field is a footgun — anyone could assign a non-thread-safe closure. Make it unexported and use a constructor option or test-only build tag.

### 5. A/B Doesn't Respect MinSampleSize
`ABConfig.MinSampleSize` is validated and stored but **never read**. The spec says "minimum sessions per group before computing results" — the collector should skip z-test computation when either group has fewer sessions than this threshold. Currently `collector.go` always computes the z-test regardless of sample size, which can produce misleading p-values from tiny samples.

**Fix:** Either pass `MinSampleSize` to the collector, or at minimum add a `SufficientData bool` field to `ABTestResult` and set it based on whether both groups exceed some minimum (the spec says 100).

### 6. Control Group Sessions Missing from Injection Rate
`SessionsWithInjection` counts `WHERE delivered = 1`. This is correct for the injection *rate* metric (§3.2), but the stats output labels it generically. Control group sessions that *would have* received injection aren't counted anywhere separately. Consider adding a "would-have-injected" count for the A/B report so you can verify the control group isn't accidentally biased toward no-match sessions.

### 7. A/B Session Success Counting Is Per-Event, Not Per-Session
`collectAB()` counts `COUNT(DISTINCT session_id) ... WHERE session_outcome = 'success'`. But `session_outcome` is set per injection_event row, not per session. If a session has 3 injected skills and only 2 are marked 'success' (the third NULL), the session still counts as successful. This is probably fine but undocumented — the assumption is that `session_outcome` is set identically across all events in a session. If it's ever set per-skill, the z-test results become wrong.

---

## Minor Issues

### 8. Decay Bucket Boundary Overlap
`{Min: 0.0, Max: 0.001}` and `{Min: 0.001, Max: 0.25}` — a skill with `decay_score = 0.001` lands in the "low" bucket, not "dead". This is fine mathematically (`>=` and `<`) but the label "dead (0.00)" is misleading since it catches scores like 0.0005. Consider `{Min: 0, Max: 0.01}` for "dead" or just `= 0.0`.

### 9. No `--json` Flag on Stats
Every other CLI tool in the modern world supports `--json` output. This only does human-readable. When someone wants to pipe metrics into monitoring, they'll be parsing printf output with regex. Add a `--json` flag or at minimum `--format=json|text`.

### 10. AB Config Validation Silently Corrects Bad Values
`NewABInjector` silently resets `ControlRatio` to 0.1 if it's ≤0 or ≥1. Log a warning. A user who sets `control_ratio: 1.5` by mistake will think 100% is control when actually it's 10%.

### 11. normalCDF Approximation Precision
The Abramowitz & Stegun formula (26.2.17) has max error ~1.5×10⁻⁷, which is fine for this use case. But you should document the precision bound in a comment so nobody replaces it with something worse thinking it's "just an approximation."

### 12. Stats Command Doesn't Check MinSampleSize Before Showing A/B
The stats command prints A/B results even with 2 sessions. Add a note like "(insufficient data, need N more sessions)" when below threshold.

---

## What's Good (brief, because that's not my job)
- Z-test math is correct, verified against known values
- Test coverage on ab.go is solid — deterministic randomization testing is the right approach
- Schema has `ab_group` and `delivered` columns already in place, no migration needed
- Control group preserves classification — good for debugging
- Event logging happens for both groups — critical for valid A/B analysis

---

## Summary

| Severity | Count |
|----------|-------|
| Critical (wrong data) | 3 |
| Significant (logic gap) | 4 |
| Minor (polish) | 5 |

Fix #1 and #2 before shipping — they produce silently wrong metrics. Fix #3 before any real traffic. The rest can be follow-ups.
