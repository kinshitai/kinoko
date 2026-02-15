# Phase 9 Review — Round 2

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Round 1 Grade:** B-  
**Round 2 Grade:** B+

---

## Critical Issues — Status

### 1. Human Review Verdict Mismatch — ✅ FIXED
Now queries `verdict = 'agree'` and computes precision as `agree / (agree + disagree_should_reject)`. Correct per §3.4. Good.

### 2. Stage Pass Rate Counting Errors — ✅ FIXED
Added `extraction_status NOT IN ('pending', 'error')` to all stage queries. Error sessions no longer inflate pass rates.

### 3. Event ID Collisions — ✅ FIXED
Switched to `uuid.NewV7()`. No more deterministic IDs. Correct.

---

## Significant Issues — Status

### 4. ABInjector Thread Safety — ✅ FIXED
`randFunc` is now guarded by `mu` in both `assignGroup()` and `SetRandFunc()`. Field is still exported-ish via setter, but the mutex makes it safe. Acceptable.

### 5. MinSampleSize Not Respected — ✅ FIXED
Collector now takes `minSampleSize`, `SufficientData` field added to `ABTestResult`, z-test only computed when both groups exceed threshold. This is exactly what I asked for.

### 6. Control Group Sessions Missing from Injection Rate — ⚠️ NOT FIXED
Still no "would-have-injected" count for control group. Not a correctness bug, but limits A/B analysis. Downgrading to minor — it's a reporting gap, not wrong data.

### 7. A/B Session Outcome Per-Event Assumption — ✅ ADDRESSED
Added a comment documenting the assumption that `session_outcome` is set identically across all events in a session. That's sufficient — the code is correct under the documented assumption.

---

## Remaining Minor Issues

### From R1 (unfixed)
- **#8 Decay bucket boundary** — Still `Max: 0.001` for "dead". A skill at 0.0005 is labeled "dead (0.00)" which is fair but the label is misleading. Low priority.
- **#9 No `--json` flag** — Still missing. You'll regret this when someone wants to pipe stats into Grafana.
- **#10 Silent config correction** — `NewABInjector` still silently resets bad `ControlRatio` to 0.1 with no log warning. One `slog.Warn` line.
- **#11 normalCDF precision comment** — Still undocumented. Add one line.
- **#12 Stats doesn't indicate insufficient A/B data** — The collector now skips z-test computation (good), but `stats.go` still prints `Z-score: 0.000` and `P-value: 0.0000` with `Result: not significant` when data is insufficient. Should print "(insufficient data, need N more sessions)" instead. Check `m.AB.SufficientData` before printing z-test lines.

### New Issue

- **#13 `NewCollector` doesn't receive `MinSampleSize` from config** — `stats.go` calls `metrics.NewCollector(store.DB())` without `WithMinSampleSize`. The default 100 is fine, but the config's `min_sample_size` field is ignored. Either pass it through or document that it's hardcoded.

---

## Summary

All 3 critical bugs fixed. 3 of 4 significant issues fixed, 1 downgraded. Code is correct and shippable. The remaining issues are polish — none produce wrong data.

| Severity | R1 | R2 |
|----------|----|----|
| Critical | 3 | 0 |
| Significant | 4 | 0 |
| Minor (remaining) | 5 | 6 |

**Grade: B+** — Solid fixes on everything that mattered. The minor stuff is real but won't hurt you in prod. Ship it, then clean up.
