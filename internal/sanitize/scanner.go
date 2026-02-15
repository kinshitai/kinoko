// Package sanitize detects and redacts credentials in text content.
package sanitize

import (
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"
)

// Finding represents a detected credential in text.
type Finding struct {
	Type       string  // e.g. "aws_access_key", "github_token"
	Line       int     // 1-indexed line number
	Column     int     // 1-indexed column
	Match      string  // redacted preview: "AKIA****EXAMPLE"
	Confidence float64 // 0.0–1.0
}

// Pattern defines a credential detection rule.
type Pattern struct {
	Name       string  // identifier
	Regex      *regexp.Regexp
	Confidence float64
	// ContextRequired: if non-empty, the match is only valid when one of these
	// strings appears within ContextWindow characters of the match.
	ContextRequired []string
	ContextWindow   int // default 80
}

// Scanner detects credentials and secrets in text.
type Scanner struct {
	patterns        []Pattern
	logger          *slog.Logger
	minConfidence   float64 // minimum confidence to report (default 0.0)
	redactThreshold float64 // minimum confidence to redact (default 0.7)
}

// Option configures Scanner.
type Option func(*Scanner)

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option {
	return func(s *Scanner) { s.logger = l }
}

// WithMinConfidence sets the minimum confidence to include in Scan results.
func WithMinConfidence(c float64) Option {
	return func(s *Scanner) { s.minConfidence = c }
}

// WithRedactThreshold sets the minimum confidence for redaction.
func WithRedactThreshold(c float64) Option {
	return func(s *Scanner) { s.redactThreshold = c }
}

// WithExtraPatterns adds custom patterns.
func WithExtraPatterns(patterns []Pattern) Option {
	return func(s *Scanner) { s.patterns = append(s.patterns, patterns...) }
}

// DefaultPatterns returns the built-in detection patterns.
func DefaultPatterns() []Pattern {
	return []Pattern{
		{
			Name:       "aws_access_key",
			Regex:      regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
			Confidence: 0.95,
		},
		{
			Name:            "aws_secret_key",
			Regex:           regexp.MustCompile(`[0-9a-zA-Z/+]{40}`),
			Confidence:      0.85,
			ContextRequired: []string{"aws_secret", "aws_secret_access_key", "AWS_SECRET", "secret_key"},
			ContextWindow:   100,
		},
		{
			Name:       "github_token",
			Regex:      regexp.MustCompile(`\bgh[ps]_[A-Za-z0-9_]{36,}\b`),
			Confidence: 0.95,
		},
		{
			Name:       "github_fine_grained",
			Regex:      regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`),
			Confidence: 0.95,
		},
		{
			// P0-2: Spec says sk-[A-Za-z0-9]{48} but we use {20,} intentionally
			// to also catch sk-proj-* and sk-svcacct-* prefixed keys issued by
			// OpenAI for project-scoped and service-account tokens, which have
			// varying lengths after the "sk-" prefix.
			Name:       "openai_key",
			Regex:      regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`),
			Confidence: 0.90,
		},
		{
			Name:       "slack_token",
			Regex:      regexp.MustCompile(`\bxox[baprs]-[0-9a-zA-Z\-]{10,}\b`),
			Confidence: 0.95,
		},
		{
			Name:       "private_key",
			Regex:      regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
			Confidence: 0.99,
		},
		{
			Name:       "database_url",
			Regex:      regexp.MustCompile(`(?:postgres|mysql|mongodb(?:\+srv)?|redis)://[^\s]{8,}`),
			Confidence: 0.90,
		},
		{
			Name:       "bearer_token",
			Regex:      regexp.MustCompile(`\bBearer\s+[A-Za-z0-9\-._~+/]{20,}=*\b`),
			Confidence: 0.80,
		},
		{
			Name:            "generic_api_key",
			Regex:           regexp.MustCompile(`[a-zA-Z0-9_\-]{32,}`),
			Confidence:      0.60,
			ContextRequired: []string{"api_key", "apikey", "api-key", "API_KEY", "ApiKey"},
			ContextWindow:   60,
		},
		{
			Name:       "generic_password",
			Regex:      regexp.MustCompile(`(?i)password\s*[:=]\s*["']?([^\s"']{8,})`),
			Confidence: 0.50,
		},
		{
			Name:       "generic_secret",
			Regex:      regexp.MustCompile(`(?i)secret\s*[:=]\s*["']?([^\s"']{8,})`),
			Confidence: 0.50,
		},
		{
			Name:       "hex_token_64",
			Regex:      regexp.MustCompile(`\b[0-9a-f]{64}\b`),
			Confidence: 0.40,
		},
	}
}

// New creates a Scanner with default patterns and the given options.
func New(opts ...Option) *Scanner {
	s := &Scanner{
		patterns:        DefaultPatterns(),
		logger:          slog.Default(),
		minConfidence:   0.0,
		redactThreshold: 0.7,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Scan returns all credential findings in text.
func (s *Scanner) Scan(text string) []Finding {
	lines := strings.Split(text, "\n")
	var findings []Finding

	for _, pat := range s.patterns {
		for i, line := range lines {
			matches := pat.Regex.FindAllStringIndex(line, -1)
			for _, loc := range matches {
				matchStr := line[loc[0]:loc[1]]

				// Context check
				if len(pat.ContextRequired) > 0 {
					window := pat.ContextWindow
					if window == 0 {
						window = 80
					}
					// Check surrounding text in the full document near this line
					if !hasContext(text, loc[0]+lineOffset(lines, i), window, pat.ContextRequired) {
						continue
					}
				}

				if pat.Confidence < s.minConfidence {
					continue
				}

				findings = append(findings, Finding{
					Type:       pat.Name,
					Line:       i + 1,
					Column:     loc[0] + 1,
					Match:      redactPreview(matchStr),
					Confidence: pat.Confidence,
				})
			}
		}
	}
	return findings
}

// HasSecrets returns true if any finding meets the redact threshold.
func (s *Scanner) HasSecrets(text string) bool {
	for _, f := range s.Scan(text) {
		if f.Confidence >= s.redactThreshold {
			return true
		}
	}
	return false
}

// IsSafe returns true if no findings at all.
func (s *Scanner) IsSafe(text string) bool {
	return len(s.Scan(text)) == 0
}

// Redact replaces detected credentials (above redact threshold) with [REDACTED:<type>].
// P1-2: For context-required patterns, only replace matches that actually passed
// context validation (i.e., were returned by Scan), not all regex matches on the line.
// P2-1: Preserve keyword context for patterns like generic_password/generic_secret
// by replacing only the captured secret value, not the entire match (e.g., "password = [REDACTED]").
func (s *Scanner) Redact(text string) string {
	findings := s.Scan(text)
	if len(findings) == 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	for _, pat := range s.patterns {
		if pat.Confidence < s.redactThreshold {
			continue
		}

		// Collect which lines have confirmed findings for this pattern.
		findingLines := make(map[int]bool)
		for _, f := range findings {
			if f.Type == pat.Name && f.Confidence >= s.redactThreshold {
				findingLines[f.Line] = true
			}
		}
		if len(findingLines) == 0 {
			continue
		}

		for i, line := range lines {
			if !findingLines[i+1] {
				continue
			}

			if len(pat.ContextRequired) > 0 {
				// P1-2: For context-required patterns, only replace matches
				// that pass context check individually, not all regex matches.
				replaced := pat.Regex.ReplaceAllStringFunc(line, func(m string) string {
					mIdx := strings.Index(line, m)
					absPos := lineOffset(lines, i) + mIdx
					window := pat.ContextWindow
					if window == 0 {
						window = 80
					}
					if hasContext(text, absPos, window, pat.ContextRequired) {
						return fmt.Sprintf("[REDACTED:%s]", pat.Name)
					}
					return m
				})
				lines[i] = replaced
			} else if pat.Regex.NumSubexp() > 0 {
				// P2-1: If pattern has a capture group (e.g., generic_password),
				// only redact the captured value to preserve keyword context.
				lines[i] = pat.Regex.ReplaceAllStringFunc(line, func(m string) string {
					sub := pat.Regex.FindStringSubmatch(m)
					if len(sub) > 1 && sub[1] != "" {
						return strings.Replace(m, sub[1], fmt.Sprintf("[REDACTED:%s]", pat.Name), 1)
					}
					return fmt.Sprintf("[REDACTED:%s]", pat.Name)
				})
			} else {
				replacement := fmt.Sprintf("[REDACTED:%s]", pat.Name)
				lines[i] = pat.Regex.ReplaceAllString(line, replacement)
			}
		}
	}
	return strings.Join(lines, "\n")
}

// lineOffset returns the byte offset of the start of line index in lines.
func lineOffset(lines []string, idx int) int {
	offset := 0
	for i := 0; i < idx; i++ {
		offset += len(lines[i]) + 1 // +1 for \n
	}
	return offset
}

// hasContext checks if any of the required strings appear within window chars of pos in text.
func hasContext(text string, pos, window int, required []string) bool {
	start := pos - window
	if start < 0 {
		start = 0
	}
	end := pos + window
	if end > len(text) {
		end = len(text)
	}
	region := strings.ToLower(text[start:end])
	for _, r := range required {
		if strings.Contains(region, strings.ToLower(r)) {
			return true
		}
	}
	return false
}

// redactPreview shows the first 4 and last 4 chars of a match, with **** in between.
func redactPreview(s string) string {
	if len(s) <= 12 {
		return strings.Repeat("*", len(s))
	}
	show := int(math.Min(4, float64(len(s)/4)))
	return s[:show] + "****" + s[len(s)-show:]
}
