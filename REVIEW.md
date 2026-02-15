# Code Review — Jazz

## Summary
This codebase is fundamentally broken. Two developers clearly worked in isolation without talking to each other, creating incompatible config structures that will crash at runtime. The main feature (git server) is completely unimplemented but pretends to work. I wouldn't ship this to my worst enemy.

## Critical Issues (Must Fix)

### 1. Config Structure Mismatch - SHOWSTOPPER
The config structures in `internal/config/config.go` and `cmd/kinoko/init.go` are completely incompatible:

**config.go defines:**
```go
type Config struct {
    Server    ServerConfig    // Port, DataDir (missing Host)
    Storage   StorageConfig   // Driver, DSN  
    Libraries []LibraryConfig // Name, Path, URL, Priority
}
```

**init.go creates YAML with:**
```yaml
server:
  host: "127.0.0.1"        # NOT in struct!
  port: 23231
  dataDir: ~/.kinoko/data
storage: # ... same
libraries: # ... same
extraction:                # ENTIRE SECTION NOT IN STRUCT!
  auto_extract: true
  min_confidence: 0.5
  require_validation: true
hooks:                     # ENTIRE SECTION NOT IN STRUCT!
  credential_scan: true
  format_validation: true
  llm_critic: false
defaults:                  # ENTIRE SECTION NOT IN STRUCT!
  author: ""
  confidence: 0.7
```

**Result:** Anyone who runs `kinoko init` then `kinoko serve` will get YAML parsing errors because the structs don't match the generated config. This is a runtime crash waiting to happen.

### 2. Serve Command is Completely Fake
The `serve` command loads config, creates directories, then... **does absolutely nothing**. It has a TODO comment about implementing Soft Serve and just waits for Ctrl+C. This is not a "placeholder" - this is shipping broken functionality.

## Major Issues (Should Fix)

### 3. Dependency Management Nightmare
All dependencies in `go.mod` are marked as `// indirect` when they're clearly direct dependencies:
- `cobra` is used directly in cmd files
- `yaml.v3` is used directly in config package  
- `soft-serve` is imported in serve.go

This suggests `go mod tidy` was never run or the imports are wrong.

### 4. Documentation Lies About Implementation
`docs/skill-format.md` promises features that don't exist:
- "Security Validation: credential patterns, prompt injection" - **not implemented**
- "updated defaults to created value if omitted" - **not implemented, stays zero**
- Version backward compatibility - **hardcoded to only accept version 1**

### 5. Root Command References Non-existent Commands
The success message in `init.go` mentions `kinoko remote add <name> <url>` but there's no such command in `root.go`. Users will get "unknown command" errors.

### 6. Race Condition in Signal Handling
`serve.go` has a classic goroutine race:
```go
go func() {
    <-sigCh
    cancel() // This could race with main goroutine
}()
```
Should use `select` on both signal and context channels.

## Minor Issues (Nice to Fix)

### 7. Hardcoded Version String
`root.go` hardcodes `Version: "0.1.0"` instead of using build-time injection with `-ldflags`.

### 8. Inconsistent Error Handling
Some functions return `fmt.Errorf("msg: %w", err)` while others return `fmt.Errorf("msg %s: %w", path, err)`. Pick a pattern and stick to it.

### 9. Skill Validation Too Rigid
The skill parser demands exact section names "## When to Use" and "## Solution". What if someone writes "## When To Use" (capital T)? Fails validation. This will frustrate users.

### 10. No Tilde Expansion
Config paths use `~/.kinoko/` in YAML but there's no tilde expansion. Will try to create literal `~` directory instead of home directory.

### 11. Git Commands Without Validation
`init.go` runs `git` commands without checking if they succeeded. Could fail silently and leave users confused about why their repo isn't set up.

## File-by-File Notes

### cmd/kinoko/main.go
**Grade: B+**
- Clean and minimal
- Proper error handling and logging setup
- Only issue: references undefined `rootCmd` without import (works due to package scope but confusing)

### cmd/kinoko/root.go  
**Grade: C+**
- Basic Cobra setup is fine
- Hardcoded version string is amateur hour
- Missing commands that are referenced elsewhere

### cmd/kinoko/serve.go
**Grade: F (FAIL)**
- **COMPLETELY BROKEN**: Loads config, pretends to start server, does nothing
- Has detailed comments about research but zero implementation
- Will confuse and frustrate every user who tries it
- Signal handling has race condition

### cmd/kinoko/init.go
**Grade: B-**
- Actually functional (unlike serve.go)
- Good error handling and user feedback
- **CRITICAL BUG**: Creates config that can't be parsed by config.go
- References non-existent commands in help text
- No tilde expansion for paths

### internal/config/config.go
**Grade: B**
- Clean struct design and validation
- Good YAML marshaling with proper tags
- Comprehensive validation rules
- **CRITICAL**: Struct doesn't match what init.go creates

### internal/config/config_test.go  
**Grade: A-**
- Thorough test coverage for config loading/validation
- Tests invalid YAML and edge cases
- Good use of table-driven tests
- **PROBLEM**: Only tests the simplified config structure, not the complex one init.go creates

### pkg/skill/skill.go
**Grade: A**
- Most professional code in the repo
- Proper parsing with validation
- Good separation of concerns with skillYAML helper
- Custom date handling is well done
- **MINOR**: Body validation too strict on section names

### pkg/skill/skill_test.go
**Grade: A**  
- Excellent test coverage
- Tests edge cases, round-trip parsing, validation
- Good use of table-driven tests
- Tests both positive and negative cases

### docs/skill-format.md
**Grade: B+**
- Well-written, comprehensive spec
- Good examples and clear structure
- **PROBLEM**: Documents features that aren't implemented
- **INCONSISTENCY**: Says updated defaults to created, but code doesn't do this

### go.mod
**Grade: D-**
- All deps marked indirect when they're direct
- Suggests poor dependency management
- Module path looks professional but deps are wrong

## Consistency Check

**Do the two programmers' code fit together?**

**ABSOLUTELY NOT.** This looks like two different people worked on completely separate projects:

**Programmer A** (config.go, config_test.go, skill.go, skill_test.go):
- Professional Go developer
- Good testing practices
- Clean struct design and validation
- Follows Go idioms

**Programmer B** (init.go, serve.go):  
- More junior/different experience level
- Creates complex config structures not supported by Programmer A's code
- Ships non-functional serve command
- No integration testing with existing code

**Result**: These codebases are fundamentally incompatible. They can't both be correct, and integration was never tested.

## Verdict

**REJECT - DO NOT MERGE**

This PR would break the application for any user who follows the basic workflow:
1. Run `kinoko init` ✓ (works)  
2. Run `kinoko serve` ✗ (crashes on config parsing)

Beyond that showstopper, the main feature (git server) isn't implemented at all. This is pre-alpha quality code masquerading as a release.

**Before this can be merged:**
1. **FIX THE CONFIG MISMATCH** - Either update the struct or change the YAML template
2. **IMPLEMENT THE SERVER** - Don't ship fake functionality  
3. **FIX THE DEPENDENCIES** - Run `go mod tidy` properly
4. **INTEGRATION TESTING** - Test the actual user workflows
5. **ALIGN THE DOCS** - Either implement the documented features or remove them

**Estimated effort to fix:** 2-3 days of solid work

**My recommendation:** Start over with proper coordination between developers. This is what happens when people don't talk to each other.

---

*Jazz - Senior Code Reviewer*
*"I've seen this movie before. It doesn't end well."*