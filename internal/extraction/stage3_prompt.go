package extraction

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func truncateContent(content []byte, maxBytes int) []byte {
	if len(content) <= maxBytes {
		return content
	}
	// Don't cut mid-rune — back off incomplete trailing bytes (at most 3).
	truncated := content[:maxBytes]
	for i := 0; i < 3 && len(truncated) > 0; i++ {
		r, _ := utf8.DecodeLastRune(truncated)
		if r != utf8.RuneError {
			break
		}
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

// generateNonce returns a short random hex string for delimiter uniqueness.
func generateNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sanitizeDelimiters replaces any occurrence of the delimiter markers in content.
func sanitizeDelimiters(content []byte, beginDelim, endDelim string) []byte {
	s := string(content)
	s = strings.ReplaceAll(s, beginDelim, "[SANITIZED_DELIMITER]")
	s = strings.ReplaceAll(s, endDelim, "[SANITIZED_DELIMITER]")
	return []byte(s)
}

func buildCriticPrompt(content []byte, stage2 *model.Stage2Result) string {
	nonce := generateNonce()
	beginDelim := fmt.Sprintf("---BEGIN SESSION %s---", nonce)
	endDelim := fmt.Sprintf("---END SESSION %s---", nonce)

	stage2JSON, err := json.Marshal(stage2)
	if err != nil {
		stage2JSON = []byte("{}")
	}

	sanitized := sanitizeDelimiters(content, beginDelim, endDelim)

	return fmt.Sprintf(`You are a critical evaluator for an AI skill extraction system. Your job is to decide whether this session should be extracted as a reusable skill.

Review the session content and the Stage 2 scoring results, then provide your independent verdict.

Respond with ONLY a JSON object (no markdown, no explanation outside JSON):
{
  "verdict": "extract" or "reject",
  "reasoning": "Your reasoning for the verdict",
  "refined_scores": {
    "problem_specificity": 1-5,
    "solution_completeness": 1-5,
    "context_portability": 1-5,
    "reasoning_transparency": 1-5,
    "technical_accuracy": 1-5,
    "verification_evidence": 1-5,
    "innovation_level": 1-5
  },
  "confidence": 0.0-1.0,
  "reusable_pattern": true/false,
  "explicit_reasoning": true/false,
  "contradicts_best_practices": true/false
}

Stage 2 results:
%s

%s
%s
%s`, string(stage2JSON), beginDelim, string(sanitized), endDelim)
}
