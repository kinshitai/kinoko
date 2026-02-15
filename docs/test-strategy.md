# Mycelium Test Strategy

## Product Understanding

**What is Mycelium?**
- Infrastructure for automatic knowledge sharing between AI agents
- Phase 1: Git-based skill repository server using Soft Serve
- Skills stored as repo-per-skill, SKILL.md format with YAML front matter
- Self-hosted first, with layered library support (local > company > public)

**Who uses it?**
- AI agent developers (Phase 1: Hal, Egor, their agents)
- Future: Any developer working with AI agents who wants automatic knowledge extraction/injection

**Core Value Proposition:**
- "Knowledge sharing should be a byproduct of work, not a separate activity"
- Agents extract skills from sessions → store in git repos → other agents inject relevant skills automatically
- Zero human documentation effort

## Critical User Journeys

### 1. Fresh Install Flow (`mycelium init`)
**User story:** Developer installs Mycelium for the first time
**Critical path:** `mycelium init` → creates workspace → ready to use

**Success criteria:**
- Creates `~/.mycelium/` directory structure
- Generates valid `config.yaml` with sensible defaults
- Initializes git repo in `skills/` directory
- Idempotent (safe to run multiple times)
- Clear success message with next steps

**Edge cases to test:**
- No home directory permissions
- Existing `.mycelium/` directory (partial)
- Existing config file (don't overwrite)
- Missing `git` binary
- Disk full during creation
- Unicode/special characters in home directory path

### 2. Server Startup Flow (`mycelium serve`)
**User story:** Developer starts the Mycelium server
**Critical path:** `mycelium serve` → loads config → starts Soft Serve → ready for git operations

**Success criteria:**
- Server starts and binds to configured port
- SSH server accepts connections
- Admin SSH keys generated and configured
- Connection info displayed to user
- Graceful shutdown on Ctrl+C

**Edge cases to test:**
- Port already in use
- Missing `soft` binary
- Invalid config file
- Permission denied for port binding
- SSH key generation failures
- Disk full in data directory
- Process crashes during startup
- Multiple server instances (port conflicts)

### 3. Skill Repository Lifecycle
**User story:** Agent/developer manages skill repositories
**Critical path:** Create repo → Clone → Push skill → Verify contents

**Success criteria:**
- Repo creation via SSH commands works
- Git clone operations work (SSH and HTTP)
- Push operations store files correctly
- Repository listing shows created repos
- Repo deletion works cleanly

**Edge cases to test:**
- Repository name edge cases (unicode, special chars, length limits)
- Large skill files (multi-MB)
- Binary files in skill repos
- Many files per repo (performance)
- Concurrent clone/push operations
- Repo deletion while clone in progress
- SSH key permission issues
- Disk full during push operations

### 4. Config Management
**User story:** Developer configures Mycelium for their environment
**Critical path:** Edit config → Restart server → Settings applied

**Success criteria:**
- Tilde expansion works (`~/` → `/home/user/`)
- Config validation catches errors early
- Config changes applied on restart
- Layered library configuration works

**Edge cases to test:**
- Malformed YAML
- Invalid port numbers (negative, > 65535, 0)
- Invalid confidence ranges (< 0, > 1.0)
- Missing required fields
- Tilde expansion edge cases (no home dir, different users)
- Relative vs absolute paths
- Config file with wrong permissions
- Config in non-existent directory

### 5. Multi-Client Workflow
**User story:** Multiple developers/agents connect to same server
**Critical path:** Server running → Client A creates repo → Client B clones → Both can push

**Success criteria:**
- Multiple concurrent SSH connections
- SSH key sharing works
- Repository access from multiple clients
- No corruption under concurrent access

**Edge cases to test:**
- Simultaneous repo creation (same name)
- Concurrent push to same repo
- SSH connection limits
- Large number of concurrent clones
- Client disconnections during operations

## Test Pyramid

```
        E2E Tests (30%)
       - Full user journeys
       - Real Soft Serve integration
       - Cross-platform testing
       - Performance under load

        Integration Tests (40%)
       - Component integration
       - Config loading & validation
       - Git operations
       - SSH key management
       - Error handling

      Unit Tests (60%)
     - Pure function testing
     - Validation logic
     - Data structure parsing
     - Edge case handling
     - Mock integrations
```

### Unit Tests (60% of effort)
**Focus:** Pure logic, validation, parsing, data structures
- SKILL.md parsing (all edge cases)
- Config validation (invalid values, missing fields)
- Name validation patterns (kebab-case edge cases)
- Date parsing and formatting
- Tilde expansion logic
- SSH key path construction

### Integration Tests (40% of effort)  
**Focus:** Component interactions, real dependencies
- Config loading from files
- Git server startup/shutdown
- SSH key generation and management
- Repository operations (create/list/delete)
- Process lifecycle management

### E2E Tests (30% of effort)
**Focus:** Complete user workflows, real environment
- Full init → serve → repo lifecycle
- Multi-client scenarios
- Cross-platform compatibility
- Performance and load testing
- Recovery from failures

## Edge Cases & Breaking Scenarios

### The "2 AM Production" Scenarios
*What breaks when nobody's watching?*

1. **Disk Full Scenarios**
   - During SSH key generation
   - During repository creation
   - During config file write
   - During log file writes

2. **Permission Hell**
   - SSH key files with wrong permissions
   - Data directory not writable  
   - Config file not readable
   - Port binding permission denied

3. **Process Management Edge Cases**
   - Soft Serve process crashes unexpectedly
   - Zombie process cleanup
   - Signal handling during startup
   - Multiple SIGTERM signals
   - Process kill during SSH operation

4. **Network Nastiness**
   - Port already in use by another service
   - Network interface goes down during operation
   - SSH connection drops during clone
   - DNS resolution failures
   - Firewall blocks the port

5. **File System Edge Cases**
   - Home directory doesn't exist
   - Symlinks in config paths
   - Network mounted home directories
   - Case-sensitive vs case-insensitive filesystems
   - Very long file paths (PATH_MAX limits)

6. **Concurrent Access Problems**
   - Two clients create same repository name simultaneously
   - Server shutdown during active git operation
   - SSH key rotation during active connections
   - Config reload during repository operation

7. **Data Corruption Scenarios**
   - Truncated config files
   - Corrupted SSH keys
   - Malformed git repositories
   - Disk corruption in data directory

8. **Resource Exhaustion**
   - Too many concurrent SSH connections
   - Very large repository pushes
   - Memory exhaustion during server startup
   - File handle exhaustion

9. **Environment Chaos**
   - Missing environment variables
   - Wrong Go version
   - Different OS versions
   - Container environments
   - Different shell environments

10. **Integration Failures**
    - Soft Serve version compatibility issues
    - Git version compatibility
    - SSH client version differences
    - Terminal environment edge cases

## Security Test Cases

### Credential Scanning
**Critical:** Pre-commit hooks must catch secrets
- AWS keys, API tokens, passwords in SKILL.md files
- SSH private keys accidentally committed  
- Database connection strings
- Various API key formats (GitHub, OpenAI, etc.)
- Base64 encoded credentials

### SSH Key Security
- SSH key file permissions (600 for private, 644 for public)
- Key generation randomness
- Key reuse detection
- SSH key rotation scenarios

### Injection Attack Vectors
- Repository name injection (shell, SQL)
- SKILL.md content with malicious payloads
- Config file injection attacks
- SSH command injection through repo names
- Path traversal in repository operations

### Access Control
- Unauthorized repository access attempts
- SSH key authentication bypass attempts
- Admin privilege escalation
- Repository deletion authorization

## Performance Concerns

### Startup Performance
**Target:** Server starts in < 5 seconds on typical developer machine
- Config loading time
- SSH key generation time
- Soft Serve subprocess startup
- Port binding and readiness check

### Repository Operations at Scale
**Targets:** 
- 1000 repositories: list repos < 1s
- 100MB repository: clone < 30s
- 10 concurrent operations: no timeouts

### Future Concerns (Not Phase 1)
- Embedding search latency (when vector search added)
- Cross-library search performance
- Skill extraction pipeline throughput
- Memory usage with large skill collections

## Unit Test Gap Analysis

*After reviewing existing tests in `*_test.go` files*

### Existing Test Coverage: GOOD
The current unit tests are comprehensive:
- Config validation ✅
- Skill parsing edge cases ✅
- Git server creation ✅
- SSH key management ✅
- Integration test script ✅

### Missing Unit Test Scenarios

#### Config Package (`internal/config/config_test.go`)
**Need to add:**
1. Tilde expansion with non-existent home directory
2. Library validation with negative priorities
3. Config file with BOM (Byte Order Mark)
4. Very large config files (memory usage)
5. Config with circular dependencies
6. Edge case DSN formats
7. Validation of empty library arrays

#### Skill Package (`pkg/skill/skill_test.go`)
**Need to add:**
1. Very large SKILL.md files (memory limits)
2. Skills with thousands of dependencies
3. Unicode edge cases in all fields (emoji, RTL text)
4. Front matter with duplicate keys
5. Body with only whitespace
6. Skills with very long names (255+ chars)
7. Date parsing edge cases (leap years, timezone handling)
8. Markdown edge cases in body (nested code blocks, tables)
9. YAML edge cases (multiline strings, special characters)

#### Git Server Package (`internal/gitserver/server_test.go`)
**Need to add:**
1. SSH key generation with insufficient permissions
2. Port scanning and conflict resolution
3. Process cleanup after kill -9
4. SSH command timeouts
5. Repository operations with unicode names
6. Large repository clone operations
7. Soft Serve binary version compatibility tests

## Test Infrastructure Requirements

### Test Environment Setup
```bash
# Required binaries
- go test (unit tests)
- git (integration tests)  
- soft (integration tests)
- ssh, ssh-keygen (SSH tests)

# Test build tags
//go:build integration  # for tests requiring soft binary
//go:build e2e         # for full end-to-end tests
```

### Continuous Integration Considerations
- Tests that require `soft` binary: skip gracefully if not available
- Tests that require network ports: use dynamic port allocation  
- Tests that write files: use temporary directories
- Tests that start processes: proper cleanup in deferred functions
- Parallel test execution: avoid port conflicts

## Test Execution Plan

### Local Development
```bash
# Fast feedback loop
go test ./...                    # Unit tests (no external deps)

# Integration testing  
go test -tags integration ./...  # Requires soft binary

# Full end-to-end
./scripts/integration-test.sh    # Complete workflow
```

### CI/CD Pipeline
1. **Unit tests** (fast, no external deps)
2. **Integration tests** (with soft binary installed)
3. **E2E tests** (full integration test script)
4. **Cross-platform testing** (Linux, macOS, Windows)

## Success Criteria

### Quality Gates
1. **Unit test coverage:** > 90% on business logic
2. **Integration tests:** All major component interactions covered
3. **E2E tests:** All critical user journeys automated  
4. **Edge case coverage:** All "2 AM scenarios" have tests
5. **Security tests:** All injection vectors covered
6. **Performance tests:** Meet stated performance targets

### Definition of Done for Testing
- [ ] Test strategy approved by team
- [ ] Unit test gaps filled by Otso
- [ ] E2E test framework implemented
- [ ] Security test scenarios automated
- [ ] Performance benchmarks established
- [ ] CI/CD integration complete
- [ ] Documentation updated

---

*Pavel Petrov, Senior QA Engineer*  
*"If it's not tested, it doesn't work. If it's tested badly, it works worse."*