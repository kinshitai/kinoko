# Documentation Architecture for Kinoko

*Charis — February 15, 2026*

---

## Executive Summary

Kinoko is not just another developer tool. It's a knowledge system built by AI agents *for* AI agents, where humans are the ultimate beneficiaries. The documentation needs to reflect this duality: it must serve human developers discovering and debugging Kinoko, AND it must be parseable by the very AI agents that will consume and contribute to the knowledge base.

**My recommendation:** Build a dual-audience, AI-native documentation system using Starlight (Astro) with custom enhancements for machine readability, backed by docs-as-code in the main repository. Ship with a strong README-first approach for Phase 1, then evolve into a comprehensive docs site.

---

## 1. Audit Summary

### What We Have ✓

**Strong foundation documents:**
- **MANIFESTO.md** — Beautifully written, clear mission statement
- **README.md** — Concise but effective front door
- **RFCs** — Well-structured, living documents that capture vision and architecture
- **skill-format.md** — Comprehensive spec that doubles as both human reference and machine validation schema

**What's working:**
- The manifesto is compelling and accessible — it answers "why should I care?" immediately
- The skill format spec is thorough with excellent examples
- The RFC structure provides transparency into decision-making
- Front matter + markdown pattern is already AI-friendly

### What's Missing ✗

**For Day-One Developers:**
- No quickstart guide ("how do I try this in 5 minutes?")
- No installation/setup documentation
- No troubleshooting guide
- No examples of the full workflow (extract → store → inject)
- No API documentation for programmatic integration

**For AI Agents:**
- No llms.txt or structured summary for context ingestion
- No machine-readable metadata about available endpoints/hooks
- No standardized schema documentation beyond the skill format

**For Contributors:**
- No contribution guide
- No development setup instructions
- No testing documentation
- No deployment guide

**For Operators:**
- No self-hosting guide
- No configuration reference
- No monitoring/observability documentation

---

## 2. Documentation Philosophy

### Core Principles

**1. Documentation is Product**
Every doc page is a product experience. If it's confusing, broken, or outdated, we've failed a user at their moment of need.

**2. Dual-Audience Design**
Every significant piece of content serves both humans (browsing, learning, debugging) and AI agents (parsing, understanding, integrating). Structure and metadata matter as much as prose.

**3. Progressive Disclosure**
Show the simplest thing that works, then reveal complexity on demand. The 2 AM developer who just wants to get unstuck should find their answer in the first paragraph.

**4. Five-Minute Rule**
If getting started takes more than 5 minutes from README to working example, the docs failed. This is non-negotiable for developer tools.

**5. Docs-as-Code, Always**
Docs live in the repo, version with the code, and are reviewed like code. No external wikis or separate systems. Documentation updates are part of feature development.

**6. Test Your Examples**
Every code example that appears in docs must run in CI. Broken examples are worse than no examples.

---

## 3. Target Audiences

### Primary Audiences

**1. Discovery Developers** (30% of traffic)
- Finding Kinoko for the first time
- Need: What is this, why should I care, how do I try it
- Format: README, overview, quickstart

**2. Implementation Developers** (40% of traffic)  
- Building with/on Kinoko
- Need: Setup guides, API docs, troubleshooting
- Format: Guides, reference, examples

**3. AI Agents** (20% of traffic)
- Consuming Kinoko as a service or integration
- Need: Structured schemas, machine-readable metadata
- Format: OpenAPI specs, llms.txt, structured front matter

**4. Contributors/Operators** (10% of traffic)
- Contributing skills, hosting instances, customizing
- Need: Development setup, architecture deep-dive, deployment guides
- Format: Technical reference, RFCs, contribution guides

### Secondary Audiences

**Support/Community:** People answering questions need findable, linkable answers
**Search Engines:** Both human search and AI model training need discoverable, well-structured content

---

## 4. Information Architecture

```
kinoko/
├── README.md                    # Front door - what/why/quickstart
├── docs/
│   ├── index.md                 # Docs home (if we build a site)
│   │
│   ├── getting-started/
│   │   ├── quickstart.md        # 5-minute demo
│   │   ├── installation.md      # All platforms
│   │   ├── first-skill.md       # End-to-end example
│   │   └── concepts.md          # Core mental models
│   │
│   ├── guides/
│   │   ├── self-hosting.md      # Full deployment
│   │   ├── client-setup.md      # Hook integration
│   │   ├── skill-creation.md    # Manual skill authoring
│   │   ├── troubleshooting.md   # Common issues
│   │   └── best-practices.md    # Patterns & anti-patterns
│   │
│   ├── reference/
│   │   ├── api/                 # Auto-generated from code
│   │   ├── skill-format.md      # Move here from current location
│   │   ├── config-reference.md  # All config options
│   │   ├── cli-reference.md     # Generated from --help
│   │   └── hooks-reference.md   # Integration points
│   │
│   ├── architecture/
│   │   ├── overview.md          # System design
│   │   ├── data-flow.md         # How knowledge moves
│   │   ├── security.md          # Trust & privacy model
│   │   └── roadmap.md           # Future plans
│   │
│   ├── contributing/
│   │   ├── development.md       # Dev environment setup
│   │   ├── testing.md           # Running & writing tests
│   │   ├── skills.md            # Contributing to skill libraries
│   │   └── docs.md              # Contributing to docs
│   │
│   └── ai/
│       ├── llms.txt             # Machine-readable summary
│       ├── schema/              # JSON schemas for all formats
│       └── integration.md       # For other AI systems
│
├── rfcs/                        # Keep existing structure
└── examples/                    # Working code samples
    ├── basic-usage/
    ├── self-hosting/
    └── custom-extraction/
```

---

## 5. Tool Recommendation

### Primary: Starlight (Astro)

**Why Starlight:**
- **2026 cutting-edge:** Built on Astro's island architecture, excellent performance
- **Developer-friendly:** Markdown-first with excellent syntax highlighting, built-in search
- **AI-native:** Easy to enhance with structured data, JSON-LD, custom metadata
- **Proven:** Used by Astro themselves, solid community, active development
- **Extensible:** Plugin system for custom components, can add machine-readable features

**Configuration:**
```typescript
// astro.config.mjs
export default defineConfig({
  integrations: [
    starlight({
      title: 'Kinoko',
      description: 'Every problem solved once is solved for everyone.',
      logo: { src: './src/assets/kinoko-logo.svg' },
      social: {
        github: 'https://github.com/kinokoorg/kinoko',
      },
      sidebar: [
        { label: 'Getting Started', autogenerate: { directory: 'getting-started' } },
        { label: 'Guides', autogenerate: { directory: 'guides' } },
        { label: 'Reference', autogenerate: { directory: 'reference' } },
        { label: 'Architecture', autogenerate: { directory: 'architecture' } },
        { label: 'Contributing', autogenerate: { directory: 'contributing' } },
      ],
      customCss: [
        './src/styles/kinoko.css',
      ],
    }),
  ],
});
```

**Alternatives Considered:**
- **Mintlify:** Beautiful, but less flexible for custom machine-readable enhancements
- **Fumadocs:** Excellent, but newer ecosystem, less proven at scale
- **Nextra:** Good, but tied to Next.js when we don't need React complexity
- **VitePress:** Vue-based, great performance, but Starlight has better DX for technical docs

### Supporting Tools

**Docs Testing:** Custom tool to validate code examples
```bash
# examples/test-runner.go
// Extracts and runs code blocks tagged with executable languages
```

**CLI Reference Generation:** 
```bash
kinoko docs generate-cli-reference > docs/reference/cli-reference.md
```

**Schema Documentation:** Auto-generate from Go structs
```bash
kinoko docs generate-schemas > docs/ai/schema/
```

---

## 6. AI-Native Docs Strategy

This is the novel part. Kinoko's docs should be as accessible to AI agents as to human developers.

### Machine-Readable Metadata

**Every doc page gets structured front matter:**
```yaml
---
type: guide|reference|concept|tutorial
audience: [developer, agent, operator]
confidence: 0.9
prerequisites: [basic-usage]
outputs: [working-skill, configured-server]
related: [skill-format, troubleshooting]
agent_context: |
  This document explains how to create skills manually.
  Key concepts: SKILL.md format, validation rules, git workflow.
  Common issues: credential scanning, format validation.
---
```

### llms.txt Convention

**docs/ai/llms.txt:**
```
# Kinoko - Collective Knowledge System

Kinoko automatically extracts reusable knowledge from AI agent work sessions, stores it as version-controlled skills, and injects relevant knowledge into future sessions.

## Core Concepts
- Skills: Structured knowledge in SKILL.md format (YAML front matter + Markdown)
- Extraction: Automatic capture from agent sessions via hooks
- Libraries: Layered skill repositories (local > company > public)
- Injection: Context-aware skill insertion before agent prompts

## Quick Start
1. Install: curl -s https://install.kinoko.dev | sh
2. Server: kinoko serve
3. Client: kinoko init && kinoko remote add home ssh://...
4. Extract: Sessions automatically create skills in ~/.kinoko/skills/

## Key Files
- config: ~/.kinoko/config.yaml
- skills: ~/.kinoko/skills/*.git (repo per skill)
- logs: ~/.kinoko/logs/

## API Endpoints
- GET /api/skills - list available skills
- POST /api/extract - extract skill from session
- GET /api/search - semantic skill search

Full documentation: https://docs.kinoko.dev
```

### Structured Schemas

**JSON Schema for all formats:**
- `/ai/schema/skill.json` — SKILL.md validation schema
- `/ai/schema/config.json` — Configuration file schema  
- `/ai/schema/api.json` — OpenAPI specification

### Agent Integration Documentation

**docs/ai/integration.md:** How other AI systems can consume Kinoko knowledge
- REST API endpoints for skill search and retrieval
- Webhook integration for real-time skill updates
- Embedding search API for semantic discovery

---

## 7. Content Plan (Phase 1 Priority Order)

### Week 1-2: Foundation
1. **README.md refresh** — Stronger hook, clearer quickstart
2. **docs/getting-started/quickstart.md** — 5-minute working example
3. **docs/getting-started/installation.md** — All platform setup
4. **docs/troubleshooting.md** — Common setup issues

### Week 3-4: Reference
5. **docs/reference/cli-reference.md** — Auto-generated from CLI help
6. **docs/reference/config-reference.md** — All configuration options
7. **Move skill-format.md** to reference section
8. **docs/ai/llms.txt** — Machine-readable summary

### Post-Phase 1: Expansion
9. **Starlight site setup** — Full docs site with search
10. **docs/guides/self-hosting.md** — Complete deployment guide
11. **docs/architecture/overview.md** — System design deep-dive
12. **API documentation** — Auto-generated OpenAPI

---

## 8. Creative Ideas (2026 Novel Approaches)

### Living Documentation

**Self-Testing Docs:**
Every code example in the docs is extracted and run in CI. If an example breaks, the docs build fails. Never ship broken examples again.

```markdown
<!-- This code block will be extracted and tested -->
```bash test=true
kinoko serve --port 8080
# Expecting: server starts successfully
```

**Docs from Tests:**
Generate troubleshooting sections from actual test failures. When a test fails in an interesting way, capture the failure mode and solution as documentation.

### Agent-Aware Docs

**Context-Sensitive Help:**
`kinoko help` adapts based on current system state:
- No config file? Show setup steps
- Server not running? Show server commands
- No skills extracted? Show extraction guide

**Documentation as Agent Context:**
When an AI agent encounters a Kinoko error, it automatically pulls relevant documentation sections into its context window for better troubleshooting.

### Interactive Validation

**Embedded Skill Validator:**
Live validation widget embedded in the skill format docs. Paste your SKILL.md content, get instant feedback on format compliance.

**Configuration Generator:**
Interactive form that generates valid `config.yaml` based on user's setup (single machine, multi-remote, cloud integration).

### Knowledge Graph Documentation

**Related Skills Visualization:**
Docs about skills automatically show dependency graphs and related skills from the actual Kinoko database.

**Usage Analytics Integration:**
Documentation sections that are frequently needed get highlighted or promoted. Docs evolve based on real user patterns.

---

## 9. Minimum Viable Docs (Phase 1)

### What Ships With Phase 1

**Essential for usability:**
1. **Strong README** — What/why/quickstart that gets someone running in 5 minutes
2. **Installation guide** — Platform-specific setup instructions
3. **First skill walkthrough** — End-to-end example from extraction to injection
4. **Troubleshooting guide** — Common setup issues and solutions
5. **CLI reference** — Auto-generated from `--help` output
6. **skill-format.md** — Already exists, move to reference section

**Essential for AI agents:**
7. **llms.txt** — Machine-readable project summary
8. **Basic API documentation** — Key endpoints with examples

**Quality gates:**
- All code examples tested in CI
- Each doc has clear audience and purpose
- 5-minute rule validated with fresh users
- Mobile-friendly formatting

### What Comes Later

- Full Starlight docs site (Phase 2)
- Comprehensive API documentation (Phase 2)
- Architecture deep-dives (Phase 2-3)
- Advanced guides (self-hosting, federation, etc.)
- Interactive components

### Success Metrics for Phase 1

- **Time to first working skill:** Under 5 minutes from README
- **Support question reduction:** Fewer "how do I..." questions in community channels
- **Contributor onboarding:** New developers can contribute without asking setup questions
- **Agent integration:** Other AI systems can successfully consume Kinoko knowledge

---

## 10. Implementation Plan

### Phase 1 (Weeks 1-4)

**Week 1:**
- Audit and rewrite README.md with stronger hook
- Create docs/getting-started/quickstart.md
- Set up basic docs structure in repo

**Week 2:**
- Write installation guide for all platforms
- Create troubleshooting guide with common issues
- Set up code example testing in CI

**Week 3:**
- Generate CLI reference from actual CLI
- Move and enhance skill-format.md
- Create config reference documentation

**Week 4:**
- Write llms.txt for AI consumption
- Test all documentation with fresh users
- Polish and deploy

### Phase 2 (Weeks 5-8)

**Starlight deployment:**
- Set up Starlight with custom theme
- Migrate all docs to proper site
- Add search functionality
- Set up automated deployment

### Tools I'll Need

- **Go tooling** for auto-generating CLI and schema docs
- **CI/CD pipeline** for testing code examples
- **Astro/Starlight** for eventual docs site
- **Git hooks** for docs quality gates

---

## Conclusion

Kinoko isn't just another developer tool — it's infrastructure for knowledge itself. The documentation needs to embody the same principles: knowledge should flow freely, be discoverable by both humans and machines, and compound over time.

The approach I'm proposing balances pragmatism (ship useful docs for Phase 1) with innovation (AI-native features for the future). We start with excellent fundamentals — a strong README, clear guides, tested examples — then evolve into something novel: documentation that works as well for AI agents as it does for humans.

The 5-minute rule is non-negotiable. The dual-audience design is the differentiator. The progressive enhancement from README to comprehensive docs site gives us a clear path forward.

Let's build documentation that doesn't just explain Kinoko — let's build documentation that embodies its vision of knowledge that flows and compounds.

---

*"If you have to explain it twice, the docs failed."*