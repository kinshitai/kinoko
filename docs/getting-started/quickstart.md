# Quickstart

Get from zero to a working Mycelium setup in 5 minutes.

## Prerequisites

- Go 1.24+ (`go version`)
- Git (`git --version`)
- SSH client (`ssh -V`)

## 1. Install

```bash
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium
go install ./cmd/mycelium
mycelium --version
```

## 2. Initialize Your Workspace

```bash
mycelium init
```

This creates:
- `~/.mycelium/config.yaml` — configuration
- `~/.mycelium/skills/` — local skills (git repo)

You'll see:
```
🍄 Mycelium initialized successfully!

Your Mycelium workspace is ready at ~/.mycelium/
```

## 3. Set Your Author

Edit `~/.mycelium/config.yaml`:

```yaml
defaults:
  author: "you@example.com"
```

## 4. Start the Server

```bash
mycelium serve
```

Output:
```
INFO Mycelium serve command started
INFO Configuration loaded successfully host=127.0.0.1 port=23231 ...
INFO Mycelium git server is ready ssh_url=ssh://localhost:23231
INFO Mycelium is ready. Use Ctrl+C to shutdown gracefully.
```

Keep this terminal open.

## 5. Test It

In another terminal:

```bash
ssh -p 23231 localhost
```

You should see a Soft Serve TUI or connection info. Press `q` to quit.

## 6. Create a Test Skill

```bash
mkdir -p ~/.mycelium/skills/test-skill
cat > ~/.mycelium/skills/test-skill/SKILL.md << 'EOF'
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
Verifying your Mycelium setup works.

## Solution
If you can read this, your installation is working.
EOF

cd ~/.mycelium/skills
git add test-skill/
git commit -m "Add test skill"
```

## What You Have Now

- ✅ Mycelium server on `ssh://localhost:23231`
- ✅ Local skills repo at `~/.mycelium/skills/`
- ✅ A test skill demonstrating the format

## Next Steps

- [Configuration Reference](../reference/config-reference.md) — customize your setup
- [Skill Format](../reference/skill-format.md) — understand SKILL.md
- [CLI Reference](../reference/cli-reference.md) — all commands and flags
- [Troubleshooting](../troubleshooting.md) — if something went wrong
