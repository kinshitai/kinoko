# Installation Guide

Complete installation instructions for Mycelium on all platforms.

## Quick Install

**If you have Go 1.21+ installed:**

```bash
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest
```

**Verify installation:**
```bash
mycelium --version
# Should output: mycelium version dev (or version number)
```

## Platform-Specific Installation

### macOS

**Option 1: Using Go (Recommended)**
```bash
# Install Go if not already installed
brew install go

# Install Mycelium
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest

# Verify installation
mycelium --version
```

**Option 2: Download Binary (Coming Soon)**
Pre-built binaries will be available in future releases.

### Linux

**Option 1: Using Go (Recommended)**
```bash
# Install Go (varies by distribution)
# Ubuntu/Debian:
sudo apt update && sudo apt install golang-go

# Fedora:
sudo dnf install golang

# Arch:
sudo pacman -S go

# Install Mycelium
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest

# Verify installation  
mycelium --version
```

**Option 2: Download Binary (Coming Soon)**
Pre-built binaries will be available in future releases.

### Windows

**Option 1: Using Go (Recommended)**
```powershell
# Install Go from https://golang.org/dl/
# Or using Scoop:
scoop install go

# Install Mycelium
go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest

# Verify installation
mycelium --version
```

**Option 2: Download Binary (Coming Soon)**
Pre-built binaries will be available in future releases.

## Required Dependencies

### Go (Required for installation)

**Minimum version:** Go 1.21+

**Check version:**
```bash
go version
# Should output: go version go1.21.x ...
```

**Install Go:**
- **Official installer:** https://golang.org/dl/
- **macOS:** `brew install go`
- **Ubuntu/Debian:** `sudo apt install golang-go`
- **Windows:** Use official installer or `scoop install go`

### Git (Required for operation)

Mycelium uses git for skill storage and version control.

**Check if installed:**
```bash
git --version
# Should output: git version 2.x.x
```

**Install Git:**
- **macOS:** `brew install git` (or use Xcode Command Line Tools)
- **Ubuntu/Debian:** `sudo apt install git`
- **Fedora:** `sudo dnf install git`
- **Windows:** https://git-scm.com/download/win

### SSH Client (Usually pre-installed)

Required for connecting to Mycelium git servers.

**Check if available:**
```bash
ssh -V
# Should output: OpenSSH_x.x ...
```

**Usually pre-installed on:**
- macOS ✅
- Most Linux distributions ✅  
- Windows 10+ ✅

**Manual install (rare):**
- **Windows (older):** Install Git for Windows (includes SSH)
- **Minimal Linux:** `sudo apt install openssh-client`

## Build from Source

For development or latest features:

```bash
# Clone the repository
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium

# Build and install
go build -o mycelium ./cmd/mycelium
sudo mv mycelium /usr/local/bin/

# Or install to GOPATH/bin
go install ./cmd/mycelium
```

## Verify Installation

After installation, verify everything works:

```bash
# Check Mycelium version
mycelium --version

# Initialize a test workspace (optional)
mycelium init

# Check that all dependencies are working
git --version
ssh -V
go version
```

## Post-Installation Setup

### 1. Initialize Workspace

```bash
mycelium init
```

This creates `~/.mycelium/` with configuration and local skills repository.

### 2. Configure SSH Keys (for remote servers)

If you'll connect to remote Mycelium servers, ensure SSH keys are set up:

```bash
# Generate SSH key if you don't have one
ssh-keygen -t rsa -b 4096 -C "your-email@example.com"

# Add to SSH agent
ssh-add ~/.ssh/id_rsa

# Copy public key to share with server administrators
cat ~/.ssh/id_rsa.pub
```

### 3. Test Local Server

```bash
# Start the server
mycelium serve

# In another terminal, test connection
ssh -p 23231 localhost
```

## Common Installation Issues

### "command not found: mycelium"

**Cause:** Binary not in PATH.

**Solution:**
```bash
# Check if GOPATH/bin is in PATH
echo $PATH | grep -o $GOPATH/bin

# If not, add to your shell profile (~/.bashrc, ~/.zshrc, etc.)
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc
```

### "go: command not found"

**Cause:** Go is not installed or not in PATH.

**Solution:**
1. Install Go from https://golang.org/dl/
2. Ensure `/usr/local/go/bin` (or Go installation path) is in PATH
3. Restart your terminal

### "Package github.com/mycelium-dev/mycelium: cannot find module"

**Cause:** Module path doesn't exist yet (early development).

**Solution:**
Currently, install from source:
```bash
git clone [actual-repo-url]
cd mycelium  
go install ./cmd/mycelium
```

### Permission denied when starting server

**Cause:** Port 23231 requires privileges or is in use.

**Solution:**
1. Change port in `~/.mycelium/config.yaml`:
   ```yaml
   server:
     port: 2323  # Use higher port number
   ```
2. Or run with different port: Check if something is using port 23231:
   ```bash
   lsof -i :23231  # macOS/Linux
   netstat -an | grep 23231  # Windows
   ```

### SSH connection refused

**Cause:** Firewall blocking connection or server not running.

**Solution:**
1. Ensure `mycelium serve` is running
2. Check firewall settings
3. Try `ssh -v -p 23231 localhost` for debug output
4. For remote servers, ensure port is open and accessible

### Git operations fail

**Cause:** Git not configured or SSH authentication issues.

**Solution:**
```bash
# Configure git if not done
git config --global user.name "Your Name"
git config --global user.email "your-email@example.com"

# For SSH issues, test key authentication
ssh -T -p 23231 localhost
```

## Next Steps

Once installed:
1. **[Quickstart Guide](quickstart.md)** — Get running in 5 minutes
2. **[Configuration Reference](../reference/config-reference.md)** — Customize your setup  
3. **[CLI Reference](../reference/cli-reference.md)** — Learn all commands

## Uninstallation

To completely remove Mycelium:

```bash
# Remove binary
rm $(which mycelium)

# Remove workspace (WARNING: deletes all local skills)
rm -rf ~/.mycelium

# Remove from GOPATH (if installed via go install)
rm $GOPATH/bin/mycelium
```