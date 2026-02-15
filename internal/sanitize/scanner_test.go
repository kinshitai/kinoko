package sanitize

import (
	"regexp"
	"strings"
	"testing"
)

func TestAWSAccessKey(t *testing.T) {
	s := New()
	// True positive
	findings := s.Scan("my key is AKIAIOSFODNN7EXAMPLE")
	if len(findings) == 0 {
		t.Fatal("expected to find AWS access key")
	}
	if findings[0].Type != "aws_access_key" {
		t.Errorf("expected aws_access_key, got %s", findings[0].Type)
	}

	// True negative — too short
	findings = s.Scan("AKIA1234")
	found := false
	for _, f := range findings {
		if f.Type == "aws_access_key" {
			found = true
		}
	}
	if found {
		t.Error("should not match short AKIA string")
	}
}

func TestGitHubToken(t *testing.T) {
	s := New()
	findings := s.Scan("token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij1234")
	hasGH := false
	for _, f := range findings {
		if f.Type == "github_token" {
			hasGH = true
		}
	}
	if !hasGH {
		t.Fatal("expected github_token finding")
	}
}

func TestGitHubFineGrained(t *testing.T) {
	s := New()
	findings := s.Scan("github_pat_ABCDEFGHIJKLMNOPQRSTUV1234567890ab")
	hasGH := false
	for _, f := range findings {
		if f.Type == "github_fine_grained" {
			hasGH = true
		}
	}
	if !hasGH {
		t.Fatal("expected github_fine_grained finding")
	}
}

func TestOpenAIKey(t *testing.T) {
	s := New()
	findings := s.Scan("OPENAI_API_KEY=sk-aBcDeFgHiJkLmNoPqRsTuVwXyZ012345678901234567")
	hasOAI := false
	for _, f := range findings {
		if f.Type == "openai_key" {
			hasOAI = true
		}
	}
	if !hasOAI {
		t.Fatal("expected openai_key finding")
	}
}

func TestSlackToken(t *testing.T) {
	s := New()
	findings := s.Scan("SLACK_TOKEN=xoxb-1234567890-abcdefghij")
	hasSlack := false
	for _, f := range findings {
		if f.Type == "slack_token" {
			hasSlack = true
		}
	}
	if !hasSlack {
		t.Fatal("expected slack_token finding")
	}
}

func TestPrivateKey(t *testing.T) {
	s := New()
	content := "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----"
	findings := s.Scan(content)
	hasPK := false
	for _, f := range findings {
		if f.Type == "private_key" {
			hasPK = true
		}
	}
	if !hasPK {
		t.Fatal("expected private_key finding")
	}
}

func TestDatabaseURL(t *testing.T) {
	s := New()
	findings := s.Scan("DATABASE_URL=postgres://user:secret@localhost:5432/mydb")
	hasDB := false
	for _, f := range findings {
		if f.Type == "database_url" {
			hasDB = true
		}
	}
	if !hasDB {
		t.Fatal("expected database_url finding")
	}
}

func TestBearerToken(t *testing.T) {
	s := New()
	findings := s.Scan("Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U")
	hasBearer := false
	for _, f := range findings {
		if f.Type == "bearer_token" {
			hasBearer = true
		}
	}
	if !hasBearer {
		t.Fatal("expected bearer_token finding")
	}
}

func TestGenericPassword(t *testing.T) {
	s := New()
	findings := s.Scan("password = mysecretpassword123")
	hasPW := false
	for _, f := range findings {
		if f.Type == "generic_password" {
			hasPW = true
		}
	}
	if !hasPW {
		t.Fatal("expected generic_password finding")
	}
}

func TestGenericSecret(t *testing.T) {
	s := New()
	findings := s.Scan("secret: my_super_secret_value")
	hasSec := false
	for _, f := range findings {
		if f.Type == "generic_secret" {
			hasSec = true
		}
	}
	if !hasSec {
		t.Fatal("expected generic_secret finding")
	}
}

func TestHexToken(t *testing.T) {
	s := New()
	findings := s.Scan("hash: " + strings.Repeat("a1b2c3d4", 8))
	hasHex := false
	for _, f := range findings {
		if f.Type == "hex_token_64" {
			hasHex = true
		}
	}
	if !hasHex {
		t.Fatal("expected hex_token_64 finding")
	}
}

func TestAWSSecretKeyWithContext(t *testing.T) {
	s := New()
	// With context: should match
	findings := s.Scan("aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	hasAWS := false
	for _, f := range findings {
		if f.Type == "aws_secret_key" {
			hasAWS = true
		}
	}
	if !hasAWS {
		t.Fatal("expected aws_secret_key finding with context")
	}

	// Without context: should NOT match
	findings = s.Scan("some random text wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY more text")
	hasAWS = false
	for _, f := range findings {
		if f.Type == "aws_secret_key" {
			hasAWS = true
		}
	}
	if hasAWS {
		t.Error("should not match AWS secret key without context")
	}
}

func TestGenericAPIKeyWithContext(t *testing.T) {
	s := New()
	longKey := strings.Repeat("a", 32)
	// With context
	findings := s.Scan("api_key=" + longKey)
	hasAPIKey := false
	for _, f := range findings {
		if f.Type == "generic_api_key" {
			hasAPIKey = true
		}
	}
	if !hasAPIKey {
		t.Fatal("expected generic_api_key finding")
	}

	// Without context — should not match
	findings = s.Scan("some random " + longKey + " text")
	hasAPIKey = false
	for _, f := range findings {
		if f.Type == "generic_api_key" {
			hasAPIKey = true
		}
	}
	if hasAPIKey {
		t.Error("should not match generic api key without context")
	}
}

// --- False positive tests ---

// P2-3: Renamed from TestNoFalsePositiveExampleKeys — the test validates that
// placeholder/masked values don't trigger high-confidence findings.
func TestNoFalsePositiveOnPlaceholders(t *testing.T) {
	s := New(WithMinConfidence(0.7))
	safes := []string{
		"Use your-api-key-here as the token",               // placeholder
		"password = ********",                               // masked
		"the SHA256 hash of 'hello' is 2cf24dba5fb0a30e...", // truncated hex
	}
	for _, safe := range safes {
		findings := s.Scan(safe)
		for _, f := range findings {
			if f.Confidence >= 0.7 {
				t.Errorf("false positive on %q: %s (%.2f)", safe, f.Type, f.Confidence)
			}
		}
	}
}

// --- Redaction tests ---

func TestRedactPreservesStructure(t *testing.T) {
	s := New()
	content := `# Config
database_url: postgres://admin:s3cr3t@db.example.com:5432/prod
api_version: v2
token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij1234
`
	redacted := s.Redact(content)

	if !strings.Contains(redacted, "# Config") {
		t.Error("structure not preserved")
	}
	if !strings.Contains(redacted, "api_version: v2") {
		t.Error("non-secret lines should be preserved")
	}
	if strings.Contains(redacted, "s3cr3t") {
		t.Error("database password should be redacted")
	}
	if strings.Contains(redacted, "ghp_ABCDEF") {
		t.Error("github token should be redacted")
	}
	if !strings.Contains(redacted, "[REDACTED:") {
		t.Error("should contain REDACTED markers")
	}

	// Line count preserved
	origLines := strings.Count(content, "\n")
	redactedLines := strings.Count(redacted, "\n")
	if origLines != redactedLines {
		t.Errorf("line count changed: %d → %d", origLines, redactedLines)
	}
}

func TestRedactBelowThreshold(t *testing.T) {
	s := New(WithRedactThreshold(0.8))
	// generic_password is 0.50 confidence — should NOT be redacted
	content := "password = mysecretpassword123"
	redacted := s.Redact(content)
	if strings.Contains(redacted, "[REDACTED:") {
		t.Error("low-confidence findings should not be redacted at threshold 0.8")
	}
}

func TestHasSecrets(t *testing.T) {
	s := New()
	if !s.HasSecrets("my token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij1234") {
		t.Error("expected HasSecrets=true for GitHub token")
	}
	if s.HasSecrets("just some normal text without secrets") {
		t.Error("expected HasSecrets=false for clean text")
	}
}

func TestIsSafe(t *testing.T) {
	s := New()
	if s.IsSafe("AKIAIOSFODNN7EXAMPLE") {
		t.Error("expected IsSafe=false for AWS key")
	}
	if !s.IsSafe("hello world") {
		t.Error("expected IsSafe=true for clean text")
	}
}

func TestLineNumber(t *testing.T) {
	s := New()
	content := "line one\nline two\nAKIAIOSFODNN7EXAMPLE\nline four"
	findings := s.Scan(content)
	for _, f := range findings {
		if f.Type == "aws_access_key" {
			if f.Line != 3 {
				t.Errorf("expected line 3, got %d", f.Line)
			}
		}
	}
}

func TestRedactPreview(t *testing.T) {
	preview := redactPreview("AKIAIOSFODNN7EXAMPLE")
	if !strings.Contains(preview, "****") {
		t.Errorf("expected masked preview, got %q", preview)
	}
	if strings.Contains(preview, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("preview should not contain full match")
	}
}

func TestCustomPatterns(t *testing.T) {
	custom := Pattern{
		Name:       "custom_token",
		Regex:      compileRegex(`CUSTOM-[A-Z]{10}`),
		Confidence: 0.95,
	}
	s := New(WithExtraPatterns([]Pattern{custom}))
	findings := s.Scan("token: CUSTOM-ABCDEFGHIJ")
	hasCustom := false
	for _, f := range findings {
		if f.Type == "custom_token" {
			hasCustom = true
		}
	}
	if !hasCustom {
		t.Fatal("expected custom_token finding")
	}
}

// P1-5: Multiline credential tests.
func TestPrivateKeyBlock(t *testing.T) {
	s := New()
	content := `Some text before
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF0PbnGcY5unA67hFdBBCRnHiQ+kd
k3MpF4yGEKBFaGPBe2DXFQ6L2YJRrP7Kq9ZYAT9L9vd4XLK9L1W5NMQ3RL8FZJH
-----END RSA PRIVATE KEY-----
Some text after`
	findings := s.Scan(content)
	hasPK := false
	for _, f := range findings {
		if f.Type == "private_key" {
			hasPK = true
			if f.Line != 2 {
				t.Errorf("expected line 2, got %d", f.Line)
			}
		}
	}
	if !hasPK {
		t.Fatal("expected private_key finding in multiline block")
	}
}

func TestBase64EncodedSecret(t *testing.T) {
	s := New()
	// A base64-encoded secret in a config-like context
	content := `api_key=c2VjcmV0X2tleV90aGF0X2lzX3ZlcnlfbG9uZ19hbmRfaGFyZF90b19ndWVzcw==`
	findings := s.Scan(content)
	hasKey := false
	for _, f := range findings {
		if f.Type == "generic_api_key" {
			hasKey = true
		}
	}
	if !hasKey {
		t.Fatal("expected generic_api_key finding for base64 secret with context")
	}
}

func TestCredentialInCodeBlock(t *testing.T) {
	s := New()
	content := "```\npassword = SuperSecret123!\n```"
	findings := s.Scan(content)
	hasPW := false
	for _, f := range findings {
		if f.Type == "generic_password" {
			hasPW = true
		}
	}
	if !hasPW {
		t.Fatal("expected generic_password finding in code block")
	}
}

func TestECPrivateKey(t *testing.T) {
	s := New()
	content := "-----BEGIN EC PRIVATE KEY-----\nMHQCAQEE..."
	findings := s.Scan(content)
	hasPK := false
	for _, f := range findings {
		if f.Type == "private_key" {
			hasPK = true
		}
	}
	if !hasPK {
		t.Fatal("expected private_key finding for EC key")
	}
}

func compileRegex(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }

// Ensure regexp import is used.
var _ = regexp.Compile
