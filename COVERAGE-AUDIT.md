# Coverage Audit — Pre-Refactor Areas (2026-02-15)

## Summary: All 4 areas have adequate test coverage. No new tests needed.

### Area 1: Circuit Breakers (R3)
- **embedding/circuit_breaker_test.go**: ✅ Full cycle (closed→open→half-open→closed) + escalation on half-open fail
- **extraction/stage3_circuit_test.go**: ✅ Same transitions tested with clock-based approach
- **Gap found:** None. Both implementations cover all state transitions.

### Area 2: JSON Parsers (R4)
- **extraction/json_parse_test.go**: ✅ `parseCriticResponse` and `parseRubricResponse` tested with all 4 strategies (raw, ```json, ```, first-{-to-last) plus error cases
- **injection/parse_test.go**: ✅ `parseClassificationResponse` tested with raw + first-{-to-last (note: this parser intentionally doesn't support fence strategies — tech debt C.2)
- **Gap found:** None.

### Area 3: Store Methods (R6)
19 public methods on SQLiteStore. Coverage found in 3 test files:
- `store_test.go`: Put, Get, GetLatestByName, Query, UpdateUsage, UpdateDecay, ListByDecay, CosineSimilarity, sentinel errors, timestamps, uniqueness, body-to-disk, embedding model
- `store_extended_test.go`: Unbounded IN clause bug, file-after-commit, InsertSession (duplicate + get)
- `store_methods_test.go`: WriteInjectionEvent (+ control group), UpdateInjectionOutcome (+ multi-event), InsertReviewSample, UpdateSessionResult (+ rejection), GetSession not-found, UpdateUsage success correlation, helper round-trips
- **Gap found:** None. All 19 methods covered. Private helpers (loadPatterns, loadEmbeddingsMulti, etc.) exercised transitively through Query.

### Area 4: parseSessionFromLog (R9)
- **cmd/kinoko/extract_test.go**: ✅ 15+ test cases covering empty input, multi-turn sessions, metadata extraction, title/problem detection, unique IDs, large files
- **cmd/kinoko/commands_test.go**: Additional usage
- **Gap found:** None.

## Conclusion
All areas targeted for refactoring (R3, R4, R6, R9) have solid test coverage. Safe to proceed with extraction/unification/splitting.
