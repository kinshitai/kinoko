# CLI Reference

## Overview

```
mycelium - Knowledge sharing infrastructure for AI agents

USAGE:
  mycelium [command] [flags]

AVAILABLE COMMANDS:
  init      Initialize Mycelium workspace
  serve     Start the Mycelium git server
  help      Help about any command

GLOBAL FLAGS:
  -h, --help      Help for mycelium
      --version   Version for mycelium
```

## `mycelium init`

Initialize a new Mycelium workspace in `~/.mycelium/`.

```bash
mycelium init
```

**Creates:**
- `~/.mycelium/` — workspace root
- `~/.mycelium/config.yaml` — default configuration
- `~/.mycelium/skills/` — local skills directory (initialized as a git repo)

**Behavior:**
- Safe to run multiple times — won't overwrite existing files
- Skips git init gracefully if git isn't installed
- Creates an initial commit with `.gitignore`

## `mycelium serve`

Start the Soft Serve git server for hosting skill repositories.

```bash
mycelium serve [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `~/.mycelium/config.yaml` | Path to config file |

**Behavior:**
- Binds to `server.host`:`server.port` from config (default `127.0.0.1:23231`)
- Creates `server.dataDir` if it doesn't exist
- Handles SIGINT/SIGTERM for graceful shutdown
- Agents connect via SSH: `ssh://host:port`

**Example:**
```bash
# Default config
mycelium serve

# Custom config
mycelium serve --config ./my-config.yaml
```

## `mycelium --version`

Print the version string. Development builds show `dev`; release builds show a semantic version.

```bash
mycelium --version
```

## `mycelium help`

Show help for any command.

```bash
mycelium help
mycelium help init
mycelium help serve
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (config issues, startup failures) |
| 130 | Interrupted (Ctrl+C) |
