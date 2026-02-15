# CLI Reference

Complete reference for all Mycelium command-line interface commands.

## Overview

```
mycelium - Knowledge sharing infrastructure for AI agents

USAGE:
  mycelium [command] [flags]

DESCRIPTION:
  Mycelium is infrastructure where every problem solved once is solved for everyone.
  People work with agents. Agents extract what was learned. Other people's agents 
  absorb it. No one writes documentation. No one publishes anything. They just get 
  better results.

AVAILABLE COMMANDS:
  init      Initialize Mycelium workspace
  serve     Start the Mycelium git server  
  help      Help about any command

GLOBAL FLAGS:
  -h, --help      help for mycelium
  -v, --version   version for mycelium
```

## Global Commands

### `mycelium --version`

Show the Mycelium version information.

**Usage:**
```bash
mycelium --version
mycelium -v
```

**Output:**
```
mycelium version dev
```

**Notes:**
- Version is set during build via ldflags
- Development builds show `dev`
- Release builds show semantic version (e.g., `v1.0.0`)

### `mycelium help [command]`

Show help information for any command.

**Usage:**
```bash
mycelium help            # Show main help
mycelium help init       # Show help for init command  
mycelium help serve      # Show help for serve command
mycelium --help          # Alternative syntax
```

## Core Commands

### `mycelium init`

Initialize a new Mycelium workspace in `~/.mycelium/`.

**Usage:**
```bash
mycelium init
```

**Description:**
Initializes a new Mycelium workspace by creating necessary directories, configuration file, and git repository for managing local skills.

**What it creates:**
- `~/.mycelium/` directory
- `~/.mycelium/config.yaml` with default configuration
- `~/.mycelium/skills/` directory with git repository
- `~/.mycelium/skills/.gitignore` with common ignore patterns

**Example:**
```bash
$ mycelium init
INFO Initializing Mycelium workspace...
INFO Created directory path=~/.mycelium
INFO Created skills directory path=~/.mycelium/skills
INFO Created default config path=~/.mycelium/config.yaml
INFO Initialized git repository path=~/.mycelium/skills

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

**Behavior:**
- **Safe to run multiple times** — Won't overwrite existing files
- **Creates git repository** — Initializes skills directory as git repo with initial commit
- **Graceful degradation** — Continues if git is not available
- **Default config** — Creates config with sensible defaults

**Prerequisites:**
- Write permissions to home directory
- Git (optional but recommended)

### `mycelium serve`

Start the Mycelium git server for hosting skill repositories.

**Usage:**
```bash
mycelium serve [flags]
```

**Flags:**
- `--config string` — Config file path (default: `~/.mycelium/config.yaml`)

**Description:**
Starts a Soft Serve git server that hosts skill repositories. This is the source of truth for all Mycelium knowledge. Agents can git clone, push, and pull skill repositories over SSH.

**Example:**
```bash
$ mycelium serve
INFO Mycelium serve command started
INFO Configuration loaded successfully host=127.0.0.1 port=23231 dataDir=~/.mycelium/data storageDriver=sqlite libraries=1
INFO Mycelium git server is ready ssh_url=ssh://localhost:23231 host=127.0.0.1 port=23231
INFO Mycelium is ready. Use Ctrl+C to shutdown gracefully.
INFO Agents can now git clone, push, and pull over SSH
```

**Server lifecycle:**
1. **Startup** — Loads configuration, creates data directory, starts git server
2. **Ready** — Logs connection information, accepts SSH connections
3. **Shutdown** — Handles SIGINT/SIGTERM gracefully, stops git server cleanly

**Networking:**
- **Default binding:** `127.0.0.1:23231` (localhost only)
- **Configure via config.yaml:** Change `server.host` and `server.port`
- **SSH access:** Agents connect via `ssh://host:port`

**Data storage:**
- **Git repositories:** Stored in `server.dataDir` from config
- **Metadata database:** Location from `storage.dsn` in config
- **Auto-creation:** Creates data directory if it doesn't exist

**Flags:**

#### `--config`

Specify path to configuration file.

**Type:** `string`  
**Default:** `~/.mycelium/config.yaml`

**Examples:**
```bash
mycelium serve --config /etc/mycelium/config.yaml
mycelium serve --config ./dev-config.yaml
mycelium serve  # Uses default ~/.mycelium/config.yaml
```

**Graceful shutdown:**
- **Signals:** Responds to SIGINT (Ctrl+C) and SIGTERM
- **Clean exit:** Stops git server properly, closes database connections
- **No data loss:** Ongoing git operations complete before shutdown

**Error handling:**
- **Port in use:** Returns error if port is already bound
- **Config errors:** Validates configuration before starting  
- **Permission errors:** Reports filesystem permission issues clearly

## Planned Commands

These commands are planned for future releases:

### `mycelium remote` (Coming Soon)

Manage remote skill libraries.

**Planned usage:**
```bash
mycelium remote add <name> <url>     # Add remote library
mycelium remote list                 # List configured remotes  
mycelium remote remove <name>        # Remove remote library
```

### `mycelium skill` (Coming Soon)

Skill management commands.

**Planned usage:**
```bash
mycelium skill create <name>         # Create new skill template
mycelium skill validate <path>       # Validate SKILL.md format
mycelium skill list                  # List available skills
mycelium skill search <query>        # Search skills semantically
```

### `mycelium config` (Coming Soon)

Configuration management commands.

**Planned usage:**
```bash
mycelium config show                 # Display current config
mycelium config validate <path>      # Validate config file
mycelium config init                 # Create default config
```

### `mycelium extract` (Coming Soon)

Manual skill extraction from sessions.

**Planned usage:**
```bash
mycelium extract <session-file>      # Extract skills from session
mycelium extract --watch <dir>       # Watch for new sessions
```

## Common Usage Patterns

### Initial Setup

```bash
# 1. Initialize workspace
mycelium init

# 2. Configure author (edit ~/.mycelium/config.yaml)
# Set defaults.author to your identifier

# 3. Start server
mycelium serve
```

### Development/Testing

```bash
# Use custom config for testing
mycelium serve --config ./test-config.yaml

# Different terminal: test connection
ssh -p 23231 localhost

# Check server logs and stop with Ctrl+C
```

### Team Deployment

```bash
# 1. Deploy configuration to server
sudo cp config.yaml /etc/mycelium/config.yaml

# 2. Start server with custom config
mycelium serve --config /etc/mycelium/config.yaml

# 3. Team members connect via SSH
# ssh://mycelium.company.com:23231
```

## Exit Codes

Mycelium uses standard Unix exit codes:

- **0** — Success
- **1** — General error (config issues, startup failures)  
- **2** — Command line usage error (invalid flags, missing arguments)
- **130** — Interrupted by user (Ctrl+C)

## Environment Variables

Currently, Mycelium does not support configuration via environment variables. All configuration must be specified in the YAML config file.

**Planned:** Environment variable support for container deployments.

## Debugging

### Verbose Output

Currently, all logging goes to stdout/stderr. Log levels are controlled internally.

**Planned:** `--verbose` and `--debug` flags for detailed output.

### Configuration Issues

```bash
# Check if config is valid (manual inspection for now)
cat ~/.mycelium/config.yaml

# Test server startup
mycelium serve --config ~/.mycelium/config.yaml
```

### Connection Issues

```bash
# Test SSH connection to server
ssh -v -p 23231 localhost

# Check if port is available
netstat -an | grep 23231  # Linux/macOS
Get-NetTCPConnection -LocalPort 23231  # Windows PowerShell
```

### Git Issues

```bash
# Check git configuration
git config --list

# Test git operations manually
cd ~/.mycelium/skills
git status
git log --oneline
```

## Shell Completion

Shell completion is not currently implemented but is planned for a future release.

**Planned support:**
- Bash
- Zsh  
- Fish
- PowerShell

## Examples

### Basic Workflow

```bash
# Terminal 1: Start server
mycelium serve
# INFO Mycelium is ready. Use Ctrl+C to shutdown gracefully.

# Terminal 2: Test setup  
ssh -p 23231 localhost
# Should connect to Soft Serve TUI

# Create test skill
cd ~/.mycelium/skills
mkdir test-skill
echo "---
name: test-skill  
version: 1
author: me@example.com
confidence: 0.8
created: $(date +%Y-%m-%d)
---

# Test Skill

## When to Use
Testing the setup.

## Solution  
It works!" > test-skill/SKILL.md

# Commit skill
git add test-skill/
git commit -m "Add test skill"
```

### Multi-Environment Setup

```bash
# Development environment
mycelium serve --config ./dev-config.yaml

# Production environment  
mycelium serve --config /etc/mycelium/prod-config.yaml

# Testing environment
mycelium serve --config ./test-config.yaml
```

## See Also

- **[Configuration Reference](config-reference.md)** — Complete config options
- **[Quickstart Guide](../getting-started/quickstart.md)** — Get started quickly  
- **[Troubleshooting](../troubleshooting.md)** — Common issues and solutions