# Installation

## Quick Install (from source)

Mycelium is currently installed from source. Pre-built binaries are not yet available.

```bash
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium
go install ./cmd/mycelium
```

Verify:
```bash
mycelium --version
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
mycelium init

# Start server
mycelium serve
```

See the [Quickstart](quickstart.md) for a complete walkthrough.

## Common Issues

### "command not found: mycelium"

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
rm $(which mycelium)
rm -rf ~/.mycelium  # WARNING: deletes all local skills and config
```
