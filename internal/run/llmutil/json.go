// Package llmutil provides utilities for parsing LLM responses.
package llmutil

import (
	"encoding/json"
	"errors"
	"strings"
)

// ExtractJSON attempts to unmarshal a JSON object of type T from an LLM response
// using a 4-strategy cascade:
//  1. Direct JSON parse of the entire response.
//  2. Extract from ```json ... ``` fenced code block.
//  3. Extract from ``` ... ``` generic fenced code block.
//  4. Extract substring from first '{' to last '}'.
func ExtractJSON[T any](resp string) (T, error) {
	var zero T

	if strings.TrimSpace(resp) == "" {
		return zero, errors.New("empty LLM response")
	}

	// Strategy 1: direct parse.
	var result T
	if err := json.Unmarshal([]byte(resp), &result); err == nil {
		return result, nil
	}

	// Strategy 2: ```json ... ``` block.
	if start := strings.Index(resp, "```json"); start >= 0 {
		inner := resp[start+7:]
		if end := strings.Index(inner, "```"); end >= 0 {
			if err := json.Unmarshal([]byte(strings.TrimSpace(inner[:end])), &result); err == nil {
				return result, nil
			}
		}
	}

	// Strategy 3: ``` ... ``` block.
	if start := strings.Index(resp, "```"); start >= 0 {
		inner := resp[start+3:]
		if end := strings.Index(inner, "```"); end >= 0 {
			candidate := strings.TrimSpace(inner[:end])
			if err := json.Unmarshal([]byte(candidate), &result); err == nil {
				return result, nil
			}
		}
	}

	// Strategy 4: first '{' to last '}'.
	first := strings.Index(resp, "{")
	last := strings.LastIndex(resp, "}")
	if first >= 0 && last > first {
		if err := json.Unmarshal([]byte(resp[first:last+1]), &result); err == nil {
			return result, nil
		}
	}

	return zero, errors.New("could not extract valid JSON from LLM response")
}
