# Quickstart

Get from zero to a working Kinoko setup in 5 minutes.

## Prerequisites

- Go 1.24+ (`go version`)
- Git (`git --version`)
- SSH client (`ssh -V`)

## 1. Install

```bash
git clone https://github.com/kinoko-dev/kinoko.git
cd kinoko
go install ./cmd/kinoko
kinoko --version
```

## 2. Initialize Your Workspace

```bash
kinoko init
```

This creates:
- `~/.kinoko/config.yaml` — configuration
- `~/.kinoko/skills/` — local skills (git repo)

You'll see:
```
🍄 Kinoko initialized successfully!

Your Kinoko workspace is ready at ~/.kinoko/
```

## 3. Set Your Author

Edit `~/.kinoko/config.yaml`:

```yaml
defaults:
  author: "you@example.com"
```

## 4. Start the Server

Kinoko uses a two-process model. First, start the shared infrastructure server:

```bash
kinoko serve
```

Output:
```
INFO Kinoko serve command started
INFO Configuration loaded successfully host=127.0.0.1 port=23231 ...
INFO Kinoko git server is ready ssh_url=ssh://localhost:23231
INFO Kinoko is ready. Use Ctrl+C to shutdown gracefully.
```

Keep this terminal open. `serve` runs the git server, discovery API, credential-scanning hooks, and the index DB.

## 5. Start the Local Daemon

In a **second terminal**, start the agent daemon:

```bash
export OPENAI_API_KEY=sk-...
kinoko run --server localhost:23231
```

`run` starts the worker pool, extraction pipeline, scheduler (decay, stale sweep), and injection system. It communicates with `serve` over HTTP for indexing, search, and embedding.

> **Note:** `kinoko run` requires a running `kinoko serve`. If the server is unreachable, `run` will log connection errors and operate in degraded mode.

## 6. Test the Server

In another terminal:

```bash
ssh -p 23231 localhost
```

You should see a Soft Serve TUI or connection info. Press `q` to quit.

## 7. Create a Test Skill

```bash
mkdir -p ~/.kinoko/skills/test-skill
cat > ~/.kinoko/skills/test-skill/SKILL.md << 'EOF'
---
name: test-skill
version: 1
tags: [test]
author: you@example.com
confidence: 0.8
created: 2026-02-15
---

# Test Skill

## When to Use
Verifying your Kinoko setup works.

## Solution
If you can read this, your installation is working.
EOF

cd ~/.kinoko/skills
git add test-skill/
git commit -m "Add test skill"
```

## What You Have Now

- ✅ `kinoko serve` running on `ssh://localhost:23231` (git + API + index DB)
- ✅ `kinoko run` connected to serve (workers + scheduler + injection + queue DB)
- ✅ Local skills repo at `~/.kinoko/skills/`
- ✅ A test skill demonstrating the format

## Next Steps

- [Configuration Reference](../reference/config-reference.md) — customize your setup
- [Skill Format](../reference/skill-format.md) — understand SKILL.md
- [CLI Reference](../reference/cli-reference.md) — all commands and flags
- [Troubleshooting](../troubleshooting.md) — if something went wrong
