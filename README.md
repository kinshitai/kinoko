# Mycelium

**Every problem solved once is solved for everyone.**

Mycelium is knowledge-sharing infrastructure for AI agents. When an agent solves a problem, Mycelium extracts the knowledge, stores it as a version-controlled skill, and injects it into future sessions — automatically.

```
Agent solves problem → Knowledge extracted → Stored in git → Injected into future sessions
```

## Project Status

**Active development.** Core infrastructure and extraction pipeline are implemented. 11 packages, 373 tests, ~17K lines of Go.

**What works today:**
- `mycelium init` — set up a workspace
- `mycelium serve` — run a local git server with automatic extraction and injection hooks
- `mycelium extract` — manually run the 3-stage extraction pipeline on a session log
- `mycelium decay` — run a decay cycle to demote stale skills
- `mycelium stats` — view pipeline metrics, A/B test results, and decay distribution
- Full extraction pipeline: metadata filtering → embedding novelty + rubric scoring → LLM critic
- Skill storage in SQLite with embedding-based similarity search
- Injection pipeline with prompt classification, pattern matching, and degraded mode
- A/B testing framework for measuring injection effectiveness
- Half-life decay with category-aware rates and rescue mechanics
- Pipeline metrics with stage pass rates, extraction precision, and statistical significance testing

## Quick Start

**Requirements:** Go 1.24+, Git, SSH, OpenAI API key

```bash
# Install
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium
go install ./cmd/mycelium

# Set up workspace
mycelium init

# Set API key (needed for extraction and injection)
export OPENAI_API_KEY=sk-...

# Start the server (extraction + injection hooks active)
mycelium serve
```

Your Mycelium server is now running at `ssh://localhost:23231`.

### Run extraction manually

```bash
# Extract knowledge from a session log
mycelium extract ./session.log

# View pipeline metrics
mycelium stats

# Run decay cycle
mycelium decay --library local
mycelium decay --dry-run  # preview changes
```

### Create a test skill

```bash
mkdir -p ~/.mycelium/skills/hello-world
cat > ~/.mycelium/skills/hello-world/SKILL.md << 'EOF'
---
name: hello-world
version: 1
tags: [test]
author: you@example.com
confidence: 0.8
created: 2026-02-15
---

# Hello World

## When to Use
Testing that your Mycelium setup works.

## Solution
If you can read this, it works.
EOF

cd ~/.mycelium/skills
git add hello-world/
git commit -m "Add hello-world skill"
```

## How It Works

### Extraction Pipeline

Sessions pass through a 3-stage filter. Each stage is more expensive than the last; most sessions are rejected early.

1. **Stage 1 — Metadata pre-filters:** Duration, tool calls, error rate, successful execution. No I/O.
2. **Stage 2 — Embedding novelty + rubric scoring:** Computes embedding distance from existing skills, then scores 7 quality dimensions via LLM.
3. **Stage 3 — LLM critic:** Independent extract/reject verdict with retry, circuit breaker, and contradiction detection.

Extracted skills are persisted as `SKILL.md` files with YAML front matter and stored in SQLite with embeddings for similarity search.

### Injection Pipeline

When an agent starts a session, Mycelium classifies the prompt, queries the skill store, and injects the top-ranked skills as context. Ranking combines pattern overlap, embedding similarity, and historical success rate. Falls back to pattern-only matching when embeddings are unavailable.

### Decay System

Skills decay based on category-specific half-lives (foundational: 365d, tactical: 90d, contextual: 180d). Skills used recently with positive outcomes are rescued. Skills below the deprecation threshold are effectively retired.

### A/B Testing

Optional framework that withholds skills from a control group to measure injection effectiveness. Results are analyzed with a two-proportion z-test.

## Documentation

- [Installation](docs/getting-started/installation.md) — platform-specific setup
- [Quickstart](docs/getting-started/quickstart.md) — detailed walkthrough
- [Architecture](docs/architecture.md) — system design and data flow
- [CLI Reference](docs/reference/cli-reference.md) — commands and flags
- [Configuration](docs/reference/config-reference.md) — config.yaml options
- [Skill Format](docs/reference/skill-format.md) — SKILL.md specification
- [Glossary](docs/glossary.md) — terminology
- [Troubleshooting](docs/troubleshooting.md) — common issues

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TBD
