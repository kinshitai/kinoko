# Code Standards

## 2026-02-15
- Error handling: consistent `fmt.Errorf("context: %w", err)` pattern
- Logging: slog, structured key-value pairs
- Version: ldflags injection, never hardcoded
- Tests: table-driven, test both positive and negative cases
- Dependencies: run `go mod tidy`, mark direct deps as direct
- No external tool deps in tests (use Go's net.Listen, not nc)
- Signal handling: use select on both signal and context channels, no goroutine races
- No compiled binaries in repo, .gitignore catches them
