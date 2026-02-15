# Test Infrastructure

## 2026-02-15
- E2e tests: tests/e2e/ with build tag `//go:build integration`
- Test helpers: setup_test.go — isolated temp environments, dynamic port allocation, binary building, cleanup
- Fixtures: tests/fixtures/ — valid-minimal.md, valid-full.md, valid-code-blocks.md, invalid-no-frontmatter.md, invalid-bad-confidence.md, edge-unicode.md
- Integration tests requiring `soft` binary should be skippable in CI
- Port checking: use Go's net.Listen(), NOT external nc command
- Always clean up temp dirs in tests (defer cleanup)
