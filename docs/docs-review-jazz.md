# Documentation Review: Jazz's Reality Check

*Jazz — February 15, 2026*

---

## TL;DR

Charis has written a beautiful, comprehensive proposal for building a documentation system worthy of a 50-person engineering team. We have 3 developers and no users. This is like designing a cathedral when you need an outhouse.

**Bottom line:** Half the ideas are solid, half are premature optimization, and the priorities are completely backwards. Let me explain why.

---

## What She Got Right

I'll give credit where it's due, even if it pains me:

### The Brutal Audit ✓

She actually looked at what we have (almost nothing) and what we need (everything). The audit is spot-on:
- Missing quickstart guide
- Missing installation docs  
- Missing troubleshooting
- Missing examples

This is competent technical writing. She did her homework.

### The 5-Minute Rule ✓

"If getting started takes more than 5 minutes from README to working example, the docs failed."

FINALLY. Someone who gets it. I've seen too many projects die because developers can't figure out how to run the damn thing. This is non-negotiable for developer tools.

### Testing Code Examples ✓

"Every code example that appears in docs must run in CI. Broken examples are worse than no examples."

*grudging nod* This is how you do it. I've spent decades debugging examples that worked on the author's machine 6 months ago. Test your docs or don't ship them.

### README-First Approach ✓

Starting with a better README instead of jumping straight to a docs site shows restraint. Rare in a technical writer.

---

## What She Got Wrong (The Big Stuff)

### 1. Overengineering for Team Size 🚨

**Reality check:** We have 3 developers working on a prototype that "not ready for use." 

**Charis proposes:**
- Full Starlight docs site with custom theme
- Dual-audience documentation strategy  
- AI-native metadata schemas
- Interactive validation widgets
- "Living documentation" with self-testing examples
- Four-phase rollout over 12 weeks

**What we actually need:** A decent README and maybe 3-4 Markdown files.

Look, I've been doing this for 30 years. You know what kills more projects than bad docs? **Over-documenting before you have users.** Every hour spent on docs architecture is an hour not spent making the product work.

### 2. Wrong Tool for Wrong Time 🚨

**Starlight in Phase 1 is premature optimization.**

We have **zero users.** Nobody is browsing our docs site because nobody knows we exist. You want to spend weeks setting up Astro config and custom themes for an audience of... us?

Start with Markdown files in the repo. When we have 100 GitHub stars and people filing "unclear docs" issues, THEN we talk about docs sites.

### 3. AI-Native Strategy: 50% Solid, 50% Buzzword Soup

**The good stuff:**
- `llms.txt` convention is actually useful
- Structured front matter for machine parsing makes sense
- JSON schemas for validation - practical

**The buzzword soup:**
- "Dual-audience design" - just write clear docs, they'll work for humans and machines
- "Agent-Aware Docs" - what the hell does this even mean?
- "Documentation as Agent Context" - solving problems we don't have yet

**Reality:** Write good docs first. Add AI-specific features when AI agents are actually using the system. Which they're not, because it doesn't work yet.

### 4. Priority Order is Backwards 🚨

**Charis's Phase 1:**
1. README refresh
2. Quickstart guide  
3. Installation docs
4. Troubleshooting
5. CLI reference
6. Config reference
7. Move skill-format.md
8. llms.txt

**What should actually ship first:**
1. Make the damn thing run (`mycelium serve` works)
2. Write a README that explains what this does
3. Write installation instructions that work
4. ONE working example
5. STOP. SHIP IT.

Everything else is premature. We're in Phase 1 of a 12-week plan for a 3-person team. By week 8, half the architecture will have changed and half the docs will be wrong.

---

## The Missing Reality Check

### Can This Team Actually Maintain This?

Let's do some math:

- 3 developers
- 12-week docs plan  
- "Test all examples in CI"
- "Auto-generate CLI reference"
- "Living documentation"
- "Four-phase rollout"

**Who's going to maintain this when the architecture changes?** Because it will change. This is a prototype. The API will shift, the CLI will evolve, the config format will break.

Every fancy docs feature is technical debt. Starlight sites need updates. Generated docs need regeneration when code changes. Self-testing examples break when APIs change.

**The bus factor:** If Charis gets hit by a bus, who maintains the docs? Hal? Egor? They're building the actual product.

### The "Zero Users" Problem

The proposal optimizes for discoverability and onboarding when we have nobody to discover or onboard. It's architectural masturbation.

**What we actually need RIGHT NOW:**
- Hal needs to remember how to run the server tomorrow
- Egor needs to not ask "how do I set up the client again?"
- The next person who joins the team needs to get running in 5 minutes

That's it. That's the whole docs requirement.

---

## What We Should Actually Do

### Week 1: The Essential Four

1. **README.md** - What is this, why should I care, how do I try it
   ```markdown
   # Mycelium
   
   AI agents sharing knowledge automatically.
   
   ## Quick Start
   ```
   
2. **INSTALL.md** - Platform-specific setup that actually works
3. **EXAMPLE.md** - One full walkthrough: server → client → extract skill → inject skill  
4. **TROUBLESHOOTING.md** - The 3-5 things that will definitely go wrong

### Week 2: Stop

Seriously. Stop. Use the system. Let the docs break. Fix them when they break. Learn what people actually need.

### When to Add More

- **100+ GitHub stars:** Maybe add a proper docs site
- **External contributors:** Add CONTRIBUTING.md
- **API users:** Generate API docs
- **Multiple deployment patterns:** Add deployment guides

### The llms.txt Exception

I'll grudgingly admit this one makes sense. Since the whole point is AI agents sharing knowledge, having a machine-readable summary is actually practical. 

But make it simple:
```
# Mycelium - AI Knowledge Sharing

What: AI agents extract skills from work sessions automatically
How: Install server, configure clients, skills flow through git repos  
Status: Early prototype, self-hosting only

Full docs: README.md
```

Don't overthink it.

---

## The Brutal Questions

### 1. Realism: NO

Can a 3-person team maintain this docs system? Absolutely not. This is designed for a team with a dedicated technical writer and docs engineer. We have neither.

### 2. Tool Choice: WRONG FOR NOW

Starlight is a good tool. For later. Right now it's solving problems we don't have while creating maintenance burden we can't handle.

### 3. Priority: BACKWARDS  

"Week 3-4: Reference documentation" when the thing doesn't run reliably? Come on.

### 4. AI-Native Strategy: MIXED BAG

50% practical (llms.txt, schemas), 50% performative buzzwords ("agent-aware docs", "living documentation"). 

Focus on the practical half.

### 5. Missing Things: THE SHED, NOT THE CATHEDRAL

What's missing is humility. What's missing is "what's the simplest thing that could possibly work?"

What I want as a developer finding this project:
- Can I run it in 5 minutes?
- Does it do what it claims?
- How do I know if it's working?

I don't want a docs site. I don't want AI-native metadata. I want it to work.

### 6. Overengineering: ABSOLUTELY

This is a textbook case of premature optimization. Building infrastructure before you know what you need.

### 7. The 5-Minute Rule: FAILS

Can we achieve this with the current codebase? Let's see:
- "Early development"
- "Not ready for use"  
- No installation instructions
- No working examples

The 5-minute rule fails because the SOFTWARE fails the 5-minute rule. Fix the software first.

---

## The Grudging Recommendation

Here's what I'd do if I were forced at gunpoint to document this thing:

### Ship This Week

**README.md** (rewrite):
```markdown
# Mycelium

AI agents sharing knowledge automatically.

**Status:** Early prototype. Self-hosting only. Will break.

## What It Does

Your AI agent solves a problem. Mycelium extracts what it learned. 
Other people's agents automatically know the solution.

## Quick Start

[Server setup - 3 steps]
[Client setup - 2 steps]  
[Working example - 1 skill extraction + injection]

## Current Limitations

- No authentication
- Skills not filtered for quality  
- Will definitely break
- Don't use in production

That's it.
```

**INSTALL.md:**
Platform-specific instructions. Test them on fresh machines.

**EXAMPLE.md:**
One complete walkthrough that actually works.

**TROUBLESHOOTING.md:**
The 5 things that will go wrong.

### What NOT to Ship

- Docs sites
- AI-native metadata schemas  
- Interactive components
- Multi-phase rollout plans
- Tool recommendations for Phase 2

### When to Revisit

When you have users complaining about the docs. That's a good problem to have.

---

## Final Verdict

Charis clearly knows how to write docs. The audit is thorough, the 5-minute rule is sound, and the testing philosophy is solid.

But she's designing a docs system for the wrong team at the wrong time solving the wrong problems.

**Good:** Understanding of developer needs, practical testing approach, README-first thinking

**Bad:** Overengineering, wrong priorities, buzzword-heavy AI strategy

**Ugly:** Building a cathedral when we need a shed

## My Recommendation

1. Ship the Essential Four (README, INSTALL, EXAMPLE, TROUBLESHOOTING)
2. Use them daily until they break
3. Fix them when they break
4. Repeat until we have real users
5. THEN talk about docs sites

Remember: **Docs that don't exist are worse than imperfect docs that do.**

But docs that take 3 months to build for a 3-person team are worse than both.

---

*"Perfect documentation for vaporware is still vaporware."*