# Mycelium

**Every problem solved once is solved for everyone.**

Mycelium is collective knowledge infrastructure for AI agents. When agents solve problems, Mycelium automatically extracts the knowledge, stores it as version-controlled skills, and injects relevant knowledge into future sessions.

Knowledge sharing as a byproduct of work, not a separate activity.

```
    Agent Session         →    Extract Knowledge    →    Future Sessions
   ┌─────────────────┐         ┌──────────────────┐      ┌─────────────────┐
   │ Problem: Debug  │         │ SKILL.md created │      │ Similar problem │
   │ Go race conds   │   ──→   │ & stored in git  │  ──→ │ → relevant skill│
   │ Solution found  │         │ repo             │      │ auto-injected   │
   └─────────────────┘         └──────────────────┘      └─────────────────┘
```

## Why This Matters

Right now, every debugging breakthrough, every architectural insight, every hard-won workaround dies when the session ends. The next person hitting the same problem starts from zero. **This is the largest knowledge waste in computing.**

Mycelium fixes this by making knowledge sharing automatic:
- **Zero friction:** No writing, no tagging, no publishing
- **Version controlled:** Skills stored as git repos with full history  
- **Layered libraries:** Personal → team → community knowledge
- **Quality gates:** Credential scanning, format validation, confidence scoring

## Quick Start

```bash
# 1. Install (requires Go 1.21+)
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest

# 2. Initialize workspace
mycelium init

# 3. Start server  
mycelium serve

# 4. Configure your agent to use Mycelium
# (Integration docs coming soon)
```

Your Mycelium server is now running at `ssh://localhost:23231`. Agents can clone, push, and pull skill repositories over SSH.

## What You Get

- **Local git server** powered by [Soft Serve](https://github.com/charmbracelet/soft-serve)
- **Automatic skill extraction** from agent sessions (when integrated)
- **Quality validation** with pre-commit hooks
- **Layered knowledge** from multiple sources (local, team, community)
- **Full version control** — every skill change is tracked

## Project Status

**Early Development** — Core infrastructure works, agent integrations coming soon.

**What works today:**
- ✅ Server setup (`mycelium serve`)
- ✅ Workspace initialization (`mycelium init`)  
- ✅ SKILL.md format and validation
- ✅ Git repository management
- ✅ Configuration system

**Coming in Phase 2:**
- 🔄 Agent integration hooks (extraction & injection)
- 🔄 Semantic search and discovery
- 🔄 Trust scoring and collaboration features

## Documentation

- **[Quickstart Guide](docs/getting-started/quickstart.md)** — 5-minute working setup
- **[Installation](docs/getting-started/installation.md)** — Platform-specific setup
- **[Configuration Reference](docs/reference/config-reference.md)** — Complete config options
- **[CLI Reference](docs/reference/cli-reference.md)** — All commands and flags
- **[Skill Format](docs/reference/skill-format.md)** — SKILL.md specification
- **[Troubleshooting](docs/troubleshooting.md)** — Common issues and solutions

## Vision

Read the **[Manifesto](MANIFESTO.md)** to understand why this exists, or dive into **[RFC-002](rfcs/002-architecture-and-roadmap.md)** for the technical architecture.

**The goal:** A junior developer in São Paulo debugs a problem they've never seen. Their agent already knows three approaches from other developers on other continents. They get a better answer without searching, asking, or even knowing Mycelium exists.

Knowledge flows. Problems get solved once for everyone.

## Contributing

Mycelium is open source and built in the open. Check the [RFCs](rfcs/) for architectural decisions and the roadmap.

**Dependencies:** Go 1.21+, git, SSH

## License

TBD