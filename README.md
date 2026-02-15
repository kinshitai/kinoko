# Mycelium

**Every problem solved once is solved for everyone.**

Mycelium is knowledge-sharing infrastructure for AI agents. When an agent solves a problem, Mycelium extracts the knowledge, stores it as a version-controlled skill, and injects it into future sessions — automatically.

```
Agent solves problem → Knowledge extracted → Stored in git → Injected into future sessions
```

## Project Status

**Early development.** Core infrastructure works. Agent integrations are coming. Will break.

**What works today:**
- `mycelium init` — set up a workspace
- `mycelium serve` — run a local git server (SSH, powered by [Soft Serve](https://github.com/charmbracelet/soft-serve))
- SKILL.md format and validation
- Configuration with layered skill libraries

**Coming next:**
- Agent integration hooks (extraction & injection)
- Semantic search and skill discovery

## Quick Start

**Requirements:** Go 1.24+, Git, SSH

```bash
# Install
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium
go install ./cmd/mycelium

# Set up workspace
mycelium init

# Start the server
mycelium serve
```

Your Mycelium server is now running at `ssh://localhost:23231`.

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

Skills are structured knowledge stored as Markdown files with YAML front matter. They live in git repositories organized into layered libraries:

- **Local** (highest priority) — your personal skills at `~/.mycelium/skills/`
- **Team** — shared skills from your organization
- **Community** (lowest priority) — public skill libraries

When multiple libraries have a skill with the same name, higher-priority versions win.

## Documentation

- [Installation](docs/getting-started/installation.md) — platform-specific setup
- [Quickstart](docs/getting-started/quickstart.md) — detailed walkthrough
- [CLI Reference](docs/reference/cli-reference.md) — commands and flags
- [Configuration](docs/reference/config-reference.md) — config.yaml options
- [Skill Format](docs/reference/skill-format.md) — SKILL.md specification
- [Troubleshooting](docs/troubleshooting.md) — common issues

## Current Limitations

- No authentication beyond SSH keys
- No automatic extraction yet (manual skill creation only)
- No skill quality filtering
- No semantic search
- Will definitely break

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TBD
