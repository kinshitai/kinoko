# Consolidated Fix Plan

## Pavel's Test Code Issues
[Issues in the test code itself that need fixing before tests are reliable]

### Major Test Issues

1. **Port availability checking uses external dependency**
   - **File:** `tests/e2e/setup_test.go` 
   - **Issue:** Uses `nc` command for port checking, fails if netcat not installed
   - **Fix:** Replace with Go's `net.Listen()` for pure-Go port testing

2. **Incorrect test assumptions about git init behavior**
   - **File:** `tests/e2e/init_test.go`
   - **Issue:** `TestMyceliumInitNoGit` expects different behavior than documented
   - **Fix:** Test should verify graceful degradation (no git repo created) not failure

3. **Wrong dependency name validation expectations**
   - **File:** `tests/e2e/skill_test.go`
   - **Issue:** `TestSkillParsingEdgeCases/edge_case_front_matter` expects underscores to be valid
   - **Fix:** Test should expect underscores to be REJECTED (kebab-case spec)

4. **Test strategy math doesn't add up**
   - **File:** `docs/test-strategy.md`
   - **Issue:** Claims 60% unit + 40% integration + 30% e2e = 130%
   - **Fix:** Use realistic percentages that sum to 100%

### Minor Test Issues

5. **Large skill test might hit scanner buffer limits**
   - **File:** `tests/e2e/skill_test.go`
   - **Issue:** May fail due to `bufio.Scanner` 64KB default limit, not actual truncation
   - **Fix:** Either test smaller content or handle the buffer limit properly

## Real Bugs Found by Tests
[Actual product bugs exposed by Pavel's tests — what's broken, where, and how to fix]

### Critical Bug

1. **Tilde expansion doesn't handle `~user/path` syntax**
   - **File:** `internal/config/config.go`
   - **Function:** `expandPath()`
   - **Bug:** Only handles `~/path`, fails on `~username/path`
   - **Fix:** Parse user portion before tilde, lookup user's home directory
   - **Impact:** Config paths like `~otso/.mycelium/skills` won't expand correctly

### Potential Bug

2. **Large skill files may hit buffer limits**
   - **File:** `pkg/skill/skill.go`
   - **Function:** `parseContent()`
   - **Bug:** `bufio.Scanner` has 64KB default buffer limit
   - **Fix:** Call `scanner.Buffer()` to set larger buffer (e.g., 1MB)
   - **Impact:** Skills with large code examples or documentation may get truncated

## Test Assumptions That Are Wrong
[Cases where Pavel's test expects wrong behavior — the product is correct, the test is wrong]

1. **Skill names with underscores should be rejected**
   - **Pavel expects:** `skill-with-underscores_here` to be valid
   - **Reality:** Spec requires kebab-case (hyphens only), underscores are correctly rejected
   - **Fix:** Update test to expect validation error

2. **Missing git should not cause init to fail**
   - **Pavel expects:** Init command to fail when git is missing
   - **Reality:** Init gracefully skips git repo creation with warning (correct behavior)
   - **Fix:** Update test to verify warning message and missing `.git` directory

3. **Some of Pavel's "invalid" configurations are actually valid**
   - **Example:** Empty library arrays are perfectly valid
   - **Fix:** Review validation expectations in config tests

## Priority Fix Order
[Numbered list: what Otso should fix first, second, etc. Include both product fixes and test fixes.]

### High Priority (Fix Immediately)
1. **Fix tilde expansion bug** - Critical for multi-user environments
2. **Fix port availability checking in tests** - Makes tests fragile
3. **Fix skill buffer limits** - Could cause data loss with large skills

### Medium Priority (Fix Soon)  
4. **Correct test assumptions about dependency validation** - Tests are failing incorrectly
5. **Fix init test expectations** - Tests should match documented behavior
6. **Update test strategy math** - Professional documentation standards

### Low Priority (Nice to Have)
7. **Add more comprehensive tilde expansion tests** - Pavel's suggestions here are good
8. **Add SSH key generation failure tests** - Good defensive testing
9. **Add concurrent operations stress tests** - Pavel's framework supports this well

## Unit Test Requests Assessment
[Which of Pavel's requests for Otso are worth doing, which are overkill, which are wrong]

### ✅ Worth Implementing (Priority Order)

**High Value:**
1. **Tilde expansion edge cases** - Found real bug, need comprehensive coverage
2. **Large skill parsing tests** - May hit real buffer limits
3. **SSH key generation failure scenarios** - Good defensive testing
4. **Date parsing edge cases** - Leap years and boundary conditions matter
5. **Configuration partial merging** - Real user scenario

**Medium Value:**
6. **Unicode handling edge cases** - Good for i18n robustness  
7. **Concurrent repository operations** - Stress testing is valuable
8. **Config DSN validation formats** - User input validation

### ❌ Skip These (Overkill or Wrong)

**Academic Overkill:**
- "Test skill with thousands of dependencies" - Nobody will hit this
- "Test YAML multiline strings in front matter" - Spec doesn't allow this anyway
- "Test nested YAML structures" - Already handled by YAML library
- "Very long repository names" - Soft Serve handles this
- "SSH command with megabytes of output" - Not a realistic scenario

**Wrong Assumptions:**
- Any tests expecting underscores in skill names to be valid
- Tests expecting certain "invalid" DSN formats to pass
- "Test case sensitivity of required sections" - Spec is case-insensitive already

**Already Covered:**
- "Test confidence boundary values" - Trivial range checks
- "Test empty config arrays" - Already working correctly

### 📊 Unit Test Request Summary
- **Total requests:** ~40
- **Worth implementing:** ~15 (38%)  
- **Overkill/unnecessary:** ~20 (50%)
- **Based on wrong assumptions:** ~5 (12%)

---

## Overall Assessment

**Pavel's Test Code Quality:** B+ (solid framework, some bugs in assumptions)

**Bug Finding Effectiveness:** B- (1.5 real bugs found, 2.5 false positives)

**Testing Strategy:** B (comprehensive but some fluff and math errors)

**Value Add to Project:** B+ (the test infrastructure alone is worth keeping)

**Recommendation:** Fix the real bugs Pavel found, correct his test assumptions, ignore half his unit test requests, and promote this kid - he's got potential once he learns to distinguish product bugs from test bugs.

---

*Jazz - Senior Code Reviewer*  
*"Pavel's not bad for a junior. He found one real bug and built decent test infrastructure. Just needs to learn when to stop testing."*