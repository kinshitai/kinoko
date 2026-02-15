# Review Patterns

## 2026-02-15
- This codebase started broken (config mismatch between init and serve). Parallel devs without shared specs = integration bugs.
- Single dev + exact specs from Hal = much better results.
- Soft Serve can't be embedded as library — subprocess is correct approach.
- The team tends to build beautiful abstractions around TODOs instead of implementing the hard part. Push back on this.
- Pavel (QA) finds real bugs but also produces false positives. ~40% of his unit test requests are worth doing. He assumes underscores are valid in kebab-case names — they're not.
- Code quality improved dramatically across rounds: F → C+ → D+ → A-.
