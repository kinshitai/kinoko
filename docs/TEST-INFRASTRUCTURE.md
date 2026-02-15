# Test Infrastructure: Integration Testing Strategy

**Author:** Pavel (QA/SDET)  
**Date:** 2026-02-15  
**Status:** Recommendation

---

## Current State

The project uses in-memory SQLite, mock LLM/embedder, and real goroutines for integration tests. This catches pipeline logic bugs well.

### What's NOT Being Tested

| Gap | Risk |
|-----|------|
| Git operations against real Soft Serve | SSH key setup, repo creation, clone/push failures, `soft serve` lifecycle |
| Multi-process interactions | Soft Serve subprocess management, signal handling, port conflicts |
| HTTP discovery API (coming) | End-to-end client flow |
| Full write path: extract → git push → index | The core architectural promise ("git is truth") |
| Recovery: delete SQLite, rebuild from git | Data durability guarantee |

---

## Options Analysis

### Option A: Docker Compose

Soft Serve container + Mycelium test container, orchestrated via `docker-compose.yml`.

| | |
|---|---|
| **Pros** | Full isolation; reproducible; CI-friendly; tests real networking |
| **Cons** | Slow startup (~10-15s); requires Docker in CI; complex setup; hard to debug; developers need Docker locally |
| **Complexity** | High — Dockerfiles, compose config, CI integration, test coordination |
| **Catches** | Real networking, port binding, container lifecycle |

### Option B: Testcontainers-go

Spin up Soft Serve Docker container from Go test code using `testcontainers-go`.

| | |
|---|---|
| **Pros** | Programmatic; cleanup automatic; good Go integration; per-test isolation |
| **Cons** | Still requires Docker daemon; no official Soft Serve image (must build); ~10s per container start; extra dependency |
| **Complexity** | Medium — library dependency, custom container image, port management |
| **Catches** | Same as Docker Compose but with better test-level control |

### Option C: Test Binary Starts Soft Serve Subprocess

Like production: test helper starts `soft serve` on a random port, runs tests, kills it.

| | |
|---|---|
| **Pros** | Tests EXACTLY what production does; fast (~2s startup); no Docker needed; trivial to debug (it's just a process); skip gracefully if `soft` not installed |
| **Cons** | Requires `soft` binary installed; port conflicts possible (mitigated by random ports); cleanup on test crash needs care |
| **Complexity** | Low — 100 lines of test helper code |
| **Catches** | Real SSH operations, key generation, repo CRUD, clone/push, subprocess lifecycle — the actual production code path |

### Option D: Keep Current + Targeted Integration Tests

Don't change anything. Add a few tests that shell out to git commands against a mock.

| | |
|---|---|
| **Pros** | Zero disruption |
| **Cons** | Doesn't test the real thing; git mock ≠ git server; false confidence |
| **Complexity** | Low |
| **Catches** | Almost nothing new |

---

## Recommendation: Option C (Subprocess)

**Use Option C.** Here's why:

1. **Tests what matters.** The `Server.Start()` code literally starts a subprocess. Testing with a subprocess tests the real code path. Docker adds a layer that production doesn't have.

2. **Fast.** Soft Serve starts in ~2 seconds. Docker adds 10-15 seconds of overhead per test suite. In CI, this compounds.

3. **Simple.** A `TestHelper` struct with `Start(t)` and cleanup via `t.Cleanup()`. No Dockerfiles, no compose, no image builds.

4. **Graceful degradation.** If `soft` isn't installed (CI without it, developer without it), tests skip with `t.Skip()`. Existing tests unaffected.

5. **Production parity.** Docker Compose tests a different deployment model than what Mycelium actually uses. We'd be testing Docker, not Mycelium.

### When to Add Docker (Future)

Move to Docker Compose **only** when:
- Multiple services need networking (e.g., separate discovery API server + Soft Serve + client)
- We need CI environments where installing `soft` is impractical
- We add multi-node testing

That's Phase 3+. Not now.

### Implementation Plan

```
tests/integration/
├── helpers_test.go          # existing
├── gitserver_test.go        # NEW: git server test helpers + smoke tests
├── integration_test.go      # existing
├── worker_integration_test.go
└── pavel_integration_test.go
```

**Test helper API:**

```go
type GitTestServer struct {
    Port       int
    DataDir    string
    AdminKey   string
    CloneURL   func(repo string) string
}

func StartGitTestServer(t *testing.T) *GitTestServer
// Starts soft serve on random port, returns connection info.
// t.Cleanup() handles shutdown + temp dir removal.
// Calls t.Skip() if soft binary not found.
```

**Test categories:**

| Tag | Requires | Example |
|-----|----------|---------|
| `-short` | Nothing | All existing tests pass |
| (default) | `soft` binary | Git server smoke tests |
| `-run TestGit` | `soft` + `git` | Full git integration |

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Port conflict | Use port 0 / random high port; retry once on EADDRINUSE |
| Zombie process on crash | `t.Cleanup()` + `defer` kill; test timeout |
| Flaky SSH connection | Retry loop in `waitForReady()` with 15s timeout |
| Missing `soft` binary | `t.Skip("soft binary not found, skipping git integration tests")` |
| Temp dir leak | `t.TempDir()` — Go handles cleanup |

---

## Conclusion

Docker Compose is overkill for Phase 1. The system is a single binary that manages a subprocess. Test it as a single binary managing a subprocess. Add Docker when the architecture demands it.

*"Test what you ship. Ship what you test. Docker is for when you ship Docker."* — Pavel
