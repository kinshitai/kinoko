# Troubleshooting

Common issues and solutions when using Mycelium.

## Installation Issues

### "command not found: mycelium"

**Symptoms:**
```bash
$ mycelium --version
bash: mycelium: command not found
```

**Cause:** Binary not in PATH after `go install`.

**Solution:**
```bash
# Check if GOPATH/bin is in PATH
echo $PATH | grep -o $GOPATH/bin

# If not found, add to your shell profile
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# Alternative: check where go install puts binaries
go env GOPATH
ls $(go env GOPATH)/bin/mycelium
```

**For different shells:**
- **Bash:** `~/.bashrc` or `~/.bash_profile`
- **Zsh:** `~/.zshrc`  
- **Fish:** `~/.config/fish/config.fish`

### "go: command not found"

**Symptoms:**
```bash
$ go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest
bash: go: command not found
```

**Cause:** Go is not installed or not in PATH.

**Solution:**
1. Install Go from https://golang.org/dl/
2. Ensure Go's bin directory is in PATH:
   ```bash
   export PATH=/usr/local/go/bin:$PATH
   ```
3. Verify: `go version`

### "cannot find module" during install

**Symptoms:**
```bash  
$ go install github.com/mycelium-dev/mycelium/cmd/mycelium@latest
go: github.com/mycelium-dev/mycelium@latest: module github.com/mycelium-dev/mycelium: reading https://proxy.golang.org/github.com/mycelium-dev/mycelium/@v/list: 404 Not Found
```

**Cause:** Repository doesn't exist at expected path (early development phase).

**Solution:** Install from actual source:
```bash
# Clone actual repository (adjust URL as needed)
git clone [actual-repo-url] mycelium
cd mycelium
go install ./cmd/mycelium
```

## Server Startup Issues

### Port 23231 already in use

**Symptoms:**
```bash
$ mycelium serve
Error: failed to start git server: listen tcp 127.0.0.1:23231: bind: address already in use
```

**Diagnosis:**
```bash
# Check what's using the port
lsof -i :23231                    # macOS/Linux
netstat -an | grep 23231          # Linux  
Get-NetTCPConnection -LocalPort 23231  # Windows PowerShell
```

**Solutions:**

1. **Stop the conflicting service:**
   ```bash
   # If another Mycelium instance
   pkill mycelium
   
   # If other service, identify and stop it
   sudo kill -9 <PID>
   ```

2. **Change Mycelium's port:**
   Edit `~/.mycelium/config.yaml`:
   ```yaml
   server:
     host: "127.0.0.1"
     port: 2323  # Use different port
   ```

3. **Use command line override (if supported):**
   ```bash
   # Future feature
   mycelium serve --port 2323
   ```

### Permission denied when creating data directory

**Symptoms:**
```bash
$ mycelium serve
Error: failed to create data directory ~/.mycelium/data: permission denied
```

**Cause:** Insufficient permissions to create directories.

**Solutions:**

1. **Fix permissions:**
   ```bash
   chmod 755 ~/.mycelium
   mkdir -p ~/.mycelium/data
   ```

2. **Change data directory:**
   Edit `~/.mycelium/config.yaml`:
   ```yaml
   server:
     dataDir: ~/mycelium-data  # Use different location
   ```

3. **Check disk space:**
   ```bash
   df -h ~  # Check available space
   ```

### Configuration validation errors

**Symptoms:**
```bash
$ mycelium serve
Error: invalid configuration: server port must be between 1 and 65535, got 70000
```

**Common validation errors:**

1. **Invalid port range:**
   ```yaml
   server:
     port: 8080  # Must be 1-65535
   ```

2. **Empty required fields:**
   ```yaml
   server:
     host: "127.0.0.1"  # Cannot be empty
     dataDir: "~/.mycelium/data"  # Cannot be empty
   ```

3. **Invalid confidence ranges:**
   ```yaml
   defaults:
     confidence: 0.8  # Must be 0.0-1.0
   ```

4. **Library configuration errors:**
   ```yaml
   libraries:
     - name: "local"  # Name cannot be empty
       path: "~/.mycelium/skills"  # Need either path or URL
       priority: 100  # Cannot be negative
   ```

## SSH Connection Issues

### SSH connection refused

**Symptoms:**
```bash
$ ssh -p 23231 localhost  
ssh: connect to host localhost port 23231: Connection refused
```

**Causes and solutions:**

1. **Server not running:**
   ```bash
   # Start the server in another terminal
   mycelium serve
   ```

2. **Firewall blocking connection:**
   ```bash
   # macOS: Check firewall settings
   sudo /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate
   
   # Linux: Check iptables  
   sudo iptables -L
   
   # Windows: Check Windows Defender Firewall
   ```

3. **SSH service not available:**
   ```bash
   # Check if SSH client is available
   ssh -V
   
   # Install if missing (rare)
   # macOS: Install Xcode command line tools
   # Linux: sudo apt install openssh-client
   ```

### SSH authentication issues

**Symptoms:**
```bash
$ ssh -p 23231 localhost
Permission denied (publickey).
```

**Diagnosis:**
```bash
# Use verbose SSH to see what's happening
ssh -v -p 23231 localhost
```

**Solutions:**

1. **Generate SSH keys if missing:**
   ```bash
   ssh-keygen -t rsa -b 4096 -C "your-email@example.com"
   ssh-add ~/.ssh/id_rsa
   ```

2. **Check SSH agent:**
   ```bash
   ssh-add -l  # List loaded keys
   ssh-add ~/.ssh/id_rsa  # Add key if missing
   ```

3. **For remote servers, ensure key is authorized:**
   ```bash
   # Copy public key to server admin
   cat ~/.ssh/id_rsa.pub
   ```

### SSH connection hangs

**Symptoms:**
SSH command hangs without connecting or rejecting.

**Diagnosis:**
```bash  
# Use verbose mode with timeout
timeout 10 ssh -v -p 23231 localhost
```

**Solutions:**

1. **Network connectivity issues:**
   ```bash
   # Test basic connectivity
   telnet localhost 23231
   nc -v localhost 23231
   ```

2. **DNS resolution issues:**
   ```bash
   # Use IP address instead of hostname
   ssh -p 23231 127.0.0.1
   ```

3. **Host key verification issues:**
   ```bash
   # Remove old host key if server was reinstalled
   ssh-keygen -R "[localhost]:23231"
   ```

## Git Repository Issues

### "Not a git repository" errors

**Symptoms:**
```bash
$ cd ~/.mycelium/skills
$ git status
fatal: not a git repository (or any of the parent directories): .git
```

**Cause:** Skills directory wasn't initialized as git repository.

**Solution:**
```bash
cd ~/.mycelium/skills
git init
git add .gitignore
git commit -m "Initial commit"
```

### Git user not configured

**Symptoms:**
```bash
$ git commit -m "Add skill"
Author identity unknown

*** Please tell me who you are.

Run
  git config --global user.email "you@example.com"
  git config --global user.name "Your Name"
```

**Solution:**
```bash
git config --global user.name "Your Name"
git config --global user.email "you@example.com"

# Or configure per repository
cd ~/.mycelium/skills  
git config user.name "Your Name"
git config user.email "you@example.com"
```

### Permission denied on git operations

**Symptoms:**
```bash
$ git push origin main
Permission denied (publickey).
fatal: Could not read from remote repository.
```

**Cause:** SSH authentication not set up for git remote.

**Solution:** See SSH authentication issues above.

## Skill Format Issues

### SKILL.md validation errors

**Symptoms:**
When parsing skills, you get format validation errors.

**Common errors:**

1. **Missing front matter delimiters:**
   ```markdown
   name: my-skill  ❌ Missing --- delimiters
   version: 1
   
   # My Skill
   ```
   
   **Fix:**
   ```markdown
   ---
   name: my-skill  ✅ Proper front matter
   version: 1
   ---
   
   # My Skill
   ```

2. **Invalid name format:**
   ```yaml
   ---
   name: "My Skill"  ❌ Must be kebab-case
   ---
   ```
   
   **Fix:**
   ```yaml
   ---
   name: my-skill  ✅ Lowercase with hyphens
   ---
   ```

3. **Invalid confidence range:**
   ```yaml
   ---
   confidence: 1.5  ❌ Must be 0.0-1.0
   ---
   ```
   
   **Fix:**
   ```yaml
   ---
   confidence: 0.9  ✅ Within valid range
   ---
   ```

4. **Missing required sections:**
   ```markdown
   # My Skill
   
   This is my skill.  ❌ Missing required sections
   ```
   
   **Fix:**
   ```markdown
   # My Skill
   
   ## When to Use  ✅ Required section
   Description of when this applies.
   
   ## Solution  ✅ Required section
   The actual knowledge content.
   ```

## Database Issues

### SQLite database locked

**Symptoms:**
```bash
Error: failed to connect to database: database is locked
```

**Causes:**
1. Another Mycelium process is running
2. SQLite file permissions issues
3. Unclean shutdown left lock file

**Solutions:**

1. **Stop other processes:**
   ```bash
   pkill mycelium
   ```

2. **Remove lock files:**
   ```bash
   rm ~/.mycelium/mycelium.db-wal
   rm ~/.mycelium/mycelium.db-shm
   ```

3. **Check file permissions:**
   ```bash
   ls -la ~/.mycelium/mycelium.db*
   chmod 644 ~/.mycelium/mycelium.db
   ```

### Database corruption

**Symptoms:**
```bash
Error: database disk image is malformed
```

**Recovery:**
```bash
# Backup corrupted database
cp ~/.mycelium/mycelium.db ~/.mycelium/mycelium.db.corrupted

# Try to recover
sqlite3 ~/.mycelium/mycelium.db ".recover" | sqlite3 ~/.mycelium/mycelium.db.recovered

# If recovery works, replace original
mv ~/.mycelium/mycelium.db.recovered ~/.mycelium/mycelium.db

# Otherwise, start fresh (loses metadata, but skills in git are safe)
rm ~/.mycelium/mycelium.db
mycelium serve  # Will recreate database
```

## Performance Issues

### Slow skill search/injection

**Symptoms:** Agent integration is slow when searching for relevant skills.

**Causes:**
1. Large skill database without indexes
2. Embedding index not optimized
3. Network latency to remote libraries

**Solutions:**

1. **Check database size:**
   ```bash
   ls -lh ~/.mycelium/mycelium.db
   ```

2. **Optimize SQLite database:**
   ```bash
   sqlite3 ~/.mycelium/mycelium.db "VACUUM; REINDEX;"
   ```

3. **Review library priorities:**
   ```yaml
   libraries:
     # Put frequently used libraries first
     - name: local
       priority: 100
     - name: team  
       priority: 50
   ```

### High memory usage

**Symptoms:** Mycelium server using excessive memory.

**Diagnosis:**
```bash
# Check process memory usage
ps aux | grep mycelium
top -p $(pgrep mycelium)
```

**Solutions:**
1. Restart server periodically
2. Check for memory leaks (report bug)
3. Use PostgreSQL instead of SQLite for large deployments

## Network Issues

### Can't connect to remote libraries

**Symptoms:**
```bash
Error: failed to connect to remote library: connection timeout
```

**Diagnosis:**
```bash
# Test network connectivity
ping mycelium.company.com
telnet mycelium.company.com 23231
```

**Solutions:**

1. **Check URL in config:**
   ```yaml
   libraries:
     - name: remote
       url: ssh://mycelium.company.com:23231  # Verify host/port
   ```

2. **VPN/firewall issues:**
   - Connect to company VPN if required
   - Check corporate firewall rules
   - Try different ports if blocked

3. **SSH configuration:**
   ```bash
   # Test SSH connection manually
   ssh -p 23231 mycelium.company.com
   ```

## Debugging Tips

### Enable verbose logging

Currently not implemented, but when troubleshooting:

1. **Check server logs** (stdout/stderr from `mycelium serve`)
2. **Use SSH verbose mode** (`ssh -v`)
3. **Check system logs** for related errors
4. **Test components individually** (git, SSH, database)

### Collect diagnostic information

When reporting bugs, include:

```bash
# System information
uname -a
go version
git --version
ssh -V

# Mycelium information  
mycelium --version
cat ~/.mycelium/config.yaml

# Server logs (last 50 lines)
mycelium serve 2>&1 | tail -50

# Network test
ssh -v -p 23231 localhost 2>&1 | head -20
```

## Getting Help

If troubleshooting doesn't solve your issue:

1. **Check documentation:** [docs/](../docs/)
2. **Review RFCs:** [rfcs/](../rfcs/) for architectural context
3. **File an issue:** Include diagnostic information above
4. **Community:** Join discussions (links TBD)

## Common Workflow Issues

### "No skills found" when searching

**Symptoms:** Agent integration reports no relevant skills found.

**Causes:**
1. No skills in configured libraries
2. Search query too specific
3. Skills not properly indexed

**Solutions:**

1. **Check skill availability:**
   ```bash
   ls ~/.mycelium/skills/
   git log --oneline  # In skills directory
   ```

2. **Verify library configuration:**
   ```yaml
   libraries:
     - name: local
       path: ~/.mycelium/skills  # Check path exists
       priority: 100
   ```

3. **Manual skill search (future feature):**
   ```bash
   # Will be available in future versions
   mycelium skill search "debugging golang"
   ```

### Skills not updating from remote

**Symptoms:** Local skills don't reflect changes pushed to remote library.

**Cause:** Manual git operations needed (automatic sync coming in Phase 2).

**Solution:**
```bash
cd ~/.mycelium/skills
git pull origin main  # Update from remote
```

Remember: **Git repositories are the source of truth**. When in doubt, check the actual git repositories for the real state of your skills.