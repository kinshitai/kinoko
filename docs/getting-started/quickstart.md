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

Keep this terminal open.

## 5. Test It

In another terminal:

```bash
ssh -p 23231 localhost
```

You should see a Soft Serve TUI or connection info. Press `q` to quit.

## 6. Create a Test Skill

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

- ✅ Kinoko server on `ssh://localhost:23231`
- ✅ Local skills repo at `~/.kinoko/skills/`
- ✅ A test skill demonstrating the format

## Next Steps

- [Configuration Reference](../reference/config-reference.md) — customize your setup
- [Skill Format](../reference/skill-format.md) — understand SKILL.md
- [CLI Reference](../reference/cli-reference.md) — all commands and flags
- [Troubleshooting](../troubleshooting.md) — if something went wrong
