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

// buildCombinedPrompt builds the Phase B combined critic + extraction prompt.
// It evaluates the session AND generates SKILL.md in one LLM call.
func buildCombinedPrompt(content []byte, stage2 *model.Stage2Result) string {
	nonce := generateNonce()
	beginDelim := fmt.Sprintf("---BEGIN SESSION %s---", nonce)
	endDelim := fmt.Sprintf("---END SESSION %s---", nonce)

	stage2JSON, err := json.Marshal(stage2)
	if err != nil {
		stage2JSON = []byte("{}")
	}

	sanitized := sanitizeDelimiters(content, beginDelim, endDelim)

	return fmt.Sprintf(`IMPORTANT: You are extracting skills for a DIFFERENT agent working on a DIFFERENT project. That agent has never heard of this project, its codebase, its architecture decisions, or its team.

Before marking something as a skill, ask: "Would this help someone who has never seen this project and never will?" If the answer requires knowing this project's architecture, it's documentation, not a skill.

You are evaluating a session for reusable knowledge extraction.

STEP 1: EVALUATE
Score these dimensions (1-5):
1. Problem Specificity — Clearly defined, concrete problem?
2. Solution Completeness — Can someone follow this start to finish?
3. Context Portability — Apply the SUBSTITUTION TEST:
   Replace all project-specific names with generic placeholders.
   Does the remaining skill still contain non-obvious, actionable knowledge?
   Score 1-2 if: reduces to common sense after substitution.
   Score 3 if: some actionable insight remains but thin.
   Score 4-5 if: core technique/pattern intact and non-trivial.
   REJECT if <3 regardless of other scores.
4. Reasoning Transparency — Is the WHY explained, not just the WHAT?
5. Technical Accuracy — Correct and verified?
6. Verification Evidence — Proven to work?
7. Innovation Level — Novel or standard knowledge?

HARD REJECT if ANY:
- 3 or more proper nouns specific to the source project are essential to the solution
- Removing project context reduces skill to less than 2 sentences of non-obvious advice
- Skill primarily documents WHAT was built rather than reusable HOW/WHY
- A competent developer in the domain would say "that's just how you'd do it"

STEP 2: If verdict is "extract", generate SKILL.md content.

The SKILL.md must follow this format:
---
name: <kebab-case-name>
version: 1
category: <BUILD|FIX|OPTIMIZE|DEBUG|DESIGN|LEARN>
tags:
  - <pattern/path>
---

# <Title>

## Problem
<What specific problem does this solve?>

## Solution
<Step-by-step approach. Code snippets where helpful. Concrete, actionable.>

## Why It Works
<The reasoning/insight behind the approach.>

## Pitfalls
<What to watch out for. Common mistakes.>

## References
<Links, docs, tools mentioned.>

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
  "contradicts_best_practices": true/false,
  "skill_md": "..." // only if verdict=="extract", full SKILL.md content (markdown with YAML front matter)
}

Stage 2 results:
%s

%s
%s
%s`, string(stage2JSON), beginDelim, string(sanitized), endDelim)
}

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

// buildCriticPromptLegacy is the original critic-only prompt (kept for reference).
func buildCriticPromptLegacy(content []byte, stage2 *model.Stage2Result) string {
	return buildCriticPromptImpl(content, stage2)
}

func buildCriticPromptImpl(content []byte, stage2 *model.Stage2Result) string {
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
