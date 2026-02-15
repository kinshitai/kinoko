# Lessons Learned

## 2026-02-15
- SKILL.md spec requires kebab-case names — underscores are INVALID. Don't assume otherwise.
- `mycelium init` gracefully degrades when git is missing — it doesn't fail. Test for warning + no .git dir, not exit code failure.
- Don't use external tools (nc/netcat) in Go tests — use net.Listen() for port checking.
- Test strategy percentages should sum to 100%, not 130%. Embarrassing.
- bufio.Scanner has 64KB default buffer — large skill tests may fail due to scanner limit, not product truncation. Real bug in product, but test needs to be robust about what it's actually testing.
- Jazz approved ~40% of my unit test requests. The "thousands of dependencies" and "YAML multiline strings" tests were overkill. Focus on realistic scenarios.
