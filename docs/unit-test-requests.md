# Unit Test Requests for Otso

Pavel here. I've analyzed the existing unit tests and found good coverage overall, but there are specific edge cases and scenarios that need additional testing. Here are my requests for additional unit tests, organized by file.

## `internal/config/config_test.go`

### Tilde Expansion Edge Cases
**Function:** `expandPath()` and config loading with tilde paths
**Missing test scenarios:**

1. **Test tilde expansion with non-existent user:**
   ```go
   func TestTildeExpansionNonExistentUser(t *testing.T) {
       // Test path like "~nonexistentuser/path"
       // Should return path unchanged or handle gracefully
   }
   ```

2. **Test tilde expansion with current user lookup failure:**
   ```go
   func TestTildeExpansionUserLookupFailure(t *testing.T) {
       // Mock user.Current() to return error
       // Should fall back to os.UserHomeDir()
   }
   ```

3. **Test paths with tildes in middle:**
   ```go
   func TestTildeInMiddleOfPath(t *testing.T) {
       // Test paths like "/some/path/~/subdir"
       // Should not expand tildes that aren't at start
   }
   ```

### Library Configuration Edge Cases
**Function:** `Validate()` - library validation
**Missing test scenarios:**

1. **Test library with negative priority:**
   ```go
   func TestLibraryNegativePriority(t *testing.T) {
       // Should fail validation with negative priority
       libraries := []LibraryConfig{{Name: "test", Path: "/path", Priority: -1}}
   }
   ```

2. **Test empty library array:**
   ```go
   func TestEmptyLibraryArray(t *testing.T) {
       // Should be valid to have no libraries
       cfg.Libraries = []LibraryConfig{}
   }
   ```

3. **Test duplicate library names:**
   ```go
   func TestDuplicateLibraryNames(t *testing.T) {
       // Should fail validation with duplicate names
       libraries := []LibraryConfig{
           {Name: "same", Path: "/path1", Priority: 100},
           {Name: "same", Path: "/path2", Priority: 50},
       }
   }
   ```

### Configuration Merging and Defaults
**Function:** `Load()` - default configuration merging
**Missing test scenarios:**

1. **Test partial config file (missing sections):**
   ```go
   func TestPartialConfigFile(t *testing.T) {
       // Config file with only server section
       // Should merge with defaults for other sections
   }
   ```

2. **Test config with extra unknown fields:**
   ```go
   func TestConfigWithExtraFields(t *testing.T) {
       // YAML with unknown fields should be ignored, not error
   }
   ```

### DSN Validation Edge Cases  
**Function:** `Validate()` - storage DSN validation
**Missing test scenarios:**

1. **Test various DSN formats:**
   ```go
   func TestDSNFormats(t *testing.T) {
       // Test postgres://, sqlite://, file:// formats
       // Test relative paths, absolute paths
   }
   ```

## `pkg/skill/skill_test.go`

### Advanced Parsing Edge Cases
**Function:** `Parse()` and `parseContent()`
**Missing test scenarios:**

1. **Test very large skills (memory limits):**
   ```go
   func TestVeryLargeSkill(t *testing.T) {
       // Test skill with 10MB+ body content
       // Ensure parser doesn't exhaust memory
       largeContent := strings.Repeat("content ", 1000000)
   }
   ```

2. **Test skills with thousands of dependencies:**
   ```go
   func TestManyDependencies(t *testing.T) {
       // Test skill with 1000+ dependencies
       // Ensure parser handles large arrays
   }
   ```

3. **Test front matter with duplicate keys:**
   ```go
   func TestFrontMatterDuplicateKeys(t *testing.T) {
       // YAML with duplicate "name:" fields
       // Should use last value or error appropriately
   }
   ```

4. **Test nested YAML structures (invalid):**
   ```go
   func TestNestedYAMLStructures(t *testing.T) {
       // Front matter with nested objects (not allowed in spec)
       // Should fail validation cleanly
   }
   ```

### Date and Time Edge Cases
**Function:** Date parsing in `parseContent()`
**Missing test scenarios:**

1. **Test leap year edge cases:**
   ```go
   func TestLeapYearDates(t *testing.T) {
       tests := []struct{date string; valid bool}{
           {"2024-02-29", true},  // Valid leap year
           {"2023-02-29", false}, // Invalid leap year
           {"2000-02-29", true},  // Century leap year
           {"1900-02-29", false}, // Century non-leap year
       }
   }
   ```

2. **Test date boundary conditions:**
   ```go
   func TestDateBoundaryConditions(t *testing.T) {
       // Test dates like "2024-13-01" (invalid month)
       // Test "2024-02-30" (invalid day for month)
       // Test future dates (year 9999)
   }
   ```

3. **Test timezone handling in dates:**
   ```go
   func TestTimeZoneInDates(t *testing.T) {
       // Dates with timezone info should be rejected
       // Format should be strictly YYYY-MM-DD
   }
   ```

### Body Content Validation Edge Cases
**Function:** `validateBody()`
**Missing test scenarios:**

1. **Test body with only whitespace:**
   ```go
   func TestBodyOnlyWhitespace(t *testing.T) {
       // Body with spaces, tabs, newlines but no content
       body := "   \n\t  \n   \n\t\t  "
   }
   ```

2. **Test case sensitivity of required sections:**
   ```go
   func TestSectionCaseSensitivity(t *testing.T) {
       // Test "## WHEN TO USE" vs "## when to use" vs "## When To Use"
       // All should be accepted (case-insensitive)
   }
   ```

3. **Test sections with extra formatting:**
   ```go
   func TestSectionsWithFormatting(t *testing.T) {
       // Test "## **When to Use**" or "## _Solution_"
       // Should still be recognized as valid sections
   }
   ```

4. **Test title sections with different levels:**
   ```go
   func TestTitleSectionLevels(t *testing.T) {
       // Test "## Title" vs "# Title" vs "### Title"
       // Only "# Title" should be accepted as title
   }
   ```

### Name Validation Edge Cases
**Function:** `Validate()` - name pattern validation  
**Missing test scenarios:**

1. **Test edge cases of kebab-case validation:**
   ```go
   func TestKebabCaseEdgeCases(t *testing.T) {
       tests := []struct{name string; valid bool}{
           {"skill-123-test", true},
           {"123-skill", true},
           {"skill-", false},      // Trailing dash
           {"-skill", false},      // Leading dash
           {"skill--test", false}, // Double dash
           {"", false},            // Empty
           {"a", true},            // Single char
           {strings.Repeat("a", 300), false}, // Very long name
       }
   }
   ```

### Confidence Range Edge Cases
**Function:** `Validate()` - confidence validation
**Missing test scenarios:**

1. **Test confidence boundary values:**
   ```go
   func TestConfidenceBoundaryValues(t *testing.T) {
       tests := []struct{confidence float64; valid bool}{
           {0.0, true},     // Minimum valid
           {1.0, true},     // Maximum valid
           {-0.0, true},    // Negative zero (should be valid)
           {1.0000001, false}, // Just over limit
           {-0.0000001, false}, // Just under limit
           {math.NaN(), false}, // Not a number
           {math.Inf(1), false}, // Positive infinity
           {math.Inf(-1), false}, // Negative infinity
       }
   }
   ```

### YAML Parsing Edge Cases
**Function:** YAML unmarshaling in `parseContent()`
**Missing test scenarios:**

1. **Test YAML with special characters:**
   ```go
   func TestYAMLSpecialCharacters(t *testing.T) {
       // Test author names with quotes, colons, etc.
       // Test tags with special YAML characters
   }
   ```

2. **Test YAML multiline strings:**
   ```go
   func TestYAMLMultilineStrings(t *testing.T) {
       // Front matter with | or > multiline strings
       // Should be rejected (not in spec)
   }
   ```

## `internal/gitserver/server_test.go`

### SSH Key Generation Edge Cases
**Function:** `ensureAdminKeys()` 
**Missing test scenarios:**

1. **Test SSH key generation with insufficient disk space:**
   ```go
   func TestSSHKeyGenerationDiskFull(t *testing.T) {
       // Mock filesystem with no space
       // Should handle error gracefully
   }
   ```

2. **Test SSH key with wrong permissions:**
   ```go
   func TestSSHKeyWrongPermissions(t *testing.T) {
       // Pre-create keys with wrong permissions
       // Should correct permissions or regenerate
   }
   ```

3. **Test SSH key generation without ssh-keygen:**
   ```go
   func TestSSHKeyGenerationNoSshKeygen(t *testing.T) {
       // Mock environment without ssh-keygen binary
       // Should handle gracefully
   }
   ```

### Server Process Management Edge Cases
**Function:** `Start()` and `Stop()`
**Missing test scenarios:**

1. **Test server startup timeout:**
   ```go
   func TestServerStartupTimeout(t *testing.T) {
       // Mock Soft Serve that never becomes ready
       // Should timeout and clean up
   }
   ```

2. **Test server process crashes immediately:**
   ```go
   func TestServerProcessCrashesImmediately(t *testing.T) {
       // Mock Soft Serve that exits immediately
       // Should detect and report error
   }
   ```

3. **Test multiple SIGTERM signals:**
   ```go  
   func TestMultipleSIGTERM(t *testing.T) {
       // Send multiple SIGTERM signals during shutdown
       // Should handle gracefully without panic
   }
   ```

### SSH Command Execution Edge Cases
**Function:** `runSSHCommand()`
**Missing test scenarios:**

1. **Test SSH command with very long output:**
   ```go
   func TestSSHCommandLongOutput(t *testing.T) {
       // Command that produces megabytes of output
       // Should handle without memory issues
   }
   ```

2. **Test SSH command timeout:**
   ```go
   func TestSSHCommandTimeout(t *testing.T) {
       // Command that hangs indefinitely  
       // Should timeout and return error
   }
   ```

3. **Test SSH command with binary output:**
   ```go
   func TestSSHCommandBinaryOutput(t *testing.T) {
       // Command that produces binary data
       // Should handle without corrupting output
   }
   ```

### Repository Name Validation Edge Cases
**Function:** `CreateRepo()`, `DeleteRepo()`
**Missing test scenarios:**

1. **Test repository names with Unicode:**
   ```go
   func TestRepositoryNamesUnicode(t *testing.T) {
       // Test repo names with non-ASCII characters
       // Document behavior (should likely fail)
   }
   ```

2. **Test very long repository names:**
   ```go
   func TestVeryLongRepositoryNames(t *testing.T) {
       // Test names approaching filesystem limits
       // Should handle gracefully
   }
   ```

## Testing Strategy Notes

### Build Tags Usage
All these tests should use appropriate build tags:
- Unit tests: No build tags (run always)  
- Integration tests needing external binaries: `//go:build integration`

### Error Message Validation
For all error cases, validate that error messages are:
1. **Specific:** Mention what exactly failed
2. **Actionable:** Give user guidance on how to fix
3. **Consistent:** Follow same format patterns

### Mock Usage Guidelines  
When mocking external dependencies:
1. **Mock at interface boundaries:** Don't mock internal functions
2. **Test error paths:** Mock failures, not just success
3. **Validate call patterns:** Ensure mocks are called as expected

### Test Data Management
For tests requiring files or data:
1. **Use t.TempDir():** Always use temporary directories  
2. **Clean up:** Use defer or t.Cleanup()
3. **Deterministic:** Tests should be reproducible

---

*Pavel Petrov, Senior QA Engineer*  
*"These test scenarios come from 15 years of seeing production systems break in creative ways. Every edge case here is based on real bugs I've encountered."*