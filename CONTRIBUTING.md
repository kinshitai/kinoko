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
