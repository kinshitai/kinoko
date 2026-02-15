# Quickstart Guide

Get from zero to a working Mycelium setup in 5 minutes.

## Prerequisites

- **Go 1.21+** — Check with `go version`
- **Git** — Check with `git --version`  
- **SSH client** — Usually pre-installed

## 1. Install Mycelium

```bash
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest
```

Verify installation:
```bash
mycelium --version
# Should output: mycelium version dev (or a version number)
```

## 2. Initialize Your Workspace

```bash
mycelium init
```

This creates:
- `~/.mycelium/` directory
- `~/.mycelium/config.yaml` with default settings
- `~/.mycelium/skills/` git repository for local skills
- `~/.mycelium/data/` for server data

You should see:
```
🍄 Mycelium initialized successfully!

Your Mycelium workspace is ready at ~/.mycelium/

Next steps:
  • Edit ~/.mycelium/config.yaml to configure your setup
  • Set your preferred author in the config file
  • Run 'mycelium serve' to start the git server
  • Agents can then git clone, push, and pull skill repositories over SSH

Your local skills will be stored in ~/.mycelium/skills/
This directory is already a git repository for version control.
```

## 3. Configure Your Author

Edit `~/.mycelium/config.yaml` and set your author identifier:

```yaml
defaults:
  author: "your-email@example.com"  # or "your-username" 
  confidence: 0.7
```

This will be used when creating skills automatically.

## 4. Start the Server

```bash
mycelium serve
```

You should see:
```
INFO Mycelium serve command started
INFO Configuration loaded successfully host=127.0.0.1 port=23231 dataDir=~/.mycelium/data
INFO Mycelium git server is ready ssh_url=ssh://localhost:23231
INFO Mycelium is ready. Use Ctrl+C to shutdown gracefully.
INFO Agents can now git clone, push, and pull over SSH
```

**Keep this terminal open** — the server needs to run for agents to access skills.

## 5. Test Your Setup

Open a new terminal and test that the git server is working:

```bash
# Test SSH connection to your Mycelium server
ssh -p 23231 localhost

# Should show a Soft Serve TUI or connection info
# Press 'q' to quit if a TUI appears
```

If SSH works, your Mycelium server is ready for agent integration.

## 6. Create a Test Skill (Optional)

Create a sample skill to verify everything works:

```bash
cd ~/.mycelium/skills
```

Create `test-skill/SKILL.md`:
```bash
mkdir test-skill
cat > test-skill/SKILL.md << 'EOF'
---
name: test-skill
version: 1
tags: [test]
author: your-email@example.com
confidence: 0.8
created: 2026-02-15
---

# Test Skill

## When to Use
This is a test skill to verify Mycelium setup.

## Solution
If you can read this, your Mycelium installation is working!

## See Also
- Check the [quickstart guide](../docs/getting-started/quickstart.md) for more info
EOF
```

Validate the skill format:
```bash
# Go to the mycelium project directory to use the parser (if available)
# This is mainly for development verification
```

Commit the test skill:
```bash
cd ~/.mycelium/skills
git add test-skill/
git commit -m "Add test skill to verify setup"
```

## What's Next?

You now have:
- ✅ Mycelium server running on `ssh://localhost:23231`
- ✅ Local skills directory at `~/.mycelium/skills/`  
- ✅ Configuration ready for customization
- ✅ Test skill demonstrating the format

### Agent Integration

**For OpenClaw/Claude Code users:** Integration hooks are coming in Phase 2. The server is ready — agents just need hooks to extract and inject skills.

**For other AI frameworks:** The skill format is documented at [docs/reference/skill-format.md](../reference/skill-format.md). Agents need to:
1. Extract knowledge into `SKILL.md` format
2. Push to the git server over SSH
3. Pull and search existing skills for context injection

### Add Remote Libraries

To connect to team or community skill libraries:

```yaml
# Add to ~/.mycelium/config.yaml under libraries:
libraries:
  - name: local
    path: ~/.mycelium/skills
    priority: 100
    description: "Local skills on this machine"
  
  - name: team  
    url: ssh://mycelium.company.com:23231
    priority: 50
    description: "Company skill library"
```

### Self-Hosting for Teams

To run Mycelium for a team:
1. Deploy on a server (same `mycelium serve` command)
2. Configure SSH access for team members
3. Each team member runs `mycelium init` and adds the remote
4. Skills automatically flow between team members

## Troubleshooting

If something didn't work, check [Troubleshooting](../troubleshooting.md) for common issues.

**Most common issues:**
- **Port 23231 already in use:** Change the port in `~/.mycelium/config.yaml`
- **SSH connection refused:** Check firewall settings or use `ssh -v` for debug info
- **Permission denied:** Ensure SSH keys are set up correctly

## Getting Help

- **Documentation:** [docs/](../) 
- **Architecture:** [rfcs/002-architecture-and-roadmap.md](../../rfcs/002-architecture-and-roadmap.md)
- **Vision:** [MANIFESTO.md](../../MANIFESTO.md)

You're ready to build with Mycelium! 🍄