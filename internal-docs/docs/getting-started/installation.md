# Installation

## Quick Install (from source)

Kinoko is currently installed from source. Pre-built binaries are not yet available.

```bash
git clone https://github.com/kinoko-dev/kinoko.git
cd kinoko
go install ./cmd/kinoko
```

Verify:
```bash
kinoko --version
```

## Requirements

### Go 1.24+

```bash
go version
# Should output: go version go1.24.x ...
```

Install Go: https://golang.org/dl/ or via package manager:
- **macOS:** `brew install go`
- **Ubuntu/Debian:** `sudo apt install golang-go` (check version — distro packages may be old)
- **Windows:** Official installer or `scoop install go`

### Git

```bash
git --version
```

Usually pre-installed. If not:
- **macOS:** `brew install git` or Xcode Command Line Tools
- **Ubuntu/Debian:** `sudo apt install git`
- **Windows:** https://git-scm.com/download/win

### SSH Client

```bash
ssh -V
```

Pre-installed on macOS, most Linux distros, and Windows 10+.

## Post-Install Setup

```bash
# Initialize workspace
kinoko init

# Terminal 1 — Start the server
kinoko serve

# Terminal 2 — Start the local daemon
kinoko run --server localhost:23231
```

Kinoko uses a two-process model: `serve` runs shared infrastructure (git, API, index DB) and `run` runs the local agent daemon (workers, scheduler, injection, queue DB). Both must be running for full functionality.

See the [Quickstart](quickstart.md) for a complete walkthrough.

## Common Issues

### "command not found: kinoko"

Your Go bin directory isn't in PATH:

```bash
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

Adjust for your shell (`~/.zshrc` for Zsh, `~/.config/fish/config.fish` for Fish).

### "go: command not found"

Go isn't installed or isn't in PATH. Install from https://golang.org/dl/ and ensure `/usr/local/go/bin` is in PATH.

## Uninstall

```bash
rm $(which kinoko)
rm -rf ~/.kinoko  # WARNING: deletes all local skills and config
```
