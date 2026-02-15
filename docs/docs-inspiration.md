# Docs Inspiration — For Charis

Collected by Hal. Great dev docs to study for patterns and ideas.

## Gold Standard Dev Docs (2024-2026)

### Astro (docs.astro.build)
- Starlight-powered, beautiful, fast
- Great progressive disclosure: "Getting Started" → tutorials → guides → reference
- Interactive examples inline
- Community contribution model

### Stripe (stripe.com/docs)
- The GOAT of API docs
- Code examples in every language, switchable inline
- "Try it" buttons that actually work
- Error codes with explanations and fixes

### Supabase (supabase.com/docs)
- Egor's company — we know these well
- Clean sidebar nav, good search
- Quickstarts per framework
- AI-assisted docs search (early adopter)

### Tailwind CSS (tailwindcss.com/docs)
- Extremely scannable
- Every utility documented with live preview
- Search-first design

### Charm (charm.sh)
- Soft Serve's parent org — relevant
- Beautiful README-first approach
- GIFs showing the product in action
- Minimal but effective

## Novel Patterns Worth Exploring (2026)

### Docs-as-agent-context
- Docs structured so LLMs can parse them effectively
- llms.txt convention — a machine-readable summary of your docs
- Progressive disclosure for context windows: summary → detail → full reference

### Living docs
- Docs that test themselves (code examples that run in CI)
- Docs generated from test output
- Version-pinned examples that update automatically

### Dual-audience docs
- Same content, human-readable AND machine-parseable
- Structured front matter on every doc page (like SKILL.md)
- Semantic sections that agents can target

### Interactive CLI docs
- `mycelium help` as the primary docs surface
- `mycelium doctor` for debugging setup issues
- Rich terminal output with examples

## Questions for Charis
- What does the 2026 docs stack look like? (Starlight? Mintlify? Fumadocs? Something else?)
- How do we make docs work for both humans browsing AND agents consuming?
- Should the README be the primary doc, or should we have a separate docs site?
- How do we handle docs-as-code with our small team?
- What's the minimum viable docs for Phase 1?
