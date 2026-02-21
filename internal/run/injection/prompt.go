package injection

import (
	"fmt"
	"strings"
)

const maxPromptBytes = 32 * 1024 // 32KB max total injected content

const promptHeader = `## Relevant Knowledge (auto-injected by Kinoko)

The following skills may be relevant to your current task. Use them if applicable.

`

// BuildInjectionPrompt formats matched skills as a markdown prompt section.
// Returns empty string if no skills are provided.
// Truncates total output to 32KB.
func BuildInjectionPrompt(skills []MatchedSkill) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(promptHeader)

	for _, s := range skills {
		section := fmt.Sprintf("### %s (relevance: %.2f)\n\n%s\n\n", s.Name, s.Score, s.Content)

		if b.Len()+len(section) > maxPromptBytes {
			remaining := maxPromptBytes - b.Len()
			if remaining > 0 {
				b.WriteString(section[:remaining])
			}
			break
		}
		b.WriteString(section)
	}

	return b.String()
}
