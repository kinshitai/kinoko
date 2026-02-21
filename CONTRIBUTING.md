# Contributing to Kinoko

## Project Structure

```
cmd/kinoko/            CLI entry point — all cobra commands
internal/
  run/                 Client daemon (kinoko run)
    apiclient/         HTTP client for the server API
    client/            Local config and SSH key management
    debug/             Pipeline debug tracing
    extraction/        3-stage skill extraction pipeline
    injection/         Skill injection (classify → discover → prompt)
    llm/               LLM provider abstraction
    llmutil/           JSON extraction helpers
    metrics/           Client pipeline metrics
    queue/             Local SQLite job queue
    sanitize/          Credential scanning
    worker/            Worker pool and scheduler
  serve/               Infrastructure server (kinoko serve)
    api/               HTTP API (/api/v1/discover, /health, /ingest, etc.)
    embedding/         Embedding providers (OpenAI, ONNX)
    gitserver/         Soft Serve git server, hooks, committer
    storage/           SQLite skill store (schema, indexer, querier)
  shared/              Shared by both run and serve
    circuitbreaker/    Circuit breaker for external calls
    config/            YAML config loading and defaults
    decay/             Skill decay (half-life degradation)
pkg/
  model/               Public domain types and interfaces
  skill/               SKILL.md parser and validator
tests/
  architecture/        Import boundary tests
  e2e/                 End-to-end tests
  integration/         Integration tests
  fixtures/            Test data
docs/                  Architecture and reference docs
site/                  Public documentation site (Astro)
rfcs/                  Design RFCs
scripts/               Git hooks and tooling
```

For a detailed walkthrough, see [docs/architecture.md](docs/architecture.md).

## Building and Testing

```bash
make build             # Build the kinoko binary
make check             # Full CI: build + vet + lint + unit tests + integration + e2e
go test ./...          # Unit tests only
go test -tags integration ./tests/integration/   # Integration tests
go test -tags integration ./tests/e2e/           # End-to-end tests
```

Requirements: Go 1.22+, `golangci-lint`.

## Git Flow

**Never push directly to `main`.** All changes go through pull requests.

### Workflow

```
1. Create a branch    →  git checkout -b <type>/<short-name>
2. Do the work        →  commits on branch
3. Push & open PR     →  git push -u origin <branch>
4. CI must pass       →  Lint + Test + Build (all three green)
5. Jazz reviews code  →  must approve before merge
6. Squash merge       →  into main, delete branch
```

### Branch Naming

```
feat/<name>       — new feature or capability
fix/<name>        — bug fix
docs/<name>       — documentation only
refactor/<name>   — code restructuring, no behavior change
test/<name>       — test additions or fixes
chore/<name>      — CI, tooling, cleanup
```

### PR Requirements

Before merging, every PR must have:

- [ ] **CI green** — Lint, Test, Build all pass
- [ ] **Jazz review** — code review approved (grade B+ or higher)
- [ ] **No architectural drift** — changes align with MANIFESTO.md and RFCs
- [ ] **Docs updated** — if behavior changed, docs reflect it (Charis owns this)

### Commit Messages

```
<type>: <short description>

<optional body — what and why, not how>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Keep it English only. No emoji in commit messages.

### Who Does What

| Role | Person | Responsibility |
|---|---|---|
| Architecture & specs | Hal 🔧 | Write specs, delegate, guard architecture |
| Implementation | Otso 🇫🇮 | Build from specs, branch + PR |
| Code review | Jazz 👴 | Review every PR, grade B+ minimum to merge |
| QA | Pavel 🇷🇺 | Test coverage, integration tests, verify on branch |
| Docs | Charis 🇨🇧 | Update docs when behavior changes, audit regularly |
| R&D | Luka 🇩🇰 | Research briefs, specs for new features |

### Review Process

1. Otso (or whoever implements) opens PR with description of what changed and why
2. Pavel runs tests on the branch, reports coverage delta
3. Jazz reviews code — checks correctness, security, alignment with spec
4. Jazz approves or requests changes with specific feedback
5. Once approved + CI green → squash merge to main

### What Hal Does NOT Do

- Write code directly (delegate to Otso)
- Push to main (nobody does)
- Skip review (even "obvious" fixes go through Jazz)
