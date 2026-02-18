# Contributing to Kinoko

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
| Docs | Charis 🇨🇦 | Update docs when behavior changes, audit regularly |
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

## Project Structure

```
cmd/kinoko/          CLI entry point — one file per subcommand
internal/
  api/               HTTP API server (health, discover, embed, ingest, decay)
  circuitbreaker/    Thread-safe circuit breaker with exponential backoff
  client/            End-user client library (discover, pull, sync)
  config/            YAML config loading, validation, defaults
  debug/             Pipeline debug tracing
  decay/             Half-life decay runner (foundational/tactical/contextual)
  embedding/         Embedding: HTTP client (OpenAI-compatible) + ONNX engine (build tag)
  extraction/        3-stage extraction pipeline (filter → score → critic)
  gitserver/         Soft Serve subprocess management, hooks, committer
  injection/         Skill injection: classifier, ranker, A/B testing, prompt builder
  llm/               LLM clients (OpenAI, Anthropic) + factory
  llmutil/           JSON extraction from LLM responses
  metrics/           Pipeline health metrics collector
  model/             Domain types and interfaces (no business logic)
  queue/             Local extraction work queue (SQLite-backed)
  sanitize/          Credential scanner (regex + context-aware)
  serverclient/      HTTP client for run→serve communication
  storage/           SQLite persistence (skills, sessions, events, embeddings)
  worker/            Worker pool + scheduler
pkg/skill/           Public SKILL.md parser and validator
site/                Documentation website (Astro + Starlight)
tests/
  e2e/               End-to-end tests
  integration/       Integration tests
  fixtures/          Test SKILL.md fixtures
```

## Running Locally

```bash
# Build
go build ./cmd/kinoko

# Initialize workspace
./kinoko init

# Start the server (in one terminal)
./kinoko serve

# Start the agent daemon (in another terminal)
export OPENAI_API_KEY=sk-...
./kinoko run

# One-off commands
./kinoko extract session.log
./kinoko match "fix database timeout"
./kinoko stats
./kinoko queue stats
```

## Testing

```bash
# Unit tests
go test ./...

# With race detector
go test -race ./...

# Integration tests (require more setup)
go test ./tests/integration/...

# E2E tests
go test ./tests/e2e/...
```

## Build Tags

- Default: no ONNX, embedding via HTTP API
- `embedding`: enables local ONNX embedding engine (requires native libraries)

```bash
# Build with ONNX support
go build -tags embedding ./cmd/kinoko
```

## Key Design Decisions

- **Git as source of truth**: Skills live in Soft Serve repos as `SKILL.md` files. SQLite is a derived cache rebuilt on every push via post-receive hooks.
- **Client/server split**: `kinoko serve` owns storage and git; `kinoko run` owns extraction workers and uses HTTP to communicate with serve.
- **3-stage extraction**: Cheap metadata filter → embedding novelty + rubric → expensive LLM critic. Each stage reduces the volume for the next.
- **Circuit breaker on LLM calls**: Stage 3 uses a circuit breaker to prevent cascading failures from LLM provider outages.
- **Credential scanning**: Pre-receive hooks scan pushed content; extraction pipeline also scans before committing.
