# Kinoko Documentation Site — Design & Architecture

*Charis — February 15, 2026*

---

## Preamble

Kinoko is infrastructure where knowledge flows underground, invisibly, between AI agents and the humans they serve. The documentation site must embody that same philosophy: knowledge should arrive exactly when you need it, in exactly the form you need it, without friction.

This document is the blueprint. It covers visual design direction, information architecture, tooling, and content gap analysis. It is opinionated by design — one recommendation per decision, defended with reasoning.

---

## Part 1: Reference Site Analysis

### Flowglad (flowglad.com)

**What works:** Clean developer-facing aesthetic. "Payments Without Webhooks" — the headline *is* the value prop. Pricing is transparent and inline. The page is dense with information but reads effortlessly because hierarchy is crystal clear. The MCP-first integration callout signals "we understand the 2026 developer."

**What to steal:** The headline-as-value-prop pattern. Flowglad opens with the *problem they kill*, not the product they built. Kinoko should do the same: "Every problem solved once is solved for everyone" above the fold, not "knowledge-sharing infrastructure for AI agents."

**What doesn't fit:** Flowglad is a SaaS product with pricing tables and checkout flows. Kinoko is open-source infrastructure. We don't need conversion-optimized CTAs or pricing tiers. Their visual density works for a single-page product pitch but would overwhelm a docs site.

### Supabase Docs (supabase.com/docs)

**What works:** The gold standard. Left sidebar navigation with collapsible sections. Product-organized landing page (Database, Auth, Storage, Realtime, Edge Functions). Dark mode default. Code examples are first-class citizens, not afterthoughts. Search is instant and excellent. The "Additional resources" section (Management API, CLI, Platform Guides, Integrations, Troubleshooting) provides clear escape hatches.

**What to steal:** Almost everything structural. The sidebar navigation pattern. The product-section cards on the docs landing page. The way code examples sit alongside prose without breaking flow. The breadcrumb + sidebar + content three-column layout. The edit-on-GitHub link on every page.

**What doesn't fit:** Supabase serves dozens of products across multiple languages. Their navigation depth (5+ levels) would be overkill for Kinoko's current scope. Their tone is neutral-corporate — Kinoko should feel warmer, more organic.

### Physical Intelligence (pi.website)

**What works:** Minimalist elegance. The page breathes. Research papers are presented as a clean chronological feed — each entry is title + date + one-sentence summary. No visual clutter. The design says "serious research, no nonsense" without feeling cold.

**What to steal:** The restraint. Not everything needs a card, a gradient, or an animation. The chronological research feed pattern would work beautifully for a changelog or "What's New" section. The generous whitespace is a reminder that density ≠ clarity.

**What doesn't fit:** Pi is a research lab, not a developer tool. Their site is read-only — no interactive elements, no code, no "try it now." Kinoko needs to be more hands-on.

### Sesame (sesame.com)

**What works:** Warm, conversational, human. "We believe in a future where computers are lifelike." The site feels like it was written by a person, not a marketing team. Minimal to the point of austerity — the design gets out of the way of the message.

**What to steal:** The warmth. Kinoko's manifesto already has this voice ("A junior developer in São Paulo starts debugging a problem they've never seen before"). The docs site should carry that same humanity. Also: the courage to say less. Sesame's homepage is ~50 words. That's confidence in your message.

**What doesn't fit:** Sesame is consumer-facing AI. Too minimal for developer docs — developers need code examples, reference tables, and structured information. Pure minimalism would frustrate the people actually trying to use Kinoko.

---

## Part 2: Design Direction

### Visual Identity

**Mood:** Dark, warm, organic. Think forest floor at dusk — not sterile lab, not candy-colored startup.

**Color Palette:**

| Role | Color | Hex | Notes |
|------|-------|-----|-------|
| Background (dark) | Deep Soil | `#0F0F0F` | Near-black with warmth, not blue-tinted |
| Background (light) | Birch | `#FAFAF7` | Off-white, cream warmth |
| Surface | Loam | `#1A1A18` | Cards, code blocks, elevated surfaces |
| Text primary | Kinoko White | `#E8E4DF` | Warm off-white, never pure white |
| Text secondary | Stone | `#9B9590` | Muted, readable on dark backgrounds |
| Accent primary | Spore | `#C4A265` | Warm gold — the color of mycelial networks |
| Accent secondary | Moss | `#5B8C5A` | Life, growth, health indicators |
| Error/Warning | Rust | `#C45B3C` | Decay, problems, things dying |
| Code text | Lichen | `#B8D4B8` | Soft green for code, easy on the eyes |

**Typography:**

- **Headings:** Inter or Geist — clean, technical, modern. No serifs.
- **Body:** System font stack (`-apple-system, BlinkMacSystemFont, 'Segoe UI', ...`) — fast loading, native feel.
- **Code:** JetBrains Mono or Fira Code — with ligatures enabled.
- **Size scale:** 16px base, 1.25 ratio. Generous line-height (1.6 for body, 1.3 for headings).

**Dark mode is default.** Light mode available. The metaphor demands it — mycelial networks operate underground, in the dark. The product *is* the thing working invisibly beneath the surface.

### Design Principles

1. **Underground, not invisible.** The design should feel like discovering something hidden — a network that was there all along. Subtle textures, depth through shadow, layers revealed on interaction.

2. **Show the simplest thing first.** Every page opens with the 10-second version. Complexity is available on demand, never on arrival. Progressive disclosure at every level.

3. **Code is content, not decoration.** Code blocks are first-class elements with the same visual weight as prose. They're not shoved into grey boxes — they're part of the page's typographic rhythm.

4. **Warmth through restraint.** Warm colors, generous whitespace, unhurried pacing. The site should feel like a conversation with someone who knows what they're talking about and isn't in a rush.

5. **Dual-audience by default.** Every page is human-readable AND machine-parseable. Structured frontmatter, semantic HTML, consistent heading hierarchies. Not as an afterthought — as a core architectural decision.

### Personality

Kinoko should feel like **a field guide written by a mycologist who also happens to write beautiful code.** It's knowledgeable without being academic. Precise without being cold. It uses biological metaphors not because they're clever, but because they're accurate — the system really does work like a mycelial network.

The tone is: "Here's something genuinely interesting. Let me show you how it works."

### Key Differentiators

What makes Kinoko docs NOT look like every other developer docs site:

1. **The biological metaphor is structural, not decorative.** The four mental models (Gold Panning, Wine Tasting, Reference Librarian, Forest Fires) aren't just clever names — they organize how information is presented. The concepts section uses these models as navigation anchors.

2. **AI-native docs.** A first-class `/llms.txt` endpoint. Structured metadata on every page. The docs site is designed to be consumed by the very agents Kinoko serves — a meta-loop that most docs sites don't even consider.

3. **The warm dark palette.** Most dev docs are either blue-corporate-dark (Supabase, Stripe) or white-clinical-light (MDN, React). The soil/gold/moss palette says "this is something organic, something alive."

4. **No hero image, no mascot, no marketing fluff.** The landing page opens with the manifesto's core line and immediately shows the core loop. Developers arrive and within 5 seconds know: what this is, why it matters, how to start.

---

## Part 3: Information Architecture

### Site Map

```
/                                   Landing page
├── /docs/                          Docs home (quick navigation hub)
│   ├── /docs/quickstart            5-minute quickstart
│   ├── /docs/installation          Platform-specific setup
│   │
│   ├── /docs/concepts/             Understanding Kinoko
│   │   ├── overview                How Kinoko thinks (the 4 mental models)
│   │   ├── extraction              Gold Panning — the extraction pipeline
│   │   ├── quality                 Wine Tasting — dimensional scoring
│   │   ├── injection               Reference Librarian — skill matching
│   │   ├── decay                   Forest Fires — knowledge lifecycle
│   │   └── skills                  What is a skill? The atomic unit of knowledge
│   │
│   ├── /docs/guides/               How-to guides
│   │   ├── first-skill             Create your first skill manually
│   │   ├── extraction-pipeline     Run and debug extraction
│   │   ├── injection-tuning        Configure and monitor injection
│   │   ├── library-management      Manage skill libraries (local/team/public)
│   │   ├── ab-testing              Set up and analyze A/B tests
│   │   ├── decay-management        Monitor and intervene in decay
│   │   └── feedback-patterns       Developer interaction patterns
│   │
│   ├── /docs/reference/            Technical reference
│   │   ├── cli                     CLI commands and flags
│   │   ├── config                  config.yaml specification
│   │   ├── skill-format            SKILL.md specification
│   │   ├── architecture            Internal architecture (packages, interfaces, data flow)
│   │   ├── taxonomy                Problem pattern taxonomy (the 20 patterns)
│   │   └── glossary                Terminology
│   │
│   ├── /docs/operations/           Running Kinoko
│   │   ├── troubleshooting         Common issues and fixes
│   │   ├── monitoring              Pipeline metrics and health
│   │   └── security                Credential scanning, sanitization, trust model
│   │
│   ├── /docs/contributing/         For contributors
│   │   ├── development-setup       Dev environment, running tests
│   │   ├── architecture-guide      Where code lives, how to navigate
│   │   └── writing-skills          How to write good SKILL.md files
│   │
│   └── /docs/ai/                   For AI agents
│       ├── llms.txt                Machine-readable project summary
│       ├── llms-full.txt           Complete structured reference
│       └── agent-integration       How agents interact with Kinoko
│
├── /blog/                          Changelog, deep dives, design decisions
├── /manifesto                      The Kinoko Manifesto (standalone page)
└── /rfcs/                          Living RFCs
```

### Navigation Design

**Left sidebar** — collapsible, persistent, grouped by section. Follows the Supabase/Starlight pattern because it works and developers expect it.

```
Getting Started
  ├── Quickstart
  └── Installation

Concepts
  ├── Overview
  ├── Extraction (Gold Panning)
  ├── Quality (Wine Tasting)
  ├── Injection (Reference Librarian)
  ├── Decay (Forest Fires)
  └── Skills

Guides
  ├── Your First Skill
  ├── Extraction Pipeline
  ├── Injection Tuning
  ├── Library Management
  ├── A/B Testing
  ├── Decay Management
  └── Feedback Patterns

Reference
  ├── CLI
  ├── Configuration
  ├── Skill Format
  ├── Architecture
  ├── Taxonomy
  └── Glossary

Operations
  ├── Troubleshooting
  ├── Monitoring
  └── Security

Contributing
  ├── Development Setup
  ├── Architecture Guide
  └── Writing Skills

AI Agents
  ├── llms.txt
  └── Agent Integration
```

**Top bar:** Logo + "Docs" | "Blog" | "Manifesto" | "GitHub" | Search | Dark/Light toggle

**Search:** Command-K triggered, full-text. Pagefind (see Tooling).

**Mobile:** Sidebar collapses to hamburger menu. Content takes full width.

### Landing Page Structure

**Above the fold:**

```
[Logo: 🍄 Kinoko]

Every problem solved once is solved for everyone.

Knowledge-sharing infrastructure for AI agents. When an agent
solves a problem, Kinoko extracts the knowledge and injects
it into future sessions — automatically.

[Get Started →]   [View on GitHub →]
```

**Below the fold — The Core Loop:**

A visual diagram (not animated — static SVG) showing:
```
Agent solves problem → Knowledge extracted → Stored in git → Injected into future sessions
```

**Section 2 — How It Works (cards):**

Four cards, one per mental model:
- 🪙 **Gold Panning** — Multi-stage extraction filters noise cheaply
- 🍷 **Wine Tasting** — 7-dimension quality scoring, not thumbs up/down
- 📚 **Reference Librarian** — Pattern matching, not just keyword search
- 🔥 **Forest Fires** — Knowledge decays. That's a feature.

Each card links to its concepts page.

**Section 3 — Quick Start:**

```bash
git clone https://github.com/kinoko-dev/kinoko.git
cd kinoko && go install ./cmd/kinoko
kinoko init && kinoko serve
```

Three commands. That's it.

**Section 4 — Project Status:**

Current stats: 11 packages, 373 tests, ~17K lines of Go. What works today (bulleted list). What's coming next.

### Content Types

| Type | Purpose | Example |
|------|---------|---------|
| **Concept** | Explain *why* something works the way it does | "How extraction works" |
| **Guide** | Walk through *how* to do something specific | "Create your first skill" |
| **Reference** | Precise, complete specification | "CLI Reference", "config.yaml" |
| **Troubleshooting** | Fix a specific problem | "Port already in use" |
| **RFC** | Record design decisions and their rationale | "Vision & References" |

These types form a Diátaxis-inspired taxonomy. Concepts teach understanding. Guides enable action. Reference provides precision. Troubleshooting solves problems. RFCs provide transparency.

### AI Agent Section

The `/docs/ai/` section serves machines:

**`/llms.txt`** — Served at the root domain as well (`kinoko.dev/llms.txt`). Contains:
- Project name, one-line description
- Core concepts (3 sentences each)
- Available CLI commands
- Skill format summary
- Links to llms-full.txt for deep context

**`/llms-full.txt`** — Complete structured reference:
- Full architecture overview
- All CLI commands with flags
- Complete config.yaml specification
- Skill format with validation rules
- Problem pattern taxonomy
- Glossary

**Agent Integration guide** — How an AI agent should interact with Kinoko:
- How injection works from the agent's perspective
- How to format session logs for optimal extraction
- How to interpret injected skills

Both `.txt` files are auto-generated from the docs source at build time. Single source of truth — the docs are the source, the txt files are derived artifacts.

---

## Part 4: Tooling Recommendation

### Static Site Generator: Astro Starlight

**The choice:** [Starlight](https://starlight.astro.build/) (Astro-based).

**Why:**
- Built specifically for documentation. Not a blog framework pretending to be a docs tool.
- Sidebar navigation, search (Pagefind built in), dark mode, i18n — all out of the box.
- Markdown + MDX with frontmatter. Our existing docs drop in with zero rewriting of the format.
- Astro's island architecture means the site is fast by default — static HTML with JS only where needed.
- Component overrides let us customize deeply without forking. We can build the warm, organic aesthetic on top of solid structural defaults.
- Active development, strong community, Astro team backing.

**Why not the others:**
- **Mintlify** — Hosted SaaS. We'd be dependent on their platform for an open-source project. No.
- **Fumadocs** — Next.js based. Good, but heavier than needed. We don't need React islands for docs.
- **Nextra** — Next.js based. Same weight concern. Also: Nextra 3 is still stabilizing.
- **Docusaurus** — React-based, Meta-maintained. Solid but opinionated toward the Facebook design system. The default aesthetic is hard to escape. Also: heavier than Starlight.
- **Custom** — We're building knowledge infrastructure, not a docs framework. Use the good one that exists.

### Hosting: Cloudflare Pages

**Why:** Free for open-source. Global CDN. Git-push deploys. Preview deployments for PRs. Custom domains. No cold starts (it's static). Cloudflare's edge network is fast everywhere — including São Paulo, per the manifesto's vision.

**Why not:**
- **Vercel** — Also good, but Cloudflare's free tier is more generous and we don't need Next.js integration.
- **GitHub Pages** — Works but limited: no preview deploys, no edge functions, slower CDN.

### Search: Pagefind

**Why:** Comes built into Starlight. Client-side search — no external service, no API keys, no cost. Index builds at deploy time. Fast, accurate, works offline. For a project our size, Pagefind is perfect.

**When to reconsider:** If we hit 500+ pages and need faceted search, consider Algolia DocSearch (free for open-source). Not now.

### Extras

| Feature | Implementation |
|---------|---------------|
| **Dark/Light mode** | Starlight built-in. Dark default, CSS custom properties for our palette. |
| **Edit on GitHub** | Starlight built-in. `editLink` config. |
| **OpenGraph images** | Auto-generated via `@astrojs/og` or a custom OG template. Each page gets a branded card. |
| **Version switcher** | Not needed yet. When Kinoko hits v2, add Starlight's version dropdown. |
| **RSS/Changelog** | Astro's built-in RSS for the `/blog/` section. |
| **llms.txt generation** | Custom Astro integration that builds `llms.txt` and `llms-full.txt` from docs source at build time. |
| **Copy code button** | Starlight built-in. |
| **Tabs for code** | Starlight's `<Tabs>` component for multi-language/multi-platform examples. |

### Mapping Existing Docs to the New Site

| Current File | New Location | Status |
|---|---|---|
| `docs/getting-started/quickstart.md` | `/docs/quickstart` | ✅ Ready — minor frontmatter addition |
| `docs/getting-started/installation.md` | `/docs/installation` | ✅ Ready — add Starlight frontmatter |
| `docs/concepts.md` | `/docs/concepts/overview` | ✅ Ready — split into sub-pages per model |
| `docs/architecture.md` | `/docs/reference/architecture` | ✅ Ready — dense, may benefit from diagrams |
| `docs/glossary.md` | `/docs/reference/glossary` | ✅ Ready |
| `docs/feedback-patterns.md` | `/docs/guides/feedback-patterns` | ✅ Ready |
| `docs/reference/cli-reference.md` | `/docs/reference/cli` | ✅ Ready |
| `docs/reference/config-reference.md` | `/docs/reference/config` | ✅ Ready |
| `docs/reference/skill-format.md` | `/docs/reference/skill-format` | ✅ Ready |
| `docs/troubleshooting.md` | `/docs/operations/troubleshooting` | ✅ Ready |
| `MANIFESTO.md` | `/manifesto` (standalone page) | ✅ Ready |
| `rfcs/001-vision-and-references.md` | `/rfcs/001-vision-and-references` | ✅ Ready |
| `docs/ai/llms.txt` | `/docs/ai/llms.txt` | 🔄 Exists, needs review |

---

## Part 5: Content Gap Analysis

### ✅ Exists and Ready (minor edits for site format)

- **Quickstart** — Excellent. Add Starlight frontmatter, done.
- **Installation** — Complete, multi-platform. Ready.
- **CLI Reference** — Thorough. Every command documented. Ready.
- **Config Reference** — Full specification with tables. Ready.
- **Skill Format** — Comprehensive spec with examples. Ready.
- **Glossary** — Complete for current scope. Ready.
- **Troubleshooting** — Covers common issues. Ready.
- **Architecture** — Deeply detailed. Ready (could use diagrams).
- **Concepts (How Kinoko Thinks)** — Beautifully written. The four mental models are superb. Needs splitting into sub-pages.
- **Feedback Patterns** — Complete DX specification. Ready.

### 🔄 Exists but Needs Rewriting or Expansion

| Document | Issue | Work Needed |
|----------|-------|-------------|
| `concepts.md` | Single monolithic file | Split into 5 sub-pages (overview + 4 models). Each gets a focused intro and cleaner navigation. |
| `architecture.md` | Dense, contributor-focused | Add diagrams (Mermaid). Consider splitting "Architecture for users" vs "Architecture for contributors." |
| `docs/ai/llms.txt` | Needs review | Regenerate from current docs. Ensure it reflects all CLI commands and current feature set. |
| Various internal docs (`docs-architecture.md`, `docs-inspiration.md`, etc.) | Planning/research artifacts | Don't publish. Keep in repo for reference but exclude from site build. |

### 🚫 Missing — Needs to Be Written

| Page | Priority | Description |
|------|----------|-------------|
| **Landing page** | P0 | The site's front door. Hero + core loop + 4-model cards + quick start + status. |
| **Concepts: Skills** | P0 | "What is a skill?" — the atomic unit. Currently only covered in the spec, not as a concept. |
| **Guide: Your First Skill** | P0 | Hands-on tutorial. Create, validate, commit a skill. Currently only a test snippet in quickstart. |
| **Guide: Extraction Pipeline** | P1 | How to run extraction, debug rejections, understand stage results. |
| **Guide: Injection Tuning** | P1 | Configure injection, monitor what's being injected, improve relevance. |
| **Guide: Library Management** | P1 | Local vs team vs public libraries. Priority layering. How to set up a team library. |
| **Reference: Taxonomy** | P1 | The 20 problem patterns as a standalone reference page. Currently buried in architecture.md. |
| **Operations: Monitoring** | P1 | How to use `kinoko stats`, what metrics mean, when to worry. |
| **Operations: Security** | P1 | Credential scanning, sanitization, trust model, prompt injection defenses. |
| **Contributing: Development Setup** | P1 | Clone, build, test, run. For people who want to contribute code. |
| **Contributing: Architecture Guide** | P2 | Package map, where to make changes, interface boundaries. |
| **Contributing: Writing Skills** | P2 | Style guide for SKILL.md files. What makes a good skill. |
| **Guide: A/B Testing** | P2 | Set up, run, and interpret A/B tests. |
| **Guide: Decay Management** | P2 | Monitor decay, rescue skills, expedite deprecation. |
| **AI: Agent Integration** | P2 | How agents consume and contribute to Kinoko. |
| **Blog: Launch Post** | P2 | "What is Kinoko and why we built it." |
| **llms-full.txt** | P2 | Complete structured reference for AI agent consumption. Auto-generated. |

### Recommended Build Order

**Phase 1 — Minimum Viable Docs Site (1-2 days):**
1. Starlight project scaffold with custom theme (palette, fonts)
2. Port all "Ready" docs with frontmatter
3. Split concepts.md into sub-pages
4. Write the landing page
5. Deploy to Cloudflare Pages

**Phase 2 — Essential Guides (3-5 days):**
1. Write "Your First Skill" guide
2. Write Extraction Pipeline guide
3. Write Injection Tuning guide
4. Extract Taxonomy into standalone reference
5. Write Security operations page
6. Generate llms.txt at build time

**Phase 3 — Complete Coverage (1-2 weeks):**
1. Remaining guides (Library Management, A/B Testing, Decay Management)
2. Contributing section
3. Monitoring operations page
4. Agent Integration guide
5. Blog section with launch post
6. llms-full.txt generation

---

## Closing Thought

Kinoko's documentation should be the first skill the system extracts about itself. If we build it right — structured, honest, dual-audience — the docs become a proof of concept for the product. An AI agent reading our docs should get better at using Kinoko automatically. A developer reading our docs should understand the system in 5 minutes and be productive in 15.

The manifesto says: "Knowledge sharing should be a byproduct of work, not a separate activity." The docs site is where we prove we believe that.

Let's build it.
