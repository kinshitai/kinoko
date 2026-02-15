# Phase 5 Review — Round 2

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `pipeline.go`, `pipeline_test.go`, `types.go`  
**Previous Grade:** C+  
**Grade: B**

Most P1s addressed. Skill IDs are UUIDv7. Extractor interface exists with compile-time check. Skill name derives from classification, not raw content. SKILL.md has proper front matter. Timestamps set. Good.

But the stratified sampling is still wrong, and the SKILL.md body is an empty template — you extract structure with no content.

---

## P1 Disposition from R1

| ID | Issue | Status | Notes |
|----|-------|--------|-------|
| P5-01 | Skill ID not UUIDv7 | ✅ Fixed | `uuid.Must(uuid.NewV7())` — correct |
| P5-02 | Sampling not stratified | ❌ Still broken | See below |
| P5-03 | SKILL.md format wrong | ⚠️ Partially fixed | Front matter correct, body is empty template |
| P5-04 | Skill name heuristic | ✅ Fixed | `skillNameFromClassification` uses patterns/category |
| P5-05 | No session state persistence | — | P2, not required this round |
| P5-06 | CreatedAt/UpdatedAt zero | ✅ Fixed | Set via `time.Now()` |
| P5-07 | No Extractor interface | ✅ Fixed | Declared in types.go, compile-time `var _` check |

---

## Remaining Issues

### P5-02 (STILL OPEN): Stratified sampling is cosmetic

The code doubles the per-pool rate and logs which pool a sample came from. But **both pools use the identical sampling rate and identical code path**. The `pool` variable is only used in the log line.

Stratified sampling means: if 90% of sessions are rejected and 10% extracted, you need a *higher* rate for extracted sessions and a *lower* rate for rejected ones to get 50/50 representation. The current code samples both at `sampleRate * 2`, which gives you ~90% rejected / ~10% extracted samples — the exact base-rate problem the spec says to avoid.

Fix options:
1. Track per-pool counts and use reservoir sampling per pool
2. Use different rates: e.g. `extractedRate = sampleRate * (1 / extractedFraction)`, adjustable
3. At minimum, sample ALL extracted sessions (they're rare) and downsample rejected ones

This was P1 in R1 and it's still P1. The comment claiming stratification doesn't make it stratified.

### P5-03b (NEW): SKILL.md body contains zero extracted knowledge

```go
fmt.Fprintf(&b, "## When to Use\n\n")
fmt.Fprintf(&b, "<!-- Trigger conditions for this skill -->\n\n")
fmt.Fprintf(&b, "## Solution\n\n")
fmt.Fprintf(&b, "<!-- Core knowledge extracted from the session -->\n\n")
```

The front matter is correct now. But the body is HTML comment placeholders. `buildSkillMD` receives `*SkillRecord` which has no content field. The session content (`[]byte`) is available in `Extract()` but never passed to `buildSkillMD`. The entire point of extraction is producing reusable knowledge — this produces an empty form.

Either:
- Stage 3's critic response should include extracted knowledge sections (the LLM already reads the content)
- Or `buildSkillMD` needs the session content + stage results to populate the body

This is a P1. An extraction pipeline that produces empty skill files isn't extracting anything.

---

## New Issues

### P5-13 (P2): `cryptoRandIntn` silently drops error

```go
func cryptoRandIntn(n int) int {
    v, _ := rand.Int(rand.Reader, big.NewInt(int64(n)))
    return int(v.Int64())
}
```

If `rand.Reader` fails (rare but possible — exhausted entropy, broken /dev/urandom), `v` is nil → **panic on `v.Int64()`**. Either handle the error or document that this panics on crypto failure.

### P5-14 (P3): `strings.Title` is deprecated

`buildSkillMD` uses `strings.Title` with a `//nolint:staticcheck` suppression. Use `cases.Title(language.English)` from `golang.org/x/text` or a simple manual titlecase. The nolint just hides the problem.

### P5-15 (P2): No nil-guard on required pipeline deps (carried from P5-10)

`NewPipeline` still doesn't validate that `Stage1`, `Stage2`, `Stage3`, `Writer`, or `Log` are non-nil. First call to `Extract` panics. Either validate in constructor and return `(*Pipeline, error)`, or document the panic contract.

---

## Test Coverage

Tests improved significantly:
- ✅ `TestBuildSkillMD` validates front matter fields and section structure
- ✅ `TestSkillNameFromClassification` covers pattern parsing, fallbacks, empty inputs
- ✅ `TestPipelineSkillFields` checks UUIDv7 format, timestamps, ExtractedBy, Version
- ✅ Sampling boundary tests updated for stratified 2x math
- ✅ Sampling on extracted path tested

Still missing:
- ❌ No test proving stratification actually stratifies (test with mixed extracted/rejected and verify pool distribution)
- ❌ No test for `buildSkillMD` with empty patterns or zero quality scores
- ❌ No test for `kebab` edge cases (all-caps strings like "HTTP", mixed unicode)
- ❌ No test for context cancellation

---

## Summary

Real progress. The easy P1s (UUIDv7, interface, naming, timestamps) are all fixed correctly. The `kebab` function is well-implemented. Test coverage expanded meaningfully. Front matter format matches spec.

Two things keep this from an A: the sampling is labeled stratified but isn't, and the pipeline produces skill files with no actual content. One is a math problem, the other is an architecture gap — `buildSkillMD` needs access to extracted knowledge, which means either Stage 3 output needs to include it or the pipeline needs a Stage 4.

Fix P5-02 and P5-03b before merging.
