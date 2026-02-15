# Team Management Lessons

## 2026-02-15

### Spawning Agents
- Always include their memory folder path in the task: "Read /path/to/memory/{name}/ first"
- Always include "Update your memory files after completing the task"
- Use model `anthropic/claude-sonnet-4-20250514` for sub-agents (good balance)
- Set runTimeoutSeconds to 600-900 for complex tasks

### What Works
- **Single coder + exact spec** beats parallel coders with vague specs
- **Hal does research first**, gives coder exact implementation instructions
- **Jazz after every code change** catches real issues
- **Pavel after major features** exposes edge cases devs miss
- **Luka for unknowns** — cross-field research produces actionable insights

### What Doesn't Work
- Parallel devs without shared interface specs → config mismatch, integration bugs
- Vague "figure it out" tasks → devs build abstractions around TODOs
- Pavel's test assumptions need Jazz to verify (40% false positive rate)

### Workflow
1. Hal researches → writes exact spec
2. Otso builds (with spec + his memory)
3. Jazz reviews code
4. Pavel tests product
5. Jazz reviews Pavel's findings (sorts real bugs from false positives)
6. Otso fixes real bugs + Pavel fixes bad tests
7. Charis documents what shipped
8. Luka explores upcoming unknowns

### Agent Prompts
- Stored in docs/team-prompts.md
- Each agent has a personality that produces better output than generic prompts
- Jazz's grumpiness catches things a polite reviewer wouldn't mention
- Pavel's paranoia finds real edge cases
- Luka's cross-field mandate prevents tunnel vision
